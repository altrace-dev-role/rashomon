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
	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/fault"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/host"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/safe"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/spend"
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
	case "recap":
		// Not under guarded(): guarded exits 1 and writes to stderr on error,
		// which a hook renders as a visible "hook error" -- fine for a command
		// a person typed, wrong for the one entry that speaks only when it has
		// a finding. cmdRecap follows hook/post/probe's own rule instead and
		// always returns exitOK; see its comment.
		return cmdRecap(rest, stdin, stdout, stderr)

	case "watch":
		return guarded(stderr, noArgs(rest, stdout, cmdWatch))
	case "detach":
		return guarded(stderr, func() error { return cmdDetach(rest, stdout) })
	case "pause":
		return guarded(stderr, noArgs(rest, stdout, cmdPause))
	case "resume":
		return guarded(stderr, noArgs(rest, stdout, cmdResume))
	case "status":
		return guarded(stderr, noArgs(rest, stdout, cmdStatus))
	case "report":
		return guarded(stderr, func() error { return cmdReport(rest, stdout) })
	case "digest":
		return guarded(stderr, func() error { return cmdDigest(rest, stdin, stdout) })
	case "forget":
		return guarded(stderr, func() error { return cmdForget(rest, stdout) })
	case "spend":
		return guarded(stderr, func() error { return cmdSpend(rest, stdout) })

	// An explicit request for the version or the usage is a successful
	// command, so it goes to stdout and exits 0. Only a command we did not
	// understand is a usage ERROR, on stderr and non-zero.
	//
	// All six spellings, because `--version` is the first thing a packager
	// tries and `--help` is the first thing a person tries, and both landed
	// in `default:` -- usage on stderr, exit 1 -- while only the bare
	// `version` subcommand worked.
	//
	// version refuses anything after it, as every command in the README's
	// table refuses an argument it does not know (see noArgs); the hook entry
	// points are not in it, because they must always exit 0. help is the one
	// that ignores what follows: whatever it is, the usage is the answer to it.
	case "version", "--version", "-v":
		return guarded(stderr, noArgs(rest, stdout, printVersion))

	case "help", "--help", "-h":
		usage(stdout)
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

// noArgs adapts a command that takes no arguments to guarded, refusing any it
// is given before the command runs.
//
// watch, pause, resume, status and version once ignored whatever followed
// them, so the first thing a person types to learn what one does ran it:
// `watch --help` wrote eight hook entries and minted a store, `pause --help`
// paused recording, `resume --help` lifted the pause. A command does not act
// on an argument it does not understand. detach, report, digest, forget and
// spend already refused an unknown argument, and this is the same refusal in
// the same words, with the same exit 1. --help is not special: no command in
// the README's table takes it in this release (env and run, which the usage
// omits, keep their own), and `rashomon --help` is where the usage lives.
func noArgs(args []string, stdout io.Writer, cmd func(io.Writer) error) func() error {
	return func() error {
		if len(args) > 0 {
			return fmt.Errorf("unknown argument %q", args[0])
		}
		return cmd(stdout)
	}
}

// printVersion is version's whole body, in the shape noArgs takes.
func printVersion(stdout io.Writer) error {
	fmt.Fprintln(stdout, version)
	return nil
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
//
// checkPaused runs before anything else in the body, including openForHook:
// see its own comment for why the order is load-bearing (Part 2, H-81).
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

		if checkPaused(args, stdin, store.PhaseCall, stderr) {
			return nil
		}

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

		if checkPaused(args, stdin, store.PhasePost, stderr) {
			return nil
		}

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
		if checkPaused(args, stdin, phase, stderr) {
			return nil
		}
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

// recapStdinTimeout bounds how long cmdRecap waits for the Stop/StopFailure
// payload on stdin before giving up and proceeding as though none arrived.
//
// Not a flag, and not IsTerminal-based: H-104 fixed digest's own hang by
// requiring an explicit --stdin flag from the caller -- a promise that the
// pipe will close, which is fine for a command a person or a script invokes
// deliberately. This entry has no such caller-supplied promise to lean on:
// Claude Code always writes the Stop payload on stdin and this process
// cannot ask it to close the pipe, so unconditionally reading with a bounded
// TIME budget is the only option that is both always-correct on the healthy
// path and incapable of wedging the turn on a broken one. Three seconds
// leaves a wide margin inside the entry's own 10-second timeout
// (install.RecapTimeout) even after the ~120ms this command's own work can
// cost at several thousand records (the spec's Measurements taken).
const recapStdinTimeout = 3 * time.Second

// recapPayload is the slice of the Stop/StopFailure hook payload this command
// reads. session_id is Claude Code's own, not a tool call's or a
// transcript's, and last_assistant_message is handed straight to
// digest.Build exactly as a --stdin cmdDigest invocation would -- never
// opened as a file, never echoed back (see internal/digest's package doc and
// report.AccountFromMessage). This command has its own reader
// (readRecapPayload) rather than reusing cmdDigest's readStdinLastMessage:
// that function is only ever safe to call behind --stdin's promise, and
// this entry has no equivalent flag to gate on -- see recapStdinTimeout.
type recapPayload struct {
	SessionID            string `json:"session_id"`
	LastAssistantMessage string `json:"last_assistant_message"`
}

// readRecapPayload reads and parses stdin within a fixed budget, whatever
// stdin is and whatever it does.
//
// safe.Go, not a bare `go`: a panic on this goroutine cannot be recovered by
// its caller (internal/safe's own doc), and this command's entire contract
// is that it never produces a non-zero exit -- see cmdRecap. The result
// travels over a buffered channel rather than a variable the goroutine
// writes into directly, so there is nothing left to race once the timeout
// fires, this function has returned, and the abandoned goroutine is still
// blocked in Read: it can only ever send into a channel nothing is
// receiving from any more, once, and then it is done.
func readRecapPayload(in io.Reader, timeout time.Duration) recapPayload {
	type parsed struct {
		payload recapPayload
		ok      bool
	}
	ch := make(chan parsed, 1)
	safe.Go(func() {
		raw, err := io.ReadAll(io.LimitReader(in, hook.MaxPayloadBytes+1))
		if err != nil || len(raw) == 0 || len(raw) > hook.MaxPayloadBytes {
			ch <- parsed{}
			return
		}
		var p recapPayload
		if json.Unmarshal(raw, &p) != nil {
			ch <- parsed{}
			return
		}
		ch <- parsed{payload: p, ok: true}
	}, nil)

	select {
	case r := <-ch:
		return r.payload
	case <-time.After(timeout):
		return recapPayload{}
	}
}

// recapPending reports whether this recap invocation is the UserPromptSubmit
// catch-up. A settings entry passes `pending --install <id>` and the plugin
// passes `pending`, so the word is looked for, not its position.
func recapPending(args []string) bool {
	for _, a := range args {
		if a == "pending" {
			return true
		}
	}
	return false
}

// cmdRecap handles one Stop or StopFailure invocation: read-only, printing
// at most one line, and never producing a hook error.
//
// It departs from hook/post/probe in one way beyond dispatch: those three
// exist to record and their contract is "never exit 2, whatever else goes
// wrong". This entry's contract is narrower still -- "never exit non-zero,
// and print nothing on any internal failure" -- because under exception-only
// notification a broken recap would be the only thing on screen, every turn
// (spec, "Mechanism, and the failure rule"). guarded() is therefore never
// used here: it exits 1 and writes to stderr, which is right for a command a
// person typed and wrong for this one.
func cmdRecap(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	_ = safe.Guard(func() error {
		fault.Inject(fault.PointRecapStart)

		now := time.Now()
		payload := readRecapPayload(stdin, recapStdinTimeout)

		root, err := store.DefaultRoot()
		if err != nil {
			return nil
		}

		st, err := openStoreForRead()
		if errors.Is(err, store.ErrNoStore) {
			// A Stop firing before this machine has ever opened a store
			// cannot happen honestly -- SessionStart's own probe already
			// opens one -- and minting recap's bookkeeping file here to say
			// so would be the exact "creating something to say nothing
			// happened" mistake pause's own rule (Part 2) refuses. There is
			// also, genuinely, nothing to record a failure against: no
			// install.json means storeExists(root) is false and Claim/
			// RecordFailure are already a no-op against it.
			return nil
		}
		if err != nil {
			// A store that exists but could not be opened -- a malformed
			// install.key or install.json -- is exactly H-93's second case.
			// Unlike ErrNoStore, root DOES already hold a store here, so the
			// failure is recorded where status can show it.
			recap.RecordFailure(root, now)
			return nil
		}
		if standsDown(args, st, stderr) {
			return nil
		}

		sessionID := payload.SessionID
		if sessionID == "" {
			// Mirrors digestOrEmpty's own fallback: this process never learns
			// Claude Code's session id any other way than being told it.
			newest, _, nerr := st.NewestRun()
			if nerr != nil {
				recap.RecordFailure(root, now)
				return nil
			}
			if newest == "" {
				return nil
			}
			sessionID = newest
		}

		// `recap pending` runs on UserPromptSubmit, before the new prompt has
		// made a single call, so the newest turn in the store is the one that
		// just ended. It speaks only when no recap reached that turn: saying
		// No at a permission prompt interrupts the turn and Claude Code fires
		// no Stop after an interrupt, so without this a refused call -- the
		// case the line exists for -- could never be shown at all.
		pending := recapPending(args)
		lastMessage := payload.LastAssistantMessage
		if pending {
			// An interrupted turn has no final message, and this payload's
			// prompt field is the NEW prompt's text, never read here.
			lastMessage = ""
		}
		d, derr := digest.Build(st, sessionID, "", lastMessage, now)
		if derr != nil {
			recap.RecordFailure(root, now)
			return nil
		}
		if pending && !recap.Pending(root, d.SessionID, d.PromptID) {
			return nil
		}

		// Which origin is running decides which command the line can point
		// at. The invocation says so first: only a settings entry's command
		// line carries the --install marker (install.Spec.Command appends it;
		// the plugin's hooks.json passes plain ["recap"]), so a marked
		// invocation is the settings origin even when the binary it names
		// sits inside a plugin directory -- which is exactly where a settings
		// install made with the plugin's own binary puts it, and asking only
		// where the executable lives would send that user to a slash command
		// no loaded plugin provides. This is the same precedence coverage's
		// Resolve gives the settings entry over PluginPresent. Only an
		// unmarked invocation asks where it is running from; an error there
		// reads as "not the plugin", which points at the CLI: the settings
		// install's command, and the only one that exists without a plugin.
		fromPlugin := false
		if installArg(args) == "" {
			if present, perr := install.PluginPresent(install.EventStop); perr == nil {
				fromPlugin = present
			}
		}
		line, wantSpeak := recap.Line(d, d.SessionID, fromPlugin)
		// Claim's own error is intentionally ignored: recap.json is
		// bookkeeping, not evidence, and a failure to persist it must cost at
		// most a future duplicate or a stale health timestamp, never a
		// missing line and never a hook error (see Claim's doc).
		speak, _ := recap.Claim(root, d.SessionID, d.PromptID, now, wantSpeak)
		if !speak {
			return nil
		}
		if pending {
			line = recap.PreviousTurn(line)
		}

		b, merr := json.Marshal(map[string]string{"systemMessage": line})
		if merr != nil {
			return nil
		}
		// The human channel: systemMessage in JSON on stdout, capped at
		// 10,000 characters by Claude Code itself. Plain stdout on a hook
		// goes to the debug log, which is why hook/post above write nothing
		// there at all -- this is the one entry that has something for a
		// human to read, and this JSON envelope is how it reaches them.
		_, _ = stdout.Write(b)
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

// checkPaused is the recording-state gate shared by cmdHook, cmdPost and
// cmdProbe (Part 2). It answers "is this invocation paused" by stating one
// small file directly -- never by opening the store, which is what
// openForHook does as a side effect of doing its job. A paused machine that
// has never recorded must not acquire an install identity merely by being
// asked whether it is paused; storeExistsAt, a few hundred lines down, stats
// the same kind of marker for the identical reason, and this is why the call
// sites put it before openForHook rather than after.
//
// Where a store already exists it writes exactly one coverage record,
// carrying store.ReasonRecordingPaused, so a reader of the report sees a
// deliberate gap and not an unexplained one (H-80). Where none exists it
// writes nothing at all (H-81): creating a store to say "not recording" would
// be worse than saying nothing, and it is the one outcome pausing before ever
// recording must not produce. standsDown is still honoured on the writing
// path, so a foreign or plugin-owned entry sharing this store does not double
// the paused coverage record the way it would double a declaration.
//
// It returns whether the invocation was handled. The caller's only job on
// true is to return: there is nothing else honest left to do with a call this
// program was told not to look at.
func checkPaused(args []string, stdin io.Reader, phase string, stderr io.Writer) bool {
	root, err := store.DefaultRoot()
	if err != nil {
		return false
	}
	paused, _, err := store.Paused(root)
	if err != nil || !paused {
		return false
	}
	if !storeExistsAt(root) {
		return true
	}
	st, err := store.OpenExisting(root)
	if err != nil {
		return true
	}
	if standsDown(args, st, stderr) {
		return true
	}
	hook.RecordPaused(stdin, phase, st, time.Now)
	return true
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
func reportOrEmpty(sessionID, nonoTrail string, now time.Time) (*report.Report, []byte, error) {
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		return report.Empty(now), nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	opts := []report.Option{report.WithNonoTrail(nonoTrail)}
	key := st.Key()

	rep, err := report.Build(st, sessionID, now, opts...)
	if err != nil {
		return nil, nil, err
	}
	return rep, key, nil
}

// storeKey is the install key, or nil when there is no store to read it from.
func storeKey() []byte {
	st, err := openStoreForRead()
	if err != nil {
		return nil
	}
	return st.Key()
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

	// A live plugin refuses a settings install, the same shape as the refusal
	// above: proceeding would produce a configuration that does not work.
	// standsDown cannot save this arrangement -- it stands down on an
	// install-id MISMATCH, and a plugin invocation names no id to mismatch
	// against, so both origins would record every tool call.
	userDoc, err := settings.Load(loc.User)
	if err != nil {
		return err
	}
	if p, err := install.FindPlugin(userDoc); err != nil {
		return err
	} else if p != nil && p.Enabled && len(p.Events) > 0 {
		return fmt.Errorf("the %s plugin is enabled and already provides these hook entries (%s); "+
			"installing here would record every tool call twice -- run `/plugin disable %s` first, "+
			"or skip watch and keep using the plugin", p.Key, strings.Join(p.Events, ", "), p.Key)
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
	installID, all, force, err := detachTarget(args)
	if err != nil {
		return err
	}

	remove := func(doc *settings.Document) (int, []install.Modified, error) {
		return install.Remove(doc, install.Spec{InstallID: installID}, force)
	}
	who := "install " + installID
	if all {
		remove = func(doc *settings.Document) (int, []install.Modified, error) {
			return install.RemoveIf(doc, func(string) bool { return true }, force)
		}
		who = "every install"
	}

	path, err := settings.UserPath()
	if err != nil {
		return err
	}
	removed := 0
	var left []install.Modified
	changed, err := editSettings(path, func(doc *settings.Document) (bool, error) {
		n, l, err := remove(doc)
		removed, left = n, l
		return n > 0, err
	})
	if err != nil {
		return err
	}
	// Named before the outcome, because an entry left behind still fires on
	// every tool call and that is the part the user has to act on.
	for _, m := range left {
		fmt.Fprintf(stdout, "rashomon: removed the %s entry you had edited (%s), because --force was given\n", m.Event, m.Detail)
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
func detachTarget(args []string) (installID string, all, force bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--install":
			if i+1 >= len(args) {
				return "", false, false, errors.New("--install needs a value")
			}
			installID = args[i+1]
			i++
		case "--all":
			all = true
		case "--force":
			// The way out of an entry someone edited. Without it, one edited
			// value left every entry installed and no supported way to
			// remove them.
			force = true
		default:
			return "", false, false, fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if all && installID != "" {
		return "", false, false, errors.New("--install and --all name different sets of entries; pass one or the other")
	}
	if all || installID != "" {
		return installID, all, force, nil
	}
	installID, err = storedInstallID()
	return installID, false, force, err
}

// cmdPause engages recording state (Part 2). See checkPaused for what a
// paused hook invocation does, and the threat model in store.Pause's own
// comment for why this is a file under the store root rather than an
// environment variable or a settings edit: recording state can be changed by
// anyone who can run as this user, including the audited agent via Bash, and
// no mechanism at this layer can stop that. What it can do is leave every
// change visible in the record, which is what the gap record below is for.
//
// The gap record is written only where a store already exists, under the
// exact rule checkPaused itself follows: creating a store to say "not
// recording" would be worse than saying nothing, and H-81 is the test that a
// paused machine with no store stays that way. A machine with no store still
// gets the marker file pause reads back -- that is what lets status answer
// "paused, no store recorded here yet" truthfully -- but nothing that would
// mint an install identity.
func cmdPause(stdout io.Writer) error {
	root, err := store.DefaultRoot()
	if err != nil {
		return err
	}
	now := time.Now()
	since, already, err := store.Pause(root, now)
	if err != nil {
		return err
	}
	if already {
		fmt.Fprintf(stdout, "rashomon: already paused (since %s)\n", since.UTC().Format(time.RFC3339))
		return nil
	}

	// Zero-width evidence that the window began, in case resume never
	// follows -- a machine paused for good, or a crash, must not read
	// identically to a machine where nothing was ever decided. Resume writes
	// the record a reader actually wants: the whole span, closed.
	if storeExistsAt(root) {
		st, err := store.OpenExisting(root)
		if err != nil {
			return err
		}
		if err := st.AppendGap(store.Gap{
			Type:          store.TypeGap,
			SchemaVersion: store.SessionSchemaVersion,
			RecordedAtMS:  now.UnixMilli(),
			SessionID:     store.PausedSessionID,
			Reason:        store.GapPaused,
			FromUnixMS:    since.UnixMilli(),
			ToUnixMS:      since.UnixMilli(),
		}); err != nil {
			return err
		}
	}
	fmt.Fprintln(stdout, "rashomon: paused -- no tool call will be recorded until `rashomon resume`")
	return nil
}

// cmdResume disengages recording state (Part 2). Where a store exists it
// writes the closed gap record spanning the whole paused window -- from the
// instant pause first took effect to now -- which is what lets a report
// render the window as a named, bounded unknown rather than either a clean
// count or an unexplained absence.
func cmdResume(stdout io.Writer) error {
	root, err := store.DefaultRoot()
	if err != nil {
		return err
	}
	now := time.Now()
	wasPaused, since, err := store.Resume(root, now)
	if err != nil {
		return err
	}
	if !wasPaused {
		fmt.Fprintln(stdout, "rashomon: not paused")
		return nil
	}
	if storeExistsAt(root) {
		st, err := store.OpenExisting(root)
		if err != nil {
			return err
		}
		if err := st.AppendGap(store.Gap{
			Type:          store.TypeGap,
			SchemaVersion: store.SessionSchemaVersion,
			RecordedAtMS:  now.UnixMilli(),
			SessionID:     store.PausedSessionID,
			Reason:        store.GapPaused,
			FromUnixMS:    since.UnixMilli(),
			ToUnixMS:      now.UnixMilli(),
		}); err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "rashomon: resumed -- was paused from %s to %s\n",
		since.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339))
	return nil
}

// Recording state words. RecordingState returns exactly one of these three.
// H-82 is the test that paused, absent and unknown stay distinguishable, so
// "I turned it off" can never collapse into "it was never installed" or the
// reverse. statusRecording renders each state and separately says when an
// active pause state belongs to a machine with no install at all.
const (
	RecordingActive        = "active"
	RecordingPaused        = "paused"
	RecordingPausedNoStore = "paused, no store recorded here yet"
)

// RecordingState reports this machine's pause state for status to render
// (Part 2). It returns one of the three words above, the moment the pause
// began (zero when not paused, or when the pause file holds no readable
// instant), or an error when the pause file could not be read. statusRecording is the one place that turns these into the status
// line, and its own code is the source of truth for the wording; this
// function decides only which state holds. An error must render as unknown,
// never as active: the same unresolved-vs-absent distinction statusUnreadable
// exists for elsewhere in this file.
//
// It is a plain read, like storeExistsAt below: os.Stat and os.ReadFile,
// nothing that opens or creates a store. That is what lets it answer "paused"
// truthfully on a machine that has never recorded, which is the case the
// spec requires status be able to report.
func RecordingState() (state string, since time.Time, err error) {
	root, err := store.DefaultRoot()
	if err != nil {
		return "", time.Time{}, err
	}
	paused, pausedSince, err := store.Paused(root)
	if err != nil {
		return "", time.Time{}, err
	}
	if !paused {
		return RecordingActive, time.Time{}, nil
	}
	if !storeExistsAt(root) {
		return RecordingPausedNoStore, pausedSince, nil
	}
	return RecordingPaused, pausedSince, nil
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
	statusRecording(stdout, root)

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

	settingsLive, err := statusSettings(stdout, installID)
	if err != nil {
		return err
	}
	pluginLive, err := statusPlugin(stdout)
	if err != nil {
		return err
	}
	// Named here so a user sees it without running report, and in the SAME
	// words report uses (duplicate_declarations): a settings install and an
	// enabled plugin can share one machine during the migration window Part 1's
	// spec describes, and nothing removes either of them on its own. detach is
	// the resolution, and it is the settings entries it removes -- the
	// plugin's are not detach's to touch.
	if settingsLive && pluginLive {
		fmt.Fprintln(stdout, "overlap: both a settings install and the rashomon plugin provide these entries, "+
			"recording every tool call twice (report will read coverage: unverified, reason duplicate_declarations); "+
			"run `rashomon detach` to remove the settings entries and keep the plugin")
	}
	if err := statusHooks(stdout); err != nil {
		return err
	}
	return statusRecap(stdout, root)
}

// statusRecap reports the exception line's own evaluation history: whether a
// Stop/StopFailure has ever been evaluated here, when the last one was, and
// whether it completed cleanly (H-92).
//
// Silence on Stop is a notification policy, never a claim that the turn was
// clean -- a plugin can be disabled, a hook can fail to run, a turn can end
// without Stop at all -- so this is the fact silence itself cannot carry.
// "Evaluated, no findings" and "not evaluated" have to read as different
// lines here, or exception-only notification collapses into silence-reads-
// as-zero, which is the whole failure this part exists to avoid.
func statusRecap(stdout io.Writer, root string) error {
	evaluated, lastAt, healthy, err := recap.Status(root)
	switch {
	case err != nil:
		fmt.Fprintf(stdout, "recap: %s (state unreadable)\n", statusUnknown)
	case !evaluated:
		fmt.Fprintln(stdout, "recap: not evaluated (no Stop/StopFailure has completed here yet)")
	case !healthy:
		fmt.Fprintf(stdout, "recap: evaluated, last at %s -- that run did not complete cleanly\n",
			lastAt.UTC().Format(time.RFC3339))
	default:
		fmt.Fprintf(stdout, "recap: evaluated, last at %s\n", lastAt.UTC().Format(time.RFC3339))
	}
	return nil
}

// statusRecording says whether hooks will record, which is the one thing pause
// changes and the first thing a user who ran it will ask. "Active" is only a
// useful answer once a store exists: on a fresh machine no hooks are installed
// to record anything. A pause file that cannot be read is reported as unknown,
// never as active: hooks fall back to recording in that case, but status did
// not see the answer and does not claim one.
func statusRecording(stdout io.Writer, root string) {
	state, since, err := RecordingState()
	// An enabled plugin records from its first hook call, which is also what
	// creates the store, so a missing store alone is not "not installed".
	installed := storeExistsAt(root)
	if err == nil && !installed {
		installed, err = pluginEnabled()
	}
	switch {
	case err != nil:
		fmt.Fprintf(stdout, "  recording: %s\n", statusUnknown)
	case state == RecordingActive && !installed:
		fmt.Fprintln(stdout, "  recording: not installed")
	case state == RecordingActive:
		fmt.Fprintf(stdout, "  recording: %s\n", RecordingActive)
	default:
		line := RecordingPaused
		if !since.IsZero() {
			line += " since " + since.UTC().Format(time.RFC3339)
		}
		if state == RecordingPausedNoStore {
			line += " (no store recorded here yet)"
		}
		fmt.Fprintf(stdout, "  recording: %s\n", line)
	}
}

// pluginEnabled reports whether the rashomon plugin is enabled and declares
// at least one hook entry, the same test statusPlugin applies.
func pluginEnabled() (bool, error) {
	path, err := settings.UserPath()
	if err != nil {
		return false, err
	}
	doc, err := settings.Load(path)
	if err != nil {
		return false, err
	}
	p, err := install.FindPlugin(doc)
	if err != nil || p == nil {
		return false, err
	}
	return p.Enabled && len(p.Events) > 0, nil
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
//
// It returns whether any of our settings entries reads present, which is the
// half of the overlap check that belongs to this origin.
func statusSettings(stdout io.Writer, installID string) (bool, error) {
	path, err := settings.UserPath()
	if err != nil {
		return false, err
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
		return false, nil
	}

	if installID == "" {
		fmt.Fprintln(stdout, "  no store here, so no install id is ours and no entry can be called ours")
	}
	live := false
	for _, event := range install.Events {
		state := entryState(doc, installID, event)
		if state == "present" {
			live = true
		}
		fmt.Fprintf(stdout, "  %s: %s\n", event, state)
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
	return live, nil
}

// statusPlugin reports the plugin origin separately from the settings origin,
// so a user with both can tell which is live rather than seeing one collapsed
// verdict. It is a disk-based answer -- install.FindPlugin, not
// install.PluginPresent -- because status is not itself running as anything's
// hook and has no self-identification to report; that is a real limit and the
// resolved absolute path is printed so a user can tell what was checked.
//
// It returns whether the plugin currently provides these entries, enabled and
// declaring at least one of them -- the other half of the overlap check.
func statusPlugin(stdout io.Writer) (bool, error) {
	path, err := settings.UserPath()
	if err != nil {
		return false, err
	}
	doc, err := settings.Load(path)
	if err != nil {
		fmt.Fprintf(stdout, "plugin: %s\n", statusUnreadable)
		return false, nil
	}

	p, err := install.FindPlugin(doc)
	if err != nil {
		fmt.Fprintf(stdout, "plugin: %s\n", statusUnreadable)
		return false, nil
	}
	if p == nil {
		fmt.Fprintln(stdout, "plugin: none installed")
		return false, nil
	}

	enabled := "disabled"
	if p.Enabled {
		enabled = "enabled"
	}
	fmt.Fprintf(stdout, "plugin: %s (%s) at %s\n", p.Key, enabled, p.Root)

	declares := map[string]bool{}
	for _, event := range p.Events {
		declares[event] = true
	}
	for _, event := range install.Events {
		state := "absent"
		if declares[event] {
			state = "present"
		}
		fmt.Fprintf(stdout, "  plugin %s: %s\n", event, state)
	}
	return p.Enabled && len(p.Events) > 0, nil
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
	timeline := false
	nonoTrail := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session":
			if i+1 >= len(args) {
				return errors.New("--session needs a value")
			}
			sessionID = args[i+1]
			i++
		case "--json":
			asJSON = true
		case "--redact":
			redact = true
		case "--chain":
			chain = true
		case "--timeline":
			timeline = true
		case "--nono-audit":
			// An empty path is refused like a missing one: "" is what not
			// naming a trail means, so `--nono-audit "$NONO_TRAIL"` with the
			// variable unset would print no sandbox section and no reason.
			if i+1 >= len(args) || args[i+1] == "" {
				return errors.New("--nono-audit needs a value")
			}
			nonoTrail = args[i+1]
			i++
		default:
			return fmt.Errorf("unknown argument %q", args[i])
		}
	}
	// Opened WITHOUT creating: a command that only asks a question must not mint
	// an install identity and an HMAC key as a side effect of being asked. The
	// same rule status follows, and for the same reason.
	rep, key, err := reportOrEmpty(sessionID, nonoTrail, time.Now())
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
		// --chain and --timeline expand the text listing only. The JSON
		// carries the whole structure either way: that reader is a program
		// selecting fields, not a person scrolling, and making it pass a flag
		// to receive a section would mean a consumer could parse a report and
		// silently miss one.
		opts := []report.TextOption{}
		if chain {
			opts = append(opts, report.WithChain())
		}
		if timeline {
			opts = append(opts, report.WithTimeline())
		}
		return report.Text(stdout, rep, opts...)
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// cmdDigest renders one turn's projection as JSON.
//
// Unlike report, it can take stdin: a future Stop hook's payload carries
// last_assistant_message there, and that text -- a model's own final reply,
// which can run to several KB -- must never travel through argv, where any
// process on the machine sharing this user can read it via ps. The
// --last-assistant-message flag exists only so a person can drive this by
// hand.
//
// stdin is read ONLY when --stdin is given, never by sniffing what stdin
// happens to be. H-104 is why: a live terminal is not the only stdin that
// never sends EOF -- an inherited pipe that stays open does the same thing,
// and it is the more common case for a command invoked from a script or a
// parent process, not a shell prompt. Guessing intent from the file's mode
// narrows the failure to terminals and leaves every other never-closing
// stdin free to wedge the caller. --stdin is a promise from the caller that
// it will close the pipe, the same promise hook/post/probe already rely on
// from Claude Code without any sniffing at all.
func cmdDigest(args []string, stdin io.Reader, stdout io.Writer) error {
	sessionID := ""
	promptID := ""
	lastMsg := ""
	haveLastMsg := false
	readStdin := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--session":
			if i+1 >= len(args) {
				return errors.New("--session needs a value")
			}
			sessionID = args[i+1]
			i++
		case "--prompt":
			if i+1 >= len(args) {
				return errors.New("--prompt needs a value")
			}
			promptID = args[i+1]
			i++
		case "--last-assistant-message":
			if i+1 >= len(args) {
				return errors.New("--last-assistant-message needs a value")
			}
			lastMsg = args[i+1]
			haveLastMsg = true
			i++
		case "--stdin":
			readStdin = true
		default:
			return fmt.Errorf("unknown argument %q", args[i])
		}
	}
	// The flag wins when given; stdin is the fallback, not a merge, so an
	// explicit empty value cannot be silently overruled by whatever a caller
	// that also passed --stdin left on the pipe.
	if !haveLastMsg && readStdin {
		if m, ok := readStdinLastMessage(stdin); ok {
			lastMsg = m
		}
	}

	d, err := digestOrEmpty(sessionID, promptID, lastMsg, time.Now())
	if err != nil {
		return err
	}
	// Marshalled directly with json.Marshal, byte for byte the same call
	// truncate.go's oversize() held the ceiling against -- an indented
	// encoding, or json.Encoder's own trailing newline folded into that
	// measurement, would each be a different number of bytes than the one the
	// ceiling was actually enforced on. The document is for a script to parse
	// (Part 4's exception line, or a slash command), not a terminal to read,
	// so nothing is lost by staying compact.
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = stdout.Write(append(b, '\n'))
	return err
}

// readStdinLastMessage reads a JSON payload from stdin carrying
// last_assistant_message -- the shape a Stop hook's own stdin would carry.
// It is only ever called when the caller passed --stdin, which is the
// promise that made this read safe -- see cmdDigest's doc on why sniffing
// stdin's type cannot make that promise itself.
func readStdinLastMessage(in io.Reader) (string, bool) {
	raw, err := io.ReadAll(io.LimitReader(in, hook.MaxPayloadBytes+1))
	if err != nil || len(raw) == 0 || len(raw) > hook.MaxPayloadBytes {
		return "", false
	}
	var p struct {
		LastAssistantMessage string `json:"last_assistant_message"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return "", false
	}
	return p.LastAssistantMessage, p.LastAssistantMessage != ""
}

// digestOrEmpty builds the digest, or the empty one when there is nothing
// recorded here at all -- mirroring reportOrEmpty's rule and for the same
// reason: a command that only asks a question must not mint an install
// identity and an HMAC key as a side effect of being asked.
func digestOrEmpty(sessionID, promptID, lastAssistantMessage string, now time.Time) (*digest.Digest, error) {
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		return digest.Empty(now, sessionID, promptID), nil
	}
	if err != nil {
		return nil, err
	}
	if sessionID == "" {
		// No session named: the newest run, the same fallback `recap pending`
		// uses -- this process never learns Claude Code's session id any
		// other way. A store with no runs at all is "nothing recorded here",
		// not an error.
		newest, _, err := st.NewestRun()
		if err != nil {
			return nil, err
		}
		if newest == "" {
			return digest.Empty(now, "", promptID), nil
		}
		sessionID = newest
	}
	return digest.Build(st, sessionID, promptID, lastAssistantMessage, now)
}

// cmdSpend estimates the last N days of Claude Code spend.
//
// Zero setup: the source is Claude Code's own transcripts under its
// configuration directory -- settings.ConfigDir, so CLAUDE_CONFIG_DIR
// relocates this read exactly as it relocates settings.json and plugins/ --
// and nothing here depends on watch having run. The store is read for one
// line only, the spend inside silently failed turns, and opened WITHOUT
// creating: a machine with no store answers "not covered" for that line
// rather than being given a store by the question (H-87's rule, the one
// report and digest follow).
func cmdSpend(args []string, stdout io.Writer) error {
	days := spend.DefaultDays
	asJSON := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--days":
			if i+1 >= len(args) {
				return errors.New("--days needs a value")
			}
			// A positive whole number of days, and nothing else: zero or a
			// negative would name an empty or inverted window, and the
			// answer to that would be a confident $0.00 about nothing.
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 1 {
				return fmt.Errorf("--days needs a positive whole number of days, got %q", args[i+1])
			}
			// And at most spend.MaxDays: a window past any transcript's age
			// is refused rather than answered (see MaxDays for the overflow
			// that once turned such a window into a confident $0.00).
			if n > spend.MaxDays {
				return fmt.Errorf("--days is at most %d, got %q", spend.MaxDays, args[i+1])
			}
			days = n
			i++
		case "--json":
			asJSON = true
		default:
			return fmt.Errorf("unknown argument %q", args[i])
		}
	}

	now := time.Now()
	configDir, err := settings.ConfigDir()
	if err != nil {
		return err
	}
	files, err := spend.Discover(configDir, spend.WindowStart(now, days))
	if err != nil {
		return err
	}
	scan, err := spend.Read(files)
	if err != nil {
		return err
	}
	sum := spend.Build(scan, now, days)

	st, err := openStoreForRead()
	switch {
	case errors.Is(err, store.ErrNoStore):
		st = nil
	case err != nil:
		return err
	}
	if err := sum.Join(st); err != nil {
		return err
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(sum)
	}
	return spend.Text(stdout, sum)
}

// cmdForget evicts records at one end of the store's timeline.
//
// --since is the privacy form: forget what just happened. --before is the
// retention form: forget what is old. They are the two open ends of one window
// and one code path, and naming both would name an empty intersection, so
// naming both is refused rather than resolved.
func cmdForget(args []string, stdout io.Writer) error {
	var from, to *time.Time
	target := ""
	for i := 0; i < len(args); i++ {
		switch flag := args[i]; flag {
		case "--host":
			if i+1 >= len(args) {
				return errors.New("--host needs a value")
			}
			// Canonicalised HERE, before anything is opened, and the canonical
			// name is what every step after it sees: the record match, the
			// digest, the baselines and the line that says what was forgotten.
			// Every host rashomon holds went through host.Canonical, so taken
			// as typed, "GitHub.com" or "github.com:443" forgot nothing while
			// the line said later reports would leave it out. The value is not
			// echoed in the refusal: it may carry a credential, and stderr
			// lands wherever the caller's output does, a transcript included.
			h, ok := host.Canonical(args[i+1])
			if !ok {
				return errors.New("--host is not a hostname: give the destination's name, " +
					"such as github.com (a scheme and a port are dropped; a path, " +
					"credentials or spaces are refused)")
			}
			target = h
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
	case target != "" && (from != nil || to != nil):
		return errors.New("--host removes by destination and --since/--before remove by time; pass one kind or the other")
	case target != "":
		return forgetHost(target, stdout)
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
                               PostToolUseFailure recorders, the
                               SessionStart/SessionEnd liveness probe, and the
                               Stop/StopFailure exception-only recap
  rashomon detach                remove them, leaving everything else as found
  rashomon detach --install <id> remove one install's entries, reading no store
  rashomon detach --all          remove every entry carrying a rashomon install
                               marker, whatever its id
  rashomon pause                 stop recording on this machine, reaching every
                               installed origin; leaves a record of the change
  rashomon resume                start recording again; closes the paused
                               window with a record of how long it lasted
  rashomon status                say what is installed and what the store holds,
                               writing nothing and creating no store
  rashomon report [--session S] [--json] [--redact] [--chain] [--timeline]
                  [--nono-audit PATH]
                               render declarations and coverage, as text for a
                               terminal or as JSON for a consumer; --chain
                               lists the calls under each prompt, --timeline
                               every call across agents in order, and JSON
                               always carries both
  rashomon digest [--session S] [--prompt P] [--last-assistant-message TEXT]
                  [--stdin]
                               render one turn's projection as JSON: what one
                               prompt_id recorded, read-only and never larger
                               than 8 KiB; --session and --prompt default to
                               the most recent session and its most recently
                               started turn. --stdin reads a JSON payload
                               ({"last_assistant_message": "..."}) from
                               stdin for the final message instead of the
                               flag; omitted by default, because reading
                               stdin unless told to is how a caller that
                               never closes its pipe gets hung forever.
  rashomon spend [--days N] [--json]
                               estimate what the last N days (default 30, at
                               most 36500) of Claude Code usage would cost at
                               API list prices, from Claude Code's own
                               transcripts: by agent, model, token kind and
                               session, cache re-written after a gap,
                               refusals by category and model, retry
                               attempts and pre-output refusals in tokens
                               (out of the total, the cost unknown), the
                               responses a fallback model served, and the
                               spend in turns with a failed call the
                               summary never mentioned, over the transcripts
                               its records name (the sessions holding the
                               not-covered spend named); the totals read usage
                               fields only; the silent-failure line also
                               decodes, in memory, the text blocks of every
                               assistant line of each recorded turn with a
                               failed call, keeping only the last, and the
                               content block types (never the text) of user
                               lines with no promptId; a subagent's spend is
                               tied to its turn by the promptId on its own
                               transcript's user lines, and a sidechain
                               response in the main transcript counts toward
                               the prompt before it (a sidechain user line
                               does not end the tie); a response after a line
                               that cannot be decoded (any in a subagent
                               transcript; in the main transcript, unless it
                               is a sidechain line) is not counted; a failed
                               turn with no final message to judge is counted
                               as not checked, never as clean;
                               writes nothing
  rashomon forget --host H       evict every call that named host H
  rashomon forget --since T      evict records recorded at or after T
  rashomon forget --before T     evict records recorded before T
                               either way, leaving a coverage gap behind
  rashomon version               print the version

invoked by Claude Code, never by hand:
  rashomon hook [--install ID]   handle one PreToolUse invocation
  rashomon post [--install ID]   handle one PostToolUse invocation
  rashomon probe start|end [--install ID]
                               handle SessionStart / SessionEnd
  rashomon recap [--install ID]  handle Stop / StopFailure: print an
                               exception-only line on stdout as a
                               systemMessage when a turn has something worth
                               looking at, and nothing otherwise; never
                               exits non-zero

--install names the install whose entry is running. An entry belonging to
another install stands down: it records nothing in this environment.
`)
}

// forgetHost removes every call that named a host, and the host's entry in the
// project baseline.
//
// Both halves are required for the operation to mean anything. Removing the
// records while the baseline still remembers the host would leave it suppressed
// as "seen before" forever, with nothing left to explain why -- a deletion that
// silently changes future reports is worse than no deletion. The gap record
// carries the host, so the record admits what it no longer holds.
//
// A baseline/ directory exists only where a report has run on this machine,
// because a report writes one only then. Without it the output is one line
// saying what was removed; with it, the baseline count follows.
func forgetHost(host string, stdout io.Writer) error {
	st, err := openStoreForRead()
	if errors.Is(err, store.ErrNoStore) {
		fmt.Fprintln(stdout, nothingRecordedHere)
		return nil
	}
	if err != nil {
		return err
	}
	gaps, forgetErr := st.ForgetHost(host, time.Now())
	total := 0
	for _, g := range gaps {
		total += g.RemovedRecords
	}

	// The baseline is keyed per project, and a forget is not told which project
	// the caller meant, so every project that remembers the host loses it. That
	// is the conservative direction: a host the user asked to forget must not
	// survive in a baseline they did not think to name.
	//
	// Run whether or not ForgetHost above succeeded, and the summary below is
	// printed from whatever either one managed before either error is
	// returned. A forget that clears some runs or some baselines and then
	// fails must still say so: a caller who sees only the error cannot tell
	// that part of it already happened.
	cleared, hadBaselines, clearErr := clearBaselines(st.Root(), host)

	fmt.Fprintf(stdout, "rashomon: forgot %s naming %s across %s; %s written",
		countOf(total, "record"), host, countOf(len(gaps), "run"), countOf(len(gaps), "gap record"))
	// Said when nothing was removed, because that is when "forgot 0 records"
	// reads as "nothing happened" -- and before #45 it was true: the host came
	// back under the sandbox heading of the next report. The store-level
	// record ForgetHost now writes keeps it out, and the line says so.
	if total == 0 {
		fmt.Fprint(stdout, "; later reports still leave it out wherever they find it, "+
			"the sandbox trail included")
	}
	if hadBaselines {
		fmt.Fprintf(stdout, "; %d project baseline(s) cleared", cleared)
	}
	fmt.Fprintln(stdout)
	return errors.Join(forgetErr, clearErr)
}

// countOf is n and the noun, singular for exactly one: "1 run", "2 runs".
func countOf(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

// clearBaselines removes a host from every project baseline in the store, and
// reports whether the store holds a baseline directory at all.
//
// A file it cannot clear does not stop it: the host still leaves every other
// project's baseline, and the failures are returned together.
func clearBaselines(root, host string) (int, bool, error) {
	dir := filepath.Join(root, "baseline")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return 0, false, nil
	}
	if err != nil {
		return 0, true, err
	}
	var cleared int
	var failed []error
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		n, err := baseline.ForgetFile(filepath.Join(dir, e.Name()), host)
		if err != nil {
			failed = append(failed, err)
			continue
		}
		cleared += n
	}
	return cleared, true, errors.Join(failed...)
}

// storeExistsAt asks whether a root holds a store without opening one.
// checkPaused, cmdPause and cmdResume all need to ask this about a root
// before -- or instead of -- ever opening a *Store: opening CREATES a store,
// and none of the three may do that as a side effect of a question about
// recording state.
func storeExistsAt(root string) bool {
	_, err := os.Stat(filepath.Join(root, installMetaFile))
	return err == nil
}
