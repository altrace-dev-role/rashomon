package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/hostguard"
	"github.com/altrace-dev-role/rashomon/internal/settings"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// claudeEnv resolves harness.Detect's Env from this process's own
// environment: CLAUDE_CODE_ENTRYPOINT verbatim (Env's own doc: Detect never
// calls os.Getenv itself) and settings.ConfigDir(), which is
// CLAUDE_CONFIG_DIR when set and ~/.claude otherwise. A ConfigDir error
// (os.UserHomeDir failing) resolves to "", which Detect's own
// underProjects already treats as "no transcript match" rather than a
// crash -- this entry point must never fail on that account.
func claudeEnv() harness.Env {
	dir, _ := settings.ConfigDir()
	return harness.Env{Entrypoint: os.Getenv("CLAUDE_CODE_ENTRYPOINT"), ConfigDir: dir}
}

// onceErrReader is an io.Reader whose first Read always returns (0, err)
// and never anything else. Fed through hookio.Read's io.ReadFull loop, this
// reproduces err as hookio.Read's own terminal error, because ReadFull
// returns the first non-EOF error its underlying reader gives it without
// ever calling Read again -- see claudeHostGuard's doc for why that
// property is the whole reason this type exists.
type onceErrReader struct{ err error }

func (r *onceErrReader) Read([]byte) (int, error) { return 0, r.err }

// claudeHostGuard reads stdin EXACTLY ONCE (hookio.Read) and decides whether
// this Claude-installed entry point must stand down under H1 (Technical
// spec 6.2.3, BRD-4: internal/hostguard.Evaluate, installed-for "claude"
// for all five entry points -- hook, post, probe's two phases and recap --
// since no Cursor or Codex adapter exists in this tree to install anything
// else).
//
// It returns a replay function the caller uses in place of the real stdin
// for whatever it does next (checkPaused, h.Capture, hook.RunProbe): each
// call to replay() hands back a fresh reader over the SAME bytes hookio.Read
// already produced, or a onceErrReader reproducing the SAME read error, so
// nothing downstream can tell the difference from reading the pipe
// directly -- in particular, a payload hookio.Read itself refused (e.g.
// ErrTooLarge) still produces the identical error, and so the identical
// coverage reason, the existing Capture/RunProbe error path already writes
// for it today. See internal/hostguard.Evaluate's doc for why an unreadable
// payload must never itself be read as a stand-down.
//
// On standDown, the caller's only remaining job is to return: no
// checkPaused, no openForHook, no record, no run directory, no output.
// This function has already made a best-effort attempt to record the
// mismatch in harness_counts.json (store.RecordHarnessMismatch): best
// effort because 6.2.1 and 6.4 both describe that file as something a hook
// skips under contention, never something a stand-down fails over, and
// because a hook entry point's one hard rule is that it never fails over
// anything (see cmdHook's own file comment).
func claudeHostGuard(stdin io.Reader) (replay func() io.Reader, standDown bool) {
	raw, rerr := hookio.Read(stdin)
	replay = func() io.Reader {
		if rerr != nil {
			return &onceErrReader{err: rerr}
		}
		return bytes.NewReader(raw)
	}

	payload, id, down := hostguard.EvaluateRaw(raw, rerr, claudeEnv(), hostguard.FamilyClaude)
	if !down {
		return replay, false
	}

	sessionID := ""
	if payload != nil {
		sessionID, _ = payload.Str(hookio.SessionID)
	}
	recordHarnessMismatchBestEffort(sessionID, id)
	return replay, true
}

// recordHarnessMismatchBestEffort is claudeHostGuard's and recapHostGuard's
// shared write half. sessionID is each caller's own: claudeHostGuard's
// comes from the *hookio.Payload hookio.Read produced; recapHostGuard's
// comes from the session_id readRecapPayload already captured, never from
// syntheticHarnessPayload, which carries no session_id field at all (see
// its own doc on why -- Detect never reads one).
func recordHarnessMismatchBestEffort(sessionID string, id harness.Identity) {
	root, err := store.DefaultRoot()
	if err != nil {
		return
	}
	evidence := make([]string, len(id.Evidence))
	for i, e := range id.Evidence {
		evidence[i] = string(e)
	}
	key := hostguard.CounterKey(hostguard.FamilyClaude, id)
	// Best-effort, by design -- see claudeHostGuard's doc. Nothing here is
	// a hook error this entry point could report even if it wanted to: a
	// mismatch write that fails leaves the stand-down itself unaffected,
	// which is the one outcome that actually matters (no record, no run
	// directory, no output).
	_, _ = store.RecordHarnessMismatch(root, key, sessionID, evidence, time.Now())
}

// syntheticHarnessPayload builds a *hookio.Payload carrying exactly the
// fields internal/harness.Detect reads (its own doc lists them) from an hs
// readFinalMessage captured without ever holding the Stop payload whole.
// It goes through hookio.Intake -- the one sanctioned way any entry point
// in this tree turns bytes into a *hookio.Payload (internal/hostguard's own
// doc: "none of them re-derives identity from a payload") -- rather than
// filling a Payload's Spans by hand, so Detect is handed exactly the same
// shape of value it is everywhere else.
//
// A key is omitted rather than written as null or empty: hs.HookEventName
// and hs.TranscriptPath empty are indistinguishable from absent to Detect
// (an empty event name matches no codexTurnIDEvents entry and no "" !=
// "SessionStart" comparison changes; an empty transcript path fails
// underProjects exactly as a missing one does), so there is no value worth
// spending a key on. The four presence-only fields are written as the JSON
// literal true: Detect never reads their value, only internal/hookio.
// Payload.Has, and true is Has-true under every path (string, number and
// object values all satisfy Set() too, but true costs the fewest bytes).
//
// This can read a conversation_id/generation_id/turn_id/model JSON false as
// present where the buffered hook/post/probe path's Has would read it as
// absent (jsonscan.CapturePresence does not exclude false; Span.Set does --
// see harnessSignal's own doc). That gap can only ever make this function
// MORE willing to construct a payload Detect reads as foreign, never less:
// a false value on one of these four keys is not a shape any of Claude
// Code, Cursor or Codex has been observed to send, so the practical effect
// is nil, and the direction it is wrong in is the fail-closed one (BRD-4:
// "fail toward recording, not toward silence").
func syntheticHarnessPayload(hs harnessSignal) (*hookio.Payload, error) {
	m := make(map[string]any, 7)
	if hs.HookEventName != "" {
		m["hook_event_name"] = hs.HookEventName
	}
	if hs.TranscriptPath != "" {
		m["transcript_path"] = hs.TranscriptPath
	}
	if hs.HasCursorVersion {
		m["cursor_version"] = hs.CursorVersion
	}
	if hs.HasConversationID {
		m["conversation_id"] = true
	}
	if hs.HasGenerationID {
		m["generation_id"] = true
	}
	if hs.HasTurnID {
		m["turn_id"] = true
	}
	if hs.HasModel {
		m["model"] = true
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return hookio.Intake(bytes.NewReader(raw))
}

// recapHostGuard decides cmdRecap's H1 stand-down (BRD-4) from the
// harness-signal slice readRecapPayload already captured in the same
// stream pass as the message itself: recap gets exactly one read of stdin
// (recapStdinTimeout), and this is the only chance H1 has to see any of it.
//
// ok false (a read error, a malformed or oversized payload, or the timeout
// firing first) is treated the same way claudeHostGuard treats hookio.
// Read's own error: never a stand-down. The reasoning is the same one
// hostguard.EvaluateRaw's own doc gives for that case -- a payload this
// process never finished reading carries no POSITIVE evidence of a foreign
// harness, and recap's one hard rule (H-93) is that it produces no output
// on an internal failure, which a stand-down that fired on "stdin was
// slow" rather than "stdin said Cursor" would violate in spirit even
// though not in form.
//
// On ok true, an hs with every field at its zero value (no marker was in
// the payload at all) is not specially cased: it is run through Detect like
// any other, which still resolves Claude Code correctly from claudeEnv's
// own CLAUDE_CODE_ENTRYPOINT when that env var is set, the same as it does
// for a hook, post or probe payload carrying none of these fields either.
func recapHostGuard(hs harnessSignal, ok bool) (harness.Identity, bool) {
	if !ok {
		return harness.Identity{}, false
	}
	payload, err := syntheticHarnessPayload(hs)
	if err != nil {
		return harness.Identity{}, false
	}
	return hostguard.EvaluateFromPayload(payload, claudeEnv(), hostguard.FamilyClaude)
}
