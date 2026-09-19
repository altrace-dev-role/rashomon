// Command rashomon records what a Claude Code session asked to run.
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
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/launch"
	"github.com/altrace-dev-role/rashomon/internal/posture"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/safe"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/store"
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
		return cmdHook(rest, stdin, stderr)
	case "post":
		return cmdPost(rest, stdin, stderr)
	case "probe":
		return cmdProbe(rest, stdin, stderr)

	case "watch":
		return guarded(stderr, func() error { return cmdWatch(stdout) })
	case "detach":
		return guarded(stderr, func() error { return cmdDetach(rest, stdout) })
	case "status":
		return guarded(stderr, func() error { return cmdStatus(stdout) })
	case "report":
		return guarded(stderr, func() error { return cmdReport(rest, stdout) })
	case "forget":
		return guarded(stderr, func() error { return cmdForget(rest, stdout) })
	case "env":
		return guarded(stderr, func() error { return cmdEnv(rest, stdout) })
	case "run":
		// Not under guarded(): run returns the CHILD's exit code, and a
		// wrapper that flattened it to 0 or 1 would break every script that
		// checks the status of the command it thought it was running.
		return cmdRun(rest, stdin, stdout, stderr)

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
		fmt.Fprintln(stderr, "rashomon:", err)
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
func cmdHook(args []string, stdin io.Reader, stderr io.Writer) int {
	// The WHOLE body, not just Capture and Close. The prologue -- signal
	// watching, opening the store, reading the install id -- was outside the
	// barrier, and a panic there exits 2, which blocks the user's tool call:
	// the single outcome this program is built to make impossible. Its own file
	// comment says "not for a panic", and three of the four entry points took
	// it on faith for the first few statements.
	_ = safe.Guard(func() error {
		sig := hook.WatchSignals()
		defer sig.Stop()

		st := openForHook(stderr)
		if st == nil {
			return nil
		}
		if standsDown(args, st, stderr) {
			return nil
		}

		h := hook.New(st, time.Now)
		captureErr := safe.Guard(func() error { return h.Capture(stdin) })
		_ = safe.Guard(func() error { h.Close(sig, captureErr); return nil })
		return nil
	})
	return exitOK
}

// cmdPost handles one PostToolUse invocation, under the same rule as hook: it
// is invoked by Claude Code, it always succeeds, and it writes nothing to
// stdout.
//
// A declaration is a request. This is what closes it: the record that the call
// the agent asked for went on to run.
func cmdPost(args []string, stdin io.Reader, stderr io.Writer) int {
	// Guarded whole, for the reason given in cmdHook.
	_ = safe.Guard(func() error {
		sig := hook.WatchSignals()
		defer sig.Stop()

		st := openForHook(stderr)
		if st == nil {
			return nil
		}
		if standsDown(args, st, stderr) {
			return nil
		}

		p := hook.NewPost(st, time.Now)
		captureErr := safe.Guard(func() error { return p.Capture(stdin) })
		_ = safe.Guard(func() error { p.Close(sig, captureErr); return nil })
		return nil
	})
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
		fmt.Fprintln(stderr, "rashomon: probe expects start or end")
		return exitOK
	}

	// Guarded whole, for the reason given in cmdHook.
	_ = safe.Guard(func() error {
		st := openForHook(stderr)
		if st == nil {
			return nil
		}
		if standsDown(args, st, stderr) {
			return nil
		}
		hook.RunProbe(sig, phase, stdin, st, time.Now)
		return nil
	})
	return exitOK
}

// standDownLine is the whole of what an entry from another install says.
//
// Hook stderr reaches Claude Code's debug log, so nothing derived from the
// invocation goes into it -- not the id that was passed, not the one that was
// expected.
const standDownLine = "rashomon: entry belongs to another install; standing down"

// standsDown reports whether this invocation was installed by another install,
// in which case it must write nothing at all.
//
// Two installs' entries can sit in one settings.json -- neither watch nor
// detach touches the other's -- and Claude Code runs both of them with one
// environment. Both therefore resolve the same store, and without this check
// every tool call is recorded twice. An invocation that names no install is
// from a command line written before this was read, and is handled as it
// always was.
func standsDown(args []string, st *store.Store, stderr io.Writer) bool {
	id := installArg(args)
	if id == "" || id == st.InstallID() {
		return false
	}
	fmt.Fprintln(stderr, standDownLine)
	return true
}

// installArg returns the install id the command line names, or "" for one that
// names none. Anything else in args is ignored: these are the paths Claude
// Code invokes, and there is no argument they are allowed to fail over.
func installArg(args []string) string {
	for i, a := range args {
		if a == install.Marker && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// openForHook opens the store for a hook-invoked path. It prints a fixed
// string and returns nil on failure: there is nowhere to record that recording
// failed, and the absence of any record for the session is the signal.
func openForHook(stderr io.Writer) *store.Store {
	root, err := store.DefaultRoot()
	if err != nil {
		fmt.Fprintln(stderr, "rashomon: store location unresolved")
		return nil
	}
	st, err := store.Open(root)
	if err != nil {
		fmt.Fprintln(stderr, "rashomon: store unavailable")
		return nil
	}
	return st
}

// reportOrEmpty builds the report, or the empty one when nothing has ever been
// recorded here.
//
// "Nothing recorded yet" is not a failure and must not read as one: it is the
// state every user is in exactly once, and it is the state in which they have
// no way to tell a broken tool from one with nothing to say. The empty report
// is the same shape as any other, so a consumer does not have to know whether
// a store exists in order to parse the answer.
// It also returns the per-install key, which --redact needs: the redaction
// digest is an HMAC under it, so a shared report cannot be dictionary-attacked
// by a recipient holding a candidate hostname. A location with no store has no
// key, and also no hosts, so the nil is never used to digest anything.
func reportOrEmpty(sessionID, proxyStore string, now time.Time) (*report.Report, []byte, error) {
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		return report.Empty(now), nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	rep, err := report.Build(st, sessionID, now, report.WithProxyStore(proxyStore))
	if err != nil {
		return nil, nil, err
	}
	return rep, st.Key(), nil
}

// openStoreForRead opens the store without creating one, and reports
// ErrNoStore when there is nothing here.
//
// Every command that only reads or evicts uses this. Creating a store to answer
// a question mints an install identity and a per-install HMAC key, so a user
// who ran `report` or `forget` once would afterwards be told by `status` that a
// store is present -- true, and only because a different command fabricated it.
func openStoreForRead() (*store.Store, error) {
	root, err := store.DefaultRoot()
	if err != nil {
		return nil, err
	}
	return store.OpenExisting(root)
}

// nothingRecordedHere is what an evicting command says when there is no store.
// Not an error: asking to forget something on a machine that has recorded
// nothing is a satisfied request, and exiting non-zero would tell a user their
// privacy action failed when it had nothing to do.
const nothingRecordedHere = "nothing has been recorded here, so there is nothing to forget"

func openStore() (*store.Store, error) {
	root, err := store.DefaultRoot()
	if err != nil {
		return nil, err
	}
	return store.Open(root)
}

// installMetaFile is the store file carrying the install id. Its presence is
// what a plain detach tests, rather than opening the store, because opening
// one creates it: the command that removes an installation must not be the
// command that leaves a fresh store behind on a machine that had none.
const installMetaFile = "install.json"

// storedInstallID reads this machine's install id from a store that is already
// there, and otherwise says what to run instead of creating one.
func storedInstallID() (string, error) {
	root, err := store.DefaultRoot()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(root, installMetaFile)); err != nil {
		return "", fmt.Errorf("no store at %s, so the install id is not known here; run `rashomon detach --install <id>` with the id watch printed, or `rashomon detach --all` to remove every rashomon entry whatever its id", root)
	}
	st, err := openStore()
	if err != nil {
		return "", err
	}
	return st.InstallID(), nil
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

	var foreign []string
	changed, err := editSettings(loc.User, func(doc *settings.Document) (bool, error) {
		applied, err := install.Apply(doc, spec)
		if err != nil {
			return false, err
		}
		foreign, err = install.ForeignOwners(doc, spec.InstallID)
		return applied, err
	})
	if err != nil {
		return err
	}
	if changed {
		fmt.Fprintf(stdout, "rashomon: watching (install %s) via %s\n", st.InstallID(), loc.User)
	} else {
		fmt.Fprintf(stdout, "rashomon: already watching (install %s) via %s\n", st.InstallID(), loc.User)
	}
	// Said here because there is nowhere else it can be said: the foreign
	// entries run under this environment, stand down against this store, and
	// leave nothing behind to notice afterwards.
	if len(foreign) > 0 {
		fmt.Fprintf(stdout, "rashomon: entries from other installs are present (%s); they will fire in this environment and record nothing\n",
			strings.Join(foreign, ", "))
	}
	// Printed in full, on its own line, because it is the line that still works
	// once this binary or the store is gone: the id cannot be read back from a
	// store that is not there, and there is nowhere else it is written down.
	fmt.Fprintln(stdout, "rashomon: undo with this line -- it needs no store, and any rashomon binary will do:")
	fmt.Fprintf(stdout, "rashomon detach --install %s\n", st.InstallID())
	return nil
}

func cmdDetach(args []string, stdout io.Writer) error {
	installID, all, err := detachTarget(args)
	if err != nil {
		return err
	}

	remove := func(doc *settings.Document) (int, error) {
		return install.Remove(doc, install.Spec{InstallID: installID})
	}
	who := "install " + installID
	if all {
		remove = func(doc *settings.Document) (int, error) {
			return install.RemoveIf(doc, func(string) bool { return true })
		}
		who = "every install"
	}

	path, err := settings.UserPath()
	if err != nil {
		return err
	}
	removed := 0
	changed, err := editSettings(path, func(doc *settings.Document) (bool, error) {
		n, err := remove(doc)
		removed = n
		return n > 0, err
	})
	if err != nil {
		return err
	}
	if !changed {
		fmt.Fprintf(stdout, "rashomon: nothing to detach (%s) in %s\n", who, path)
		return nil
	}
	fmt.Fprintf(stdout, "rashomon: detached (%s, %d entries) from %s\n", who, removed, path)
	return nil
}

// detachTarget resolves which entries a detach removes: the install named on
// the command line, every install that left a marker, or -- for a plain detach
// -- the one this machine's store records.
func detachTarget(args []string) (installID string, all bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--install":
			if i+1 >= len(args) {
				return "", false, errors.New("--install needs a value")
			}
			installID = args[i+1]
			i++
		case "--all":
			all = true
		default:
			return "", false, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if all && installID != "" {
		return "", false, errors.New("--install and --all name different sets of entries; pass one or the other")
	}
	if all || installID != "" {
		return installID, all, nil
	}
	installID, err = storedInstallID()
	return installID, false, err
}

// cmdStatus prints what is installed here and what the store holds, and writes
// nothing at all.
//
// Reading the store is the easy way to break that. Opening a store creates one,
// key material and all, so the command whose whole answer may be "nothing is
// installed on this machine" must not be the command that installs something.
// It looks for the store the same way a plain detach does: by the one file that
// is only there if a store is.
func cmdStatus(stdout io.Writer) error {
	root, err := store.DefaultRoot()
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "store: %s\n", root)

	installID := ""
	if _, err := os.Stat(filepath.Join(root, installMetaFile)); err == nil {
		st, err := openStore()
		if err != nil {
			return err
		}
		installID = st.InstallID()
		fmt.Fprintf(stdout, "  present: yes (install %s)\n", installID)
		runs, err := st.Runs()
		if err != nil {
			fmt.Fprintf(stdout, "  runs: %s\n", statusUnknown)
		} else {
			fmt.Fprintf(stdout, "  runs: %d\n", len(runs))
		}
	} else {
		fmt.Fprintln(stdout, "  present: no")
	}

	if err := statusSettings(stdout, installID); err != nil {
		return err
	}
	return statusHooks(stdout)
}

// The words status prints for a thing it could not resolve. A status that
// rendered an unreadable settings file as "absent" would be reporting the one
// state it does not have as the one state users act on.
const (
	statusUnknown    = "unknown"
	statusUnreadable = "unreadable"
)

// statusSettings reports our entry per event, and the other installs sharing
// the file. Without a store there is no install id, and so no entry in the file
// is ours: the ids the entries carry are then all there is to report, and they
// are reported as that rather than as a verdict about ownership.
func statusSettings(stdout io.Writer, installID string) error {
	path, err := settings.UserPath()
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "settings: %s\n", path)

	label := "other installs"
	if installID == "" {
		label = "install ids present"
	}

	doc, err := settings.Load(path)
	if err != nil {
		for _, event := range install.Events {
			fmt.Fprintf(stdout, "  %s: %s\n", event, statusUnreadable)
		}
		fmt.Fprintf(stdout, "  %s: %s\n", label, statusUnreadable)
		return nil
	}

	if installID == "" {
		fmt.Fprintln(stdout, "  no store here, so no install id is ours and no entry can be called ours")
	}
	for _, event := range install.Events {
		fmt.Fprintf(stdout, "  %s: %s\n", event, entryState(doc, installID, event))
	}

	others, err := install.ForeignOwners(doc, installID)
	switch {
	case err != nil:
		fmt.Fprintf(stdout, "  %s: %s\n", label, statusUnreadable)
	case len(others) == 0:
		fmt.Fprintf(stdout, "  %s: none\n", label)
	default:
		fmt.Fprintf(stdout, "  %s: %s\n", label, strings.Join(others, ", "))
	}
	return nil
}

// entryState is the word for our entry under one event: present as watch
// installs it, absent, or a file that could not be read.
func entryState(doc *settings.Document, installID, event string) string {
	if installID == "" {
		// Present compares against an id, and "" is not one: an entry that
		// belongs to no install carries "" too and would match it.
		return statusUnknown
	}
	switch present, err := install.Present(doc, installID, event); {
	case err != nil:
		return statusUnreadable
	case present:
		return "present"
	default:
		return "absent"
	}
}

// statusHooks reports whether an installed entry would run at all, resolved for
// the working directory: the project and local layers are part of that answer
// and they are per-directory.
func statusHooks(stdout io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	loc, err := settings.DefaultLocations(cwd)
	if err != nil {
		return err
	}
	decision, err := settings.HooksDisabled(loc)
	switch {
	case err != nil:
		fmt.Fprintf(stdout, "hooks: %s; a settings layer could not be read\n", statusUnknown)
	case decision.Disabled:
		fmt.Fprintf(stdout, "hooks: disabled by the %s settings layer\n", decision.Layer)
	case decision.Layer != "":
		fmt.Fprintf(stdout, "hooks: enabled by the %s settings layer\n", decision.Layer)
	default:
		fmt.Fprintln(stdout, "hooks: enabled; no settings layer sets disableAllHooks")
	}
	fmt.Fprintf(stdout, "  resolved for %s\n", cwd)
	return nil
}

func cmdReport(args []string, stdout io.Writer) error {
	sessionID := ""
	asJSON := false
	redact := false
	chain := false
	proxyStore := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session":
			if i+1 >= len(args) {
				return errors.New("--session needs a value")
			}
			sessionID = args[i+1]
			i++
		case "--proxy-store":
			if i+1 >= len(args) {
				return errors.New("--proxy-store needs a value")
			}
			proxyStore = args[i+1]
			i++
		case "--json":
			asJSON = true
		case "--redact":
			redact = true
		case "--chain":
			chain = true
		default:
			return fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if proxyStore == "" {
		proxyStore = defaultProxyStore()
	}

	// Opened WITHOUT creating: a command that only asks a question must not mint
	// an install identity and an HMAC key as a side effect of being asked. The
	// same rule status follows, and for the same reason.
	rep, key, err := reportOrEmpty(sessionID, proxyStore, time.Now())
	if err != nil {
		return err
	}
	if redact {
		// Applied to the whole report before either renderer sees it, so the
		// two forms cannot disagree about what was hidden and a caller cannot
		// render the plain form from the same value by mistake.
		rep = report.Redact(rep, key)
	}
	if !asJSON {
		// --chain expands the text listing only. The JSON carries the whole
		// structure either way: that reader is a program selecting fields, not
		// a person scrolling, and making it pass a flag to receive a section
		// would mean a consumer could parse a report and silently miss one.
		var opts []report.TextOption
		if chain {
			opts = append(opts, report.WithChain())
		}
		return report.Text(stdout, rep, opts...)
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// cmdForget evicts records at one end of the store's timeline.
//
// --since is the privacy form: forget what just happened. --before is the
// retention form: forget what is old. They are the two open ends of one window
// and one code path, and naming both would name an empty intersection, so
// naming both is refused rather than resolved.
func cmdForget(args []string, stdout io.Writer) error {
	var from, to *time.Time
	host := ""
	for i := 0; i < len(args); i++ {
		switch flag := args[i]; flag {
		case "--host":
			if i+1 >= len(args) {
				return errors.New("--host needs a value")
			}
			host = args[i+1]
			i++
		case "--since", "--before":
			if i+1 >= len(args) {
				return fmt.Errorf("%s needs a value", flag)
			}
			t, err := parseInstant(flag, args[i+1], time.Now())
			if err != nil {
				return err
			}
			if flag == "--since" {
				from = &t
			} else {
				to = &t
			}
			i++
		default:
			return fmt.Errorf("unknown argument %q", args[i])
		}
	}
	switch {
	case host != "" && (from != nil || to != nil):
		return errors.New("--host removes by destination and --since/--before remove by time; pass one kind or the other")
	case host != "":
		return forgetHost(host, stdout)
	case from != nil && to != nil:
		return errors.New("--since and --before name opposite ends of the store's timeline; pass one or the other")
	case from == nil && to == nil:
		return errors.New("forget needs --since or --before <RFC3339 time or duration such as 24h>, or --host <hostname>")
	}

	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		fmt.Fprintln(stdout, nothingRecordedHere)
		return nil
	}
	if err != nil {
		return err
	}
	gaps, err := st.ForgetWindow(from, to, time.Now())
	if err != nil {
		return err
	}
	total := 0
	for _, g := range gaps {
		total += g.RemovedRecords
	}
	// Exactly one bound was named; the refusal above is what makes that true.
	var bound string
	if from != nil {
		bound = "since " + from.UTC().Format(time.RFC3339)
	} else {
		bound = "before " + to.UTC().Format(time.RFC3339)
	}
	fmt.Fprintf(stdout, "rashomon: forgot %d records across %d runs %s; %d gap records written\n",
		total, len(gaps), bound, len(gaps))
	return nil
}

// parseInstant accepts a duration ("24h", meaning that long ago) or an RFC3339
// timestamp.
func parseInstant(flag, v string, now time.Time) (time.Time, error) {
	if d, err := time.ParseDuration(v); err == nil {
		return now.Add(-d), nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%s %q is neither a duration nor an RFC3339 time", flag, v)
}

func usage(w io.Writer) {
	fmt.Fprint(w, `rashomon -- record what a Claude Code session asked to run

usage:
  rashomon watch                 install the PreToolUse, PostToolUse and
                               PostToolUseFailure recorders and the
                               SessionStart/SessionEnd liveness probe
  rashomon detach                remove them, leaving everything else as found
  rashomon detach --install <id> remove one install's entries, reading no store
  rashomon detach --all          remove every entry carrying a rashomon install
                               marker, whatever its id
  rashomon status                say what is installed and what the store holds,
                               writing nothing and creating no store
  rashomon report [--session S] [--json] [--redact] [--chain]
                  [--proxy-store PATH]
                               render declarations and coverage, as text for a
                               terminal or as JSON for a consumer; --chain
                               lists the calls under each prompt, which JSON
                               always carries
  rashomon forget --host H       evict every call that named host H, and its
                               baseline entry
  rashomon forget --since T      evict records recorded at or after T
  rashomon forget --before T     evict records recorded before T
                               either way, leaving a coverage gap behind
  rashomon env [--port N]        print the proxy variables to export, for use
                               with eval; HTTPS only, since plain HTTP is not
                               observed in this release
  rashomon run [--proxy-status PATH] -- <cmd...>
                               run a command with the proxy variables set, if
                               and only if an observe-mode proxy is running,
                               then report on the session it produced;
                               --proxy-status overrides where that is checked
  rashomon version               print the version

invoked by Claude Code, never by hand:
  rashomon hook [--install ID]   handle one PreToolUse invocation
  rashomon post [--install ID]   handle one PostToolUse invocation
  rashomon probe start|end [--install ID]
                               handle SessionStart / SessionEnd

--install names the install whose entry is running. An entry belonging to
another install stands down: it records nothing in this environment.
`)
}

// observeDefaultPort is the observe profile's listener.
//
// 18080 rather than 8080: the enforcing profile holds 8080, both are loopback
// installs on one host, and 8080 is the most commonly occupied port on a
// developer machine. The proxy prints the same number in its own banner.
const observeDefaultPort = 18080

// noProxyValue is what NO_PROXY is set to.
//
// Loopback and *.local are excluded because proxying them breaks local
// development tooling for no observational gain. Private ranges are
// deliberately NOT excluded: an agent reaching an internal service is exactly
// the finding this tool exists to surface, so LAN egress stays on the path.
const noProxyValue = "localhost,127.0.0.1,::1,0.0.0.0,*.local"

// cmdEnv prints the variables that put the observe proxy on a session's path.
//
// It prints rather than exports, because a process cannot alter its parent's
// environment; the operator runs `eval $(rashomon env)`. That is why nothing
// but assignments may reach stdout — one line of prose and eval tries to run
// it as a command — and why the explanatory text goes to no stream at all
// rather than being commented into the output.
//
// HTTPS only. Plain HTTP is not observed in this release, so exporting
// HTTP_PROXY would route traffic through a proxy that does not record it and
// then report nothing: silence read as zero, the one thing the coverage rules
// forbid. The lowercase form is not a duplicate for tidiness either — curl
// reads only lowercase https_proxy, and curl is the first thing anyone tests
// with.
//
// It reads no store and creates none. This is the first command an operator
// runs, and making it depend on having already recorded something would be
// backwards.
func cmdEnv(args []string, stdout io.Writer) error {
	port, err := envPort(args)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("http://127.0.0.1:%d", port)
	fmt.Fprintf(stdout, "export HTTPS_PROXY=%s\n", addr)
	fmt.Fprintf(stdout, "export https_proxy=%s\n", addr)
	fmt.Fprintf(stdout, "export NO_PROXY=%s\n", noProxyValue)
	return nil
}

// envPort resolves --port, refusing anything that is not a usable port.
//
// Refusing rather than falling back to the default is the whole point: a
// silently ignored --port sends the operator's traffic to whatever is
// listening on 18080, which may be another person's proxy or nothing at all,
// and they would have no way to tell from the output that their flag was
// dropped.
func envPort(args []string) (int, error) {
	port := observeDefaultPort
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--port":
			if i+1 >= len(args) {
				return 0, errors.New("env: --port needs a value")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return 0, fmt.Errorf("env: --port %q is not a number", args[i])
			}
			if n < 1 || n > 65535 {
				return 0, fmt.Errorf("env: --port %d is outside 1-65535", n)
			}
			port = n
		case "--help", "-h":
			return 0, errors.New("env: prints the proxy variables to export; --port N selects the listener (default 18080)")
		default:
			return 0, fmt.Errorf("env: unknown argument %q", args[i])
		}
	}
	return port, nil
}

// defaultProxyStore is where the observe-mode proxy writes its records.
//
// The path is a contract between two programs that ship separately: the
// observe-mode proxy writes causal.db under ~/.altrace/observe. Hard-coding it
// here rather
// than reading the proxy's config is deliberate -- this tool must not need to
// parse the closed product's configuration to do its job, and --proxy-store
// covers every operator who moved it.
//
// A home directory that cannot be resolved yields "", which the reader reports
// as "not observed (no_proxy_store)" rather than guessing at a relative path.
func defaultProxyStore() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".altrace", "observe", "causal.db")
}

// forgetHost removes every call that named a host, and the host's entry in the
// project baseline.
//
// Both halves are required for the operation to mean anything. Removing the
// records while the baseline still remembers the host would leave it suppressed
// as "seen before" forever, with nothing left to explain why -- a deletion that
// silently changes future reports is worse than no deletion.
//
// It does NOT delete from the proxy's store, and says so. That database is the
// closed product's hash-chained audit record, opened read-only here, and
// removing a row would break the chain it exists to provide. The gap record
// carries the host, and the report reads it to keep the destination suppressed
// from its view.
func forgetHost(host string, stdout io.Writer) error {
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		fmt.Fprintln(stdout, nothingRecordedHere)
		return nil
	}
	if err != nil {
		return err
	}
	gaps, err := st.ForgetHost(host, time.Now())
	if err != nil {
		return err
	}
	total := 0
	for _, g := range gaps {
		total += g.RemovedRecords
	}

	// The baseline is keyed per project, and a forget is not told which project
	// the caller meant, so every project that remembers the host loses it. That
	// is the conservative direction: a host the user asked to forget must not
	// survive in a baseline they did not think to name.
	cleared, err := clearBaselines(st.Root(), host)
	if err != nil {
		return err
	}

	fmt.Fprintf(stdout, "rashomon: forgot %d records naming %s across %d runs; "+
		"%d gap records written; %d project baseline(s) cleared\n",
		total, host, len(gaps), len(gaps), cleared)
	fmt.Fprintln(stdout, "rashomon: the proxy's own records are not ours to delete "+
		"(they are a hash-chained audit store, opened read-only), so the report "+
		"suppresses this destination from its view rather than claiming the row is gone")
	return nil
}

// clearBaselines removes a host from every project baseline in the store.
func clearBaselines(root, host string) (int, error) {
	dir := filepath.Join(root, "baseline")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var cleared int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		n, err := baseline.ForgetFile(filepath.Join(dir, e.Name()), host)
		if err != nil {
			return cleared, err
		}
		cleared += n
	}
	return cleared, nil
}

// cmdRun runs the user's command with the proxy variables set, when it is safe
// to, and reports on the session afterwards.
//
// The whole value of this command is that it makes the SAFE thing the easy
// thing. Exporting the variables by hand works, and gets you a broken session
// the day the proxy is in enforce mode or has crashed -- because an
// enforce-mode proxy refuses the client's own API tunnels, so the failure is
// not "no destinations recorded" but "the agent cannot reach the API at all".
// This checks first, and launches WITHOUT the variables when the check fails
// rather than refusing to launch: the user asked to run their command, and a
// wrapper that declined because a status file was missing would be worse than
// one that runs without recording.
//
// It returns the child's exit code. The report is rendered after the child
// exits, which is why the launcher spawns and waits rather than exec-replacing
// this process -- there would otherwise be nothing left to render it.
func cmdRun(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	statusPath := posture.DefaultPath()
	var argv []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--":
			argv = args[i+1:]
			i = len(args)
		case "--proxy-status":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "rashomon: --proxy-status needs a value")
				return exitFail
			}
			statusPath = args[i+1]
			i++
		case "--help", "-h":
			fmt.Fprintln(stdout, "Usage: rashomon run [--proxy-status PATH] -- <command> [args...]")
			return exitOK
		default:
			fmt.Fprintf(stderr, "rashomon: unknown argument %q (the command goes after --)\n", args[i])
			return exitFail
		}
	}
	if len(argv) == 0 {
		fmt.Fprintln(stderr, "rashomon: run needs a command after --, for example: rashomon run -- claude")
		return exitFail
	}

	// Refuse before launching if nothing is recording. Unlike a missing proxy,
	// this one is worth stopping for: without the hooks installed there will be
	// no session to report on, so the command would run, finish, and produce
	// nothing -- and the user would reasonably conclude the tool does not work.
	if !storeInstalled() {
		fmt.Fprintln(stderr, "rashomon: nothing is recording -- run `rashomon watch` first, "+
			"then `rashomon run -- <command>`")
		return exitFail
	}

	v := posture.Read(statusPath)
	var env []string
	if v.Export {
		env = launch.Env(v.File.ListenAddr)
		fmt.Fprintf(stderr, "rashomon: %s; destinations will be recorded\n", v.Reason)
	} else {
		// One line, on stderr, saying why. This is the difference between a
		// session that quietly records nothing and one the user knows records
		// nothing.
		fmt.Fprintf(stderr, "rashomon: running WITHOUT proxy variables -- %s\n", v.Reason)
	}

	// Taken before the child starts, so the report can tell a session the
	// child produced from one that was already in the store.
	startedAt := time.Now()

	code, runErr := launch.Run(argv, env, stdin, stdout, stderr)
	if runErr != nil {
		fmt.Fprintln(stderr, "rashomon:", runErr)
		return code
	}

	// The report follows the child, on stderr's side of the conversation: the
	// child's own stdout is the user's output and must not have a report
	// appended to it, or piping the command anywhere would corrupt the pipe.
	if err := reportNewest(stderr, v, startedAt); err != nil {
		fmt.Fprintln(stderr, "rashomon: the session report could not be rendered:", err)
	}
	return code
}

// reportNewest renders the session the child produced, if it produced one.
//
// The newest run rather than a named session, because the session id is Claude
// Code's and this process never sees it: the hooks record it, and the only
// thing this side knows is that whatever ran last is what just finished.
//
// since is when the child was launched, and the comparison against it is the
// whole point. Without it, a command that records nothing -- anything that is
// not a Claude Code session, or a session whose hooks never fired -- printed a
// full report for whatever happened to be newest in the store, from minutes or
// days earlier, under a heading that reads as though the command just produced
// it. That is this program's own discipline broken: a claim it has no evidence
// for, and the one failure it exists to make impossible.
//
// The residual limit, stated rather than papered over: a DIFFERENT session
// writing concurrently with the child can still be the newest when the child
// exits. Nothing this side sees can separate those two, because the session id
// belongs to Claude Code and never reaches this process.
func reportNewest(w io.Writer, v posture.Verdict, since time.Time) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	newest, writtenAt, err := st.NewestRun()
	if err != nil {
		return err
	}
	if newest == "" || writtenAt.Before(since) {
		fmt.Fprintln(w, "rashomon: no session was recorded for that command")
		return nil
	}

	opts := []report.Option{}
	if v.File.CausalDB != "" {
		// The proxy told us where it writes, so the report does not have to
		// guess at a storage layout it should not know.
		opts = append(opts, report.WithProxyStore(v.File.CausalDB))
	} else {
		opts = append(opts, report.WithProxyStore(defaultProxyStore()))
	}

	rep, err := report.Build(st, newest, time.Now(), opts...)
	if err != nil {
		return err
	}
	fmt.Fprintln(w)
	return report.Text(w, rep)
}

// storeInstalled reports whether watch has ever run, by the presence of the
// install marker.
//
// The marker rather than opening the store, for the same reason a plain detach
// tests it: opening the store CREATES one, and a command that asks "has this
// been set up" must not set it up as a side effect of asking.
func storeInstalled() bool {
	root, err := store.DefaultRoot()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(root, installMetaFile))
	return err == nil
}
