package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
)

// intakeFor is this file's own helper, mirroring internal/harness's intake
// test helper: every case below runs through the real hookio.Intake scan
// harnessSessionKey is built to consume, not a hand-filled Payload.
func intakeFor(t *testing.T, raw string) *hookio.Payload {
	t.Helper()
	p, err := hookio.Intake(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("hookio.Intake: %v", err)
	}
	return p
}

// TestHarnessSessionKey_CursorUsesConversationID: Cursor keys its own
// sessions by conversation_id, never session_id (Technical spec 6.2.5:
// "runs/<cursor-+conversation_id>"), so a Cursor-detected stand-down's
// dedup key must come from conversation_id even when the payload also
// happens to carry a session_id -- the field a Cursor payload would
// normally never send, but a hand-crafted or adversarial one still could.
func TestHarnessSessionKey_CursorUsesConversationID(t *testing.T) {
	raw := `{"hook_event_name":"afterFileEdit","cursor_version":"1.4.2","conversation_id":"conv-1","session_id":"should-be-ignored"}`
	p := intakeFor(t, raw)
	got := harnessSessionKey(p, harness.Identity{ID: harness.Cursor})
	if got != "conv-1" {
		t.Errorf("harnessSessionKey = %q, want %q", got, "conv-1")
	}
}

// TestHarnessSessionKey_CursorWithNoConversationIDIsEmpty: a Cursor-shaped
// payload (cursor_version alone is already enough for Detect's step 1)
// that carries no conversation_id at all must return "", never fall back
// to session_id -- session_id is not the field Cursor sessions key by, and
// a fallback there would silently resurrect the undercount this function
// exists to close for the common case (Cursor payloads normally send no
// session_id either).
func TestHarnessSessionKey_CursorWithNoConversationIDIsEmpty(t *testing.T) {
	raw := `{"hook_event_name":"afterFileEdit","cursor_version":"1.4.2"}`
	p := intakeFor(t, raw)
	got := harnessSessionKey(p, harness.Identity{ID: harness.Cursor})
	if got != "" {
		t.Errorf("harnessSessionKey = %q, want \"\"", got)
	}
}

// TestHarnessSessionKey_NonCursorUsesSessionID: every other detected
// identity (Codex, Unknown) keeps the session_id this function always
// used, since 6.2.5 names session_id, not conversation_id, as Codex's own
// key, and Unknown carries no documented convention of its own to prefer
// instead.
func TestHarnessSessionKey_NonCursorUsesSessionID(t *testing.T) {
	for _, id := range []harness.ID{harness.Codex, harness.Unknown} {
		raw := `{"hook_event_name":"PreToolUse","session_id":"sess-1","turn_id":"t1"}`
		p := intakeFor(t, raw)
		got := harnessSessionKey(p, harness.Identity{ID: id})
		if got != "sess-1" {
			t.Errorf("id=%s: harnessSessionKey = %q, want %q", id, got, "sess-1")
		}
	}
}

// TestHarnessSessionKey_NilPayloadIsEmpty: EvaluateRaw returns a nil
// payload whenever it could not parse raw (malformed JSON, a read error);
// harnessSessionKey must not panic dereferencing it, and must agree with
// RecordHarnessMismatch's own "" contract.
func TestHarnessSessionKey_NilPayloadIsEmpty(t *testing.T) {
	if got := harnessSessionKey(nil, harness.Identity{ID: harness.Cursor}); got != "" {
		t.Errorf("harnessSessionKey(nil, ...) = %q, want \"\"", got)
	}
}

// TestRES4_TheHostGuardKeepsAnOverCapPayloadsSession: the guard reads stdin
// once, and an over-cap payload's refusal reaches Intake through its replay,
// as many times as a caller asks; each time it must name the session and
// cwd the payload's head carries, so the gap is recorded in the run that
// sent it. Break: Read returns a bare ErrTooLarge, or Intake refuses the
// replay afresh from the empty head it reads, and the gap lands in the
// unattributed run.
func TestRES4_TheHostGuardKeepsAnOverCapPayloadsSession(t *testing.T) {
	head := `{"session_id":"s-big","cwd":"/p","tool_name":"Write","tool_input":{"content":"`
	tail := `"}}`
	raw := head + strings.Repeat("x", hookio.MaxPayloadBytes+1-len(head)-len(tail)) + tail
	payload, replay, down := claudeHostGuard(strings.NewReader(raw))
	if down || payload != nil {
		t.Fatalf("guard: down %v payload %v, want a refusal handed on", down, payload)
	}
	for i := 0; i < 2; i++ {
		_, err := hookio.Intake(replay())
		var tl *hookio.TooLargeError
		if !errors.As(err, &tl) {
			t.Fatalf("replay %d: %v, want a *TooLargeError", i, err)
		}
		if tl.SessionID != "s-big" || tl.CWD != "/p" {
			t.Fatalf("replay %d: session %q cwd %q, want s-big /p", i, tl.SessionID, tl.CWD)
		}
	}
}
