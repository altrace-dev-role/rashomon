package main

import (
	"io"
	"os"
	"strconv"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/fault"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/supervise"
)

// processStart stands in for the process's start: package variables of main
// are initialised after every package it imports, so it is late by their
// init, which RES-2 holds small. Every deadline the supervisor and its child
// keep is measured from it.
var processStart = time.Now()

// parent is the supervisor this process is the child of, nil when it is
// not one: a CLI command, an in-process test, or a platform without
// supervision. Its StandDown is asked before every write a hook entry
// starts.
var parent *supervise.Child

// The fixed lines a hook entry writes to stderr, and the only lines of a
// child's stderr the supervisor passes on, when the child exited 0.
const (
	storeUnresolvedLine  = "rashomon: store location unresolved"
	storeUnavailableLine = "rashomon: store unavailable"
	probePhaseLine       = "rashomon: probe expects start or end"
)

var entryLines = [...]string{standDownLine, flushLostLine, storeUnresolvedLine, storeUnavailableLine, probePhaseLine}

// relayed reports whether a child's stderr line is one the supervisor
// passes on.
func relayed(line string) bool {
	for _, l := range entryLines {
		if line == l {
			return true
		}
	}
	return fault.RelayedStderr(line)
}

// dispatch is main's: the supervised child runs the entry it was started
// for; a hook entry is supervised; anything else runs as it always has.
func dispatch(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	c, rest, isChild := supervise.Attach(args)
	if isChild {
		parent = c
		if fault.Enabled() {
			fault.RecordSupervisor("child pid=" + strconv.Itoa(os.Getpid()))
		}
		// Outside every barrier on purpose: a fault here is a child the
		// supervisor has to turn into exit 0, fatal errors included.
		fault.Inject(fault.PointSuperviseChild)
		return run(rest, stdin, stdout, stderr)
	}
	events := install.EntryEvents(rest)
	if len(events) == 0 {
		return run(rest, stdin, stdout, stderr)
	}
	timeout := 0
	for _, ev := range events {
		if t := install.TimeoutFor(ev); timeout == 0 || t < timeout {
			timeout = t
		}
	}
	cfg := supervise.Config{
		Args:         rest,
		Events:       events,
		Start:        processStart,
		HardStop:     supervise.HardStop(timeout, events, os.Getenv(supervise.SessionEndTimeoutEnv)),
		StdinCap:     hookio.MaxPayloadBytes + 1,
		Stdin:        stdin,
		Stdout:       stdout,
		Stderr:       stderr,
		Executable:   os.Executable,
		BeforeSpawn:  func() error { return fault.Fail(fault.PointSuperviseSpawn) },
		Relayed:      relayed,
		Record:       func(f supervise.Failure) bool { return recordSupervised(rest, f, time.Now()) },
		BeforeStdout: func() { fault.RecordSync(fault.SyncStdout, "stdout") },
		Direct:       func() int { return run(rest, stdin, stdout, stderr) },
	}
	if fault.Enabled() {
		cfg.Observe = observeSupervisor
	}
	return supervise.Run(cfg)
}

// observeSupervisor writes the fault build's supervisor log line.
func observeSupervisor(s supervise.Stats) {
	fault.RecordSupervisor("parent pid=" + strconv.Itoa(os.Getpid()) +
		" child_pid=" + strconv.Itoa(s.ChildPID) +
		" parent_maxrss=" + strconv.FormatInt(s.ParentMaxRSS, 10) +
		" child_maxrss=" + strconv.FormatInt(s.ChildMaxRSS, 10) +
		" outcome=" + s.Outcome)
}

// entryPhase is the coverage phase of a hook entry's records, empty for the
// recap entries, which write none.
func entryPhase(args []string) string {
	if len(args) == 0 {
		return ""
	}
	switch args[0] {
	case "hook":
		return store.PhaseCall
	case "post":
		return store.PhasePost
	case "probe":
		if len(args) > 1 && (args[1] == store.PhaseStart || args[1] == store.PhaseEnd) {
			return args[1]
		}
	}
	return ""
}

// recordSupervised is the supervisor's recorder. For a failed child it
// appends the coverage record of the entry's phase, carrying the reason and
// the session id the supervisor read, and for a failed recap marks the recap
// unhealthy where status shows it; for every failure, a refused output
// included, it appends a supervisor.ndjson line. It writes nothing where no
// store exists -- minting one to say a hook failed would be worse than saying
// nothing -- and nothing for another install's entry, as the child would not
// have. While recording is paused the coverage record is not written either.
//
// It reports false when the coverage record or its flush failed.
func recordSupervised(args []string, f supervise.Failure, now time.Time) bool {
	root, err := store.DefaultRoot()
	if err != nil {
		return f.Reason == ""
	}
	if !storeExistsAt(root) {
		return true
	}
	st, err := store.OpenExisting(root)
	if err != nil {
		return f.Reason == ""
	}
	if id := installArg(args); id != "" && id != st.InstallID() {
		return true
	}

	// Group commit, as in hook and post: barriers inside the locks, one
	// full sync after the last append, outside them.
	st.DeferDurability()
	var covErr error
	phase := entryPhase(args)
	if f.Reason != "" && phase != "" && !pausedAt(root) {
		covErr = st.AppendSupervisorCoverage(supervisedCoverage(st, phase, f, now))
	}
	if f.Reason != "" && len(args) > 0 && args[0] == "recap" {
		recap.RecordFailure(root, now)
	}
	// The entry is named from the install's own table, never copied from
	// argv, so the line holds codes only.
	entry := ""
	if events := install.EntryEvents(args); len(events) > 0 {
		entry = install.Subcommand(events[0])
	}
	st.AppendSupervisorLine(store.SupervisorLine{ //nolint:errcheck // best effort by design: a diagnostics file at its cap must not turn a recorded failure into an unrecorded one
		AtUnixMS:  now.UnixMilli(),
		Entry:     entry,
		Event:     f.Event,
		Outcome:   f.Outcome,
		ChildExit: f.ChildExit,
		Signal:    f.Signal,
	})
	flushErr := st.Flush()
	return covErr == nil && flushErr == nil
}

// pausedAt reports whether recording is paused at root. A pause that cannot
// be read is not a pause: the failure is recorded.
func pausedAt(root string) bool {
	paused, _, err := store.Paused(root)
	return err == nil && paused
}

// supervisedCoverage is the coverage record for a failed child: the entry's
// phase, unverified, with the supervisor's reason and the session id it
// read, or the unattributed bucket when it read none. The hook entry and
// probe are unknown: the supervisor resolves no configuration.
func supervisedCoverage(st *store.Store, phase string, f supervise.Failure, now time.Time) store.Coverage {
	sessionID := f.SessionID
	if sessionID == "" {
		sessionID = hook.UnattributedSession
	}
	reason := f.Reason
	return store.Coverage{
		Type:          store.TypeCoverage,
		SchemaVersion: store.SessionSchemaVersion,
		RecordedAtMS:  now.UnixMilli(),
		SessionID:     sessionID,
		InstallID:     st.InstallID(),
		Phase:         phase,
		State:         store.StateUnverified,
		Reason:        &reason,
		HookEntry:     store.EntryUnknown,
		Probe:         store.ProbeUnknown,
	}
}

// standingDown reports whether this process is a supervised child that must
// start nothing new: its supervisor has gone, or the hard stop has passed.
func standingDown() bool { return parent.StandDown() }

// guardStore gives a hook entry's store the supervised child's stand-down
// question, so every write it starts asks it first.
func guardStore(st *store.Store) *store.Store {
	if st != nil && parent != nil {
		st.StandDownWhen(parent.StandDown)
	}
	return st
}
