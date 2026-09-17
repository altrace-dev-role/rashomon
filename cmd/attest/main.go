// Command attest records what a Claude Code session asked to run.
//
// This program never exits with status 2. Exit code 2 from a PreToolUse hook
// blocks the tool call, and Claude Code documents that a JSON permissionDecision
// of allow cannot override it. A recorder that blocks the thing it is recording
// has stopped being a recorder, so 2 is not a status this binary is allowed to
// produce -- not for a usage error, not for a missing store, not for a panic.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/altrace-dev-role/altrace-attest/internal/hook"
	"github.com/altrace-dev-role/altrace-attest/internal/install"
	"github.com/altrace-dev-role/altrace-attest/internal/report"
	"github.com/altrace-dev-role/altrace-attest/internal/safe"
	"github.com/altrace-dev-role/altrace-attest/internal/settings"
	"github.com/altrace-dev-role/altrace-attest/internal/store"
)

// version is set at build time.
var version = "dev"

// The only two statuses this program produces. Notably absent: 2.
const (
	exitOK   = 0
	exitFail = 1
)

func main() {
	// os.Exit skips deferred functions, so every path that needs one has to
	// have returned before this line is reached.
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return exitFail
	}
	rest := args[1:]

	switch args[0] {
	case "hook":
		return cmdHook(stdin, stderr)
	case "probe":
		return cmdProbe(rest, stdin, stderr)

	case "watch":
		return guarded(stderr, func() error { return cmdWatch(stdout) })
	case "detach":
		return guarded(stderr, func() error { return cmdDetach(stdout) })
	case "report":
		return guarded(stderr, func() error { return cmdReport(rest, stdout) })
	case "forget":
		return guarded(stderr, func() error { return cmdForget(rest, stdout) })

	case "version":
		fmt.Fprintln(stdout, version)
		return exitOK

	default:
		usage(stderr)
		return exitFail
	}
}

// guarded runs a CLI command under the panic barrier. A panic in watch or
// detach is not a hook failure and could exit 2 harmlessly, but there is no
// reason to let this binary learn how to produce that number anywhere.
func guarded(stderr io.Writer, fn func() error) int {
	if err := safe.Guard(fn); err != nil {
		fmt.Fprintln(stderr, "attest:", err)
		return exitFail
	}
	return exitOK
}

// cmdHook handles one PreToolUse invocation and always succeeds.
//
// Every failure below is swallowed into exit 0 on purpose. There is no failure
// of this program that is worth blocking a user's tool call over, and a
// recorder that cannot record has no standing to veto anything. What it can do
// honestly is decline to claim coverage it does not have, which is what the
// coverage record is for.
//
// Nothing is written to stdout on this path. Claude Code parses hook stdout as
// control output, so anything printed there is a second way to affect a
// decision this program has no business affecting.
func cmdHook(stdin io.Reader, stderr io.Writer) int {
	sig := hook.WatchSignals()
	defer sig.Stop()

	st := openForHook(stderr)
	if st == nil {
		return exitOK
	}

	h := hook.New(st, time.Now)
	captureErr := safe.Guard(func() error { return h.Capture(stdin) })
	_ = safe.Guard(func() error { h.Close(sig, captureErr); return nil })
	return exitOK
}

// cmdProbe handles SessionStart and SessionEnd, under the same rule as hook:
// it is invoked by Claude Code and it always succeeds.
func cmdProbe(args []string, stdin io.Reader, stderr io.Writer) int {
	sig := hook.WatchSignals()
	defer sig.Stop()

	phase := ""
	if len(args) > 0 {
		phase = args[0]
	}
	if phase != store.PhaseStart && phase != store.PhaseEnd {
		fmt.Fprintln(stderr, "attest: probe expects start or end")
		return exitOK
	}

	st := openForHook(stderr)
	if st == nil {
		return exitOK
	}
	_ = safe.Guard(func() error { hook.RunProbe(sig, phase, stdin, st, time.Now); return nil })
	return exitOK
}

// openForHook opens the store for a hook-invoked path. It prints a fixed
// string and returns nil on failure: there is nowhere to record that recording
// failed, and the absence of any record for the session is the signal.
func openForHook(stderr io.Writer) *store.Store {
	root, err := store.DefaultRoot()
	if err != nil {
		fmt.Fprintln(stderr, "attest: store location unresolved")
		return nil
	}
	st, err := store.Open(root)
	if err != nil {
		fmt.Fprintln(stderr, "attest: store unavailable")
		return nil
	}
	return st
}

func openStore() (*store.Store, error) {
	root, err := store.DefaultRoot()
	if err != nil {
		return nil, err
	}
	return store.Open(root)
}

// editAttempts bounds the read-modify-write loop against a file Claude Code
// may rewrite at any moment. Three is generous: the window is one small write.
const editAttempts = 3

// editSettings applies edit to the user's settings file with optimistic
// concurrency: the file is re-read and the edit re-applied if it changed
// underneath. It reports whether anything was written.
func editSettings(path string, edit func(*settings.Document) (bool, error)) (bool, error) {
	for attempt := 0; attempt < editAttempts; attempt++ {
		original, err := settings.ReadOriginal(path)
		if err != nil {
			return false, err
		}
		doc, err := settings.Parse(original)
		if err != nil {
			return false, fmt.Errorf("%s: %w", path, err)
		}
		changed, err := edit(doc)
		if err != nil || !changed {
			return false, err
		}
		err = settings.Write(path, original, doc.Bytes())
		if errors.Is(err, settings.ErrChanged) {
			continue
		}
		return err == nil, err
	}
	return false, errors.New("settings file kept changing while it was being edited; try again")
}

func cmdWatch(stdout io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	loc, err := settings.DefaultLocations(cwd)
	if err != nil {
		return err
	}
	decision, err := settings.HooksDisabled(loc)
	if err != nil {
		return err
	}
	if decision.Disabled {
		return fmt.Errorf("hooks are disabled by the %s settings layer; an installed entry would never run", decision.Layer)
	}

	exe, err := install.Executable()
	if err != nil {
		return err
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	spec := install.Spec{Executable: exe, InstallID: st.InstallID()}

	changed, err := editSettings(loc.User, func(doc *settings.Document) (bool, error) {
		return install.Apply(doc, spec)
	})
	if err != nil {
		return err
	}
	if !changed {
		fmt.Fprintf(stdout, "attest: already watching (install %s) via %s\n", st.InstallID(), loc.User)
		return nil
	}
	fmt.Fprintf(stdout, "attest: watching (install %s) via %s\n", st.InstallID(), loc.User)
	return nil
}

func cmdDetach(stdout io.Writer) error {
	path, err := settings.UserPath()
	if err != nil {
		return err
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	spec := install.Spec{InstallID: st.InstallID()}

	removed := 0
	changed, err := editSettings(path, func(doc *settings.Document) (bool, error) {
		n, err := install.Remove(doc, spec)
		removed = n
		return n > 0, err
	})
	if err != nil {
		return err
	}
	if !changed {
		fmt.Fprintf(stdout, "attest: nothing to detach (install %s) in %s\n", st.InstallID(), path)
		return nil
	}
	fmt.Fprintf(stdout, "attest: detached (install %s, %d entries) from %s\n", st.InstallID(), removed, path)
	return nil
}

func cmdReport(args []string, stdout io.Writer) error {
	sessionID := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session":
			if i+1 >= len(args) {
				return errors.New("--session needs a value")
			}
			sessionID = args[i+1]
			i++
		default:
			return fmt.Errorf("unknown argument %q", args[i])
		}
	}

	st, err := openStore()
	if err != nil {
		return err
	}
	rep, err := report.Build(st, sessionID, time.Now())
	if err != nil {
		return err
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

func cmdForget(args []string, stdout io.Writer) error {
	var since time.Time
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--since":
			if i+1 >= len(args) {
				return errors.New("--since needs a value")
			}
			t, err := parseSince(args[i+1], time.Now())
			if err != nil {
				return err
			}
			since = t
			i++
		default:
			return fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if since.IsZero() {
		return errors.New("forget needs --since <RFC3339 time or duration such as 24h>")
	}

	st, err := openStore()
	if err != nil {
		return err
	}
	gaps, err := st.Forget(since, time.Now())
	if err != nil {
		return err
	}
	total := 0
	for _, g := range gaps {
		total += g.RemovedRecords
	}
	fmt.Fprintf(stdout, "attest: forgot %d records across %d runs since %s; %d gap records written\n",
		total, len(gaps), since.UTC().Format(time.RFC3339), len(gaps))
	return nil
}

// parseSince accepts a duration ("24h", meaning that long ago) or an RFC3339
// timestamp.
func parseSince(v string, now time.Time) (time.Time, error) {
	if d, err := time.ParseDuration(v); err == nil {
		return now.Add(-d), nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("--since %q is neither a duration nor an RFC3339 time", v)
}

func usage(w io.Writer) {
	fmt.Fprint(w, `attest -- record what a Claude Code session asked to run

usage:
  attest watch                 install the PreToolUse recorder and the
                               SessionStart/SessionEnd liveness probe
  attest detach                remove them, leaving everything else as found
  attest report [--session S]  render declarations and coverage as JSON
  attest forget --since T      evict records, leaving a coverage gap behind
  attest version               print the version

invoked by Claude Code, never by hand:
  attest hook                  handle one PreToolUse invocation
  attest probe start|end       handle SessionStart / SessionEnd
`)
}
