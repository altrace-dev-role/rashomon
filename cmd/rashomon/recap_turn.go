package main

import (
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/fault"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/safe"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

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
// (install.RecapTimeout) even after this command's own work, which
// digest.BenchmarkBuild measures at about 50 ms for a 3,500-call session and
// 300 ms for a 20,000-call one (Apple M3, measured 2026-10-13).
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
	SessionID            string
	LastAssistantMessage string
	// LastMessageTail is the message's last 64 KiB when it was longer than
	// the head (readFinalMessage), kept for the comparator (RES-17, P-40).
	LastMessageTail string
	// Harness is H1's own slice of this same stdin read (readFinalMessage's
	// withHarness), and OK is false whenever recap had nothing usable to
	// decide from: a read error, a malformed or oversized payload, or the
	// recapStdinTimeout firing first. recapHostGuard treats !OK the same as
	// claudeHostGuard treats hookio.Read's own error -- never a stand-down,
	// see recapHostGuard's doc.
	Harness harnessSignal
	OK      bool
	// CWD keys the project index the Stop view reads and writes, and
	// StopHookActive is carried unread (readFinalMessage's fields).
	CWD            string
	StopHookActive bool
}

// readRecapPayload reads and parses stdin within a fixed budget, whatever
// stdin is and whatever it does.
//
// safe.Go, not a bare `go`: a panic on this goroutine cannot be recovered by
// its caller (internal/safe's own doc), and this command's entire contract
// is that it never produces a non-zero exit -- see cmdRecap.
//
// THE READER CAN OUTLIVE THIS CALL, AND IS ONLY BOUNDED BY THE PROCESS. When
// the timeout fires first, the goroutine is still blocked in Read, and
// nothing here can unblock it: a Read on a blocking pipe takes no deadline
// and cannot be cancelled. It lives until stdin delivers or closes, which for
// a pipe Claude Code never closes is until this process exits -- moments
// later, since cmdRecap is the whole of a short-lived hook process and calls
// this once. So call it only from such a process, once: a long-lived caller
// would keep one goroutine and its read buffer per stalled call. The result
// travels over a channel with room for it, never a variable the goroutine
// writes into, so once Read returns the goroutine sends without a receiver
// and ends (TestRubricCONC7_AbandonedStdinReadEndsWhenTheReadReturns); an
// unbuffered channel would hold it on that send for good.
func readRecapPayload(in io.Reader, timeout time.Duration) recapPayload {
	type parsed struct {
		payload recapPayload
		ok      bool
	}
	ch := make(chan parsed, 1)
	safe.Go(func() {
		fm, ok := readFinalMessage(in, true, true)
		if !ok {
			ch <- parsed{}
			return
		}
		ch <- parsed{payload: recapPayload{SessionID: fm.SessionID, LastAssistantMessage: fm.Message, LastMessageTail: fm.Tail,
			Harness: fm.Harness, OK: true, CWD: fm.CWD, StopHookActive: fm.StopHookActive}, ok: true}
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
	// The body's error and panic are dropped on purpose: this entry prints
	// nothing on any internal failure (H-93), and recap writes no record whose
	// loss a reader would have to see. That includes a failure recap could not
	// record for status: RecordFailure returns it here, to be dropped by this
	// one rule rather than where it happened.
	_ = safe.Guard(func() error { return recapTurn(args, stdin, stdout, stderr) })
	return exitOK
}

// recapTurn is cmdRecap's body. An error it returns has already been recorded
// for status where it could be (recap.RecordFailure), or is the failure to
// record it; cmdRecap drops both.
func recapTurn(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	hook.IgnoreBrokenPipe()
	fault.Inject(fault.PointRecapStart)

	now := time.Now()
	payload := readRecapPayload(stdin, recapStdinTimeout)

	if id, down := recapHostGuard(payload.Harness, payload.OK); noteHarness(payload.SessionID, id, down) {
		return nil
	}

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
		return recap.RecordFailure(root, now)
	}
	if standsDown(args, st, stderr) {
		return nil
	}

	// `recap pending` runs on UserPromptSubmit, before the new prompt has
	// made a single call, so the newest turn in the store is the one that
	// just ended. It speaks only when no recap reached that turn: saying
	// No at a permission prompt interrupts the turn and Claude Code fires
	// no Stop after an interrupt, so without this a refused call -- the
	// case the line exists for -- could never be shown at all.
	pending := recapPending(args)
	if pending {
		// An interrupted turn has no final message, and this payload's
		// prompt field is the NEW prompt's text, never read here.
		payload.LastAssistantMessage, payload.LastMessageTail = "", ""
	}

	// A payload that names no session gets the digest that says so --
	// no_session_id, beside no_store -- and never a session found some
	// other way (H-107). This used to fall back to the newest run in the
	// store, on the reasoning that this process learns Claude Code's
	// session id only by being told it. That is true, and it is the
	// argument AGAINST guessing. Per Cursor's documentation -- nobody has
	// run Cursor against this recorder -- Cursor runs these hooks from
	// ~/.claude/settings.json and names a conversation_id where Claude
	// Code sends session_id, so on a machine that also runs Claude Code
	// the newest run is whichever Claude Code session wrote last. The line
	// described that session's turn, pointed at that session's report, and
	// claimed that turn in recap.json, so the real Stop for it then stayed
	// silent as a duplicate -- H-107 reproduces that with a payload of
	// that shape, without Cursor. A payload this process could not read
	// lands here too, for the same reason: an id it could not read is not
	// one it can look up.
	//
	// The catch-up goes through the same digest and stops at
	// recap.Pending, which answers false for a turn with no session id:
	// it speaks only for a turn it can show no Stop reached, and without
	// an id it can show nothing. Stating no_session_id is the Stop's job.
	//
	// The run is read once, for the digest and the Stop view (card 2.4 R2),
	// and the digest reads it first: it chooses the prompt, and build's
	// masking fields are written into the run before the view reads it.
	d := digest.Sessionless(now)
	var run *store.Run
	if payload.SessionID != "" {
		r, rerr := digest.ReadRun(st, payload.SessionID)
		if rerr != nil {
			return recap.RecordFailure(root, now)
		}
		built, derr := digest.BuildFrom(r, st, payload.SessionID, "", payload.LastAssistantMessage, now)
		if derr != nil {
			return recap.RecordFailure(root, now)
		}
		d, run = built, r
	}
	if pending && !recap.Pending(root, d.SessionID, d.PromptID) {
		return nil
	}
	stop, serr := stopView(st, run, d, payload, pending, now)
	if serr != nil {
		return serr
	}

	// recap.Line's sentences win; the done flag and the value line speak
	// only where none does (card 2.4 R12).
	fromPlugin := recapFromPlugin(args)
	line, wantSpeak := recap.Line(d, d.SessionID, fromPlugin)
	if !wantSpeak {
		line, wantSpeak = stop.line(d.SessionID, fromPlugin)
	}
	// The barrier before stdout (REC-34), and before Claim (D93): a line
	// printed over a checkpoint that is not durable would claim what the
	// record may not hold, so a failed Flush prints nothing, and it leaves
	// the prompt unclaimed, so a later Stop or the catch-up can still speak.
	if err := fault.Fail(fault.PointRecapFlush); err != nil {
		return err
	}
	if err := st.Flush(); err != nil {
		return err
	}
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
}

// recapFromPlugin reports whether this recap runs as the plugin's entry.
//
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
// unmarked invocation asks where it is running from, and it asks
// PluginFires, not PluginPresent: a plugin whose Stop entry was hand
// edited reads absent in coverage, but it is loaded and ran this, so
// its slash command exists. An error there reads as "not the
// plugin", which points at the CLI: the settings install's command,
// and the only one that exists without a plugin.
func recapFromPlugin(args []string) bool {
	if installArg(args) != "" {
		return false
	}
	present, err := install.PluginFires(install.EventStop)
	return err == nil && present
}
