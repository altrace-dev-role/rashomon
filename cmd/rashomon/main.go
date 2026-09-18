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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
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
	sig := hook.WatchSignals()
	defer sig.Stop()

	st := openForHook(stderr)
	if st == nil {
		return exitOK
	}
	if standsDown(args, st, stderr) {
		return exitOK
	}

	h := hook.New(st, time.Now)
	captureErr := safe.Guard(func() error { return h.Capture(stdin) })
	_ = safe.Guard(func() error { h.Close(sig, captureErr); return nil })
	return exitOK
}

// cmdPost handles one PostToolUse invocation, under the same rule as hook: it
// is invoked by Claude Code, it always succeeds, and it writes nothing to
// stdout.
//
// A declaration is a request. This is what closes it: the record that the call
// the agent asked for went on to run.
func cmdPost(args []string, stdin io.Reader, stderr io.Writer) int {
	sig := hook.WatchSignals()
	defer sig.Stop()

	st := openForHook(stderr)
	if st == nil {
		return exitOK
	}
	if standsDown(args, st, stderr) {
		return exitOK
	}

	p := hook.NewPost(st, time.Now)
	captureErr := safe.Guard(func() error { return p.Capture(stdin) })
	_ = safe.Guard(func() error { p.Close(sig, captureErr); return nil })
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

	st := openForHook(stderr)
	if st == nil {
		return exitOK
	}
	if standsDown(args, st, stderr) {
		return exitOK
	}
	_ = safe.Guard(func() error { hook.RunProbe(sig, phase, stdin, st, time.Now); return nil })
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
		default:
			return fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if proxyStore == "" {
		proxyStore = defaultProxyStore()
	}

	st, err := openStore()
	if err != nil {
		return err
	}
	rep, err := report.Build(st, sessionID, time.Now(), report.WithProxyStore(proxyStore))
	if err != nil {
		return err
	}
	if !asJSON {
		return report.Text(stdout, rep)
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
	for i := 0; i < len(args); i++ {
		switch flag := args[i]; flag {
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
	case from != nil && to != nil:
		return errors.New("--since and --before name opposite ends of the store's timeline; pass one or the other")
	case from == nil && to == nil:
		return errors.New("forget needs --since or --before <RFC3339 time or duration such as 24h>")
	}

	st, err := openStore()
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
  rashomon watch                 install the PreToolUse and PostToolUse
                               recorders and the SessionStart/SessionEnd
                               liveness probe
  rashomon detach                remove them, leaving everything else as found
  rashomon detach --install <id> remove one install's entries, reading no store
  rashomon detach --all          remove every entry carrying a rashomon install
                               marker, whatever its id
  rashomon status                say what is installed and what the store holds,
                               writing nothing and creating no store
  rashomon report [--session S] [--json] [--proxy-store PATH]
                               render declarations and coverage, as text for a
                               terminal or as JSON for a consumer
  rashomon forget --since T      evict records recorded at or after T
  rashomon forget --before T     evict records recorded before T
                               either way, leaving a coverage gap behind
  rashomon env [--port N]        print the proxy variables to export, for use
                               with eval; HTTPS only, since plain HTTP is not
                               observed in this release
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
// The path is a contract between two programs that ship separately: the proxy
// derives <data dir>/causal.db from server.storage.root, and its observe
// profile sets that root to ~/.altrace/observe. Hard-coding it here rather
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
