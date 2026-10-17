package store

import (
	"errors"
	"os"
	"path/filepath"
)

// FileSupervisor is the supervisor's own log at the store root: one line of
// codes per hook entry that failed, was killed, could not be started, or
// had its output refused. It carries no id, command, path or payload text.
const FileSupervisor = "supervisor.ndjson"

// SupervisorLogCap is supervisor.ndjson's size cap. A line that would take
// the file over it is refused, not truncated, so the file only ever holds
// whole lines.
const SupervisorLogCap = 256 << 10

// ErrStoodDown is returned by every write a Store refuses because its
// stand-down question (StandDownWhen) answered yes. Nothing was written.
var ErrStoodDown = errors.New("store: standing down; nothing written")

// ErrSideFileCap is returned for a write that would take a capped side file
// over its cap. Nothing was written.
var ErrSideFileCap = errors.New("store: side file is at its cap; nothing written")

// Supervisor outcomes: what one line of supervisor.ndjson says happened.
const (
	// OutcomeExitNonZero: the child exited with a status other than 0.
	OutcomeExitNonZero = "exit_nonzero"
	// OutcomeSignalled: a signal the supervisor did not send ended the child.
	OutcomeSignalled = "signalled"
	// OutcomeKilledAtHardStop: the child was still running at the hard stop
	// and the supervisor killed its process group.
	OutcomeKilledAtHardStop = "killed_at_hard_stop"
	// OutcomeSpawnFailed: the child could not be started.
	OutcomeSpawnFailed = "spawn_failed"
	// OutcomeOutputRefused: the child exited 0, but what it wrote to stdout
	// failed the output check or overflowed its cap, so nothing was printed.
	OutcomeOutputRefused = "output_refused"
)

// SupervisorLine is one line of supervisor.ndjson. Entry is the hook entry's
// subcommand ("hook", "probe start", ...) and Event the hook event it served,
// when known. ChildExit is the child's exit status, -1 when it has none
// (ended by a signal, or never started); Signal names the signal that ended
// it, empty when none did.
type SupervisorLine struct {
	AtUnixMS  int64  `json:"at_unix_ms"`
	Entry     string `json:"entry"`
	Event     string `json:"event"`
	Outcome   string `json:"outcome"`
	ChildExit int    `json:"child_exit"`
	Signal    string `json:"signal"`
}

// StandDownWhen gives the Store a question it asks before every write it
// starts: each append, the probe marker, and each run an eviction removes.
// When it answers yes the write is refused with ErrStoodDown before anything
// is touched -- no directory, no line, no seq -- and a write already under
// way is never interrupted, so a Store that stands down leaves no torn line.
//
// It is for the supervised child: its parent gone, or its own hard stop
// passed. Flush is not a write and is not refused; it makes durable what
// was already written.
func (s *Store) StandDownWhen(fn func() bool) { s.standDown = fn }

// stoodDown asks the stand-down question, if there is one.
func (s *Store) stoodDown() bool { return s.standDown != nil && s.standDown() }

// supervisorBudget is how long the supervisor waits for a lock on its
// kill-and-trace path. It is short for the reason spillBudget is: the
// supervisor has already spent its entry's budget waiting on the child.
const supervisorBudget = spillBudget

// AppendSupervisorCoverage appends the supervisor's coverage record for a
// child that failed, was killed, or could not be started. The child may have
// died in the middle of its own append to the same file, leaving a last line
// with no newline; a newline goes first in that case, in the same write, so
// the torn fragment stays one unreadable line, which readers skip, and this
// record reads whole.
//
// The lock is taken under the spill budget and given up the way a spill's
// is: the record is written anyway, as one O_APPEND write. A writer racing it
// can at worst leave an empty line, which readers skip too.
func (s *Store) AppendSupervisorCoverage(rec Coverage) error {
	return s.appendLine(filepath.Join(s.RunDir(rec.SessionID), FileCoverage), rec,
		appendOpts{budget: supervisorBudget, bestEffort: true, sealTail: true})
}

// AppendSupervisorLine appends one line to supervisor.ndjson, refusing it
// with ErrSideFileCap if it would take the file over SupervisorLogCap. The
// cap is checked under the file's lock, so the lock is not given up: a line
// that cannot take it inside the budget is not written. The line is not
// synced: every supervisor with a failed child appends here, and a sync
// inside the lock would queue each behind the others' on the path the hard
// stop's margin covers. A crash can lose it; it is diagnostics.
func (s *Store) AppendSupervisorLine(line SupervisorLine) error {
	return s.appendLine(filepath.Join(s.root, FileSupervisor), line,
		appendOpts{budget: supervisorBudget, sealTail: true, noSync: true, capBytes: SupervisorLogCap})
}

// tornTail reports whether f, an open record file, ends in the middle of a
// line: non-empty with a last byte that is not a newline.
func tornTail(f *os.File) (bool, int64, error) {
	info, err := f.Stat()
	if err != nil {
		return false, 0, err
	}
	size := info.Size()
	if size == 0 {
		return false, 0, nil
	}
	var last [1]byte
	if _, err := f.ReadAt(last[:], size-1); err != nil {
		return false, size, err
	}
	return last[0] != '\n', size, nil
}
