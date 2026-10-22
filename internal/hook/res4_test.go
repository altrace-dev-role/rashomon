package hook

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// writePayload is a Write call's payload of exactly n bytes.
func writePayload(n int, session string) []byte {
	head := fmt.Sprintf(`{"session_id":%q,"tool_use_id":"toolu_cap","tool_name":"Write","tool_input":{"file_path":"/tmp/cap.txt","content":"`, session)
	tail := `"}}`
	return []byte(head + string(bytes.Repeat([]byte("x"), n-len(head)-len(tail))) + tail)
}

// TestRES4_CapAtNAndNPlusOne: a payload of exactly MaxPayloadBytes is accepted
// and recorded; one byte more is refused after a bounded read, and the refusal
// writes a coverage record carrying the reason payload_too_large, not
// internal_error, in the run the payload's head names, or the unattributed
// run when it names none. Break: refuse at N, or record the refusal as an
// internal error, and this fails.
func TestRES4_CapAtNAndNPlusOne(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	st, err := store.Open(home + "/store")
	if err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.UnixMilli(1_760_000_000_000) }
	sig := &Signals{ch: make(chan os.Signal, 1)}

	at := writePayload(MaxPayloadBytes, "s-at-cap")
	if len(at) != MaxPayloadBytes || MaxPayloadBytes != 8<<20 {
		t.Fatalf("payload %d bytes, cap %d", len(at), MaxPayloadBytes)
	}
	h := New(st, now)
	err = h.Capture(bytes.NewReader(at))
	h.Close(sig, err)
	if err != nil {
		t.Fatalf("exactly %d bytes refused: %v", MaxPayloadBytes, err)
	}
	run, err := st.ReadRun("s-at-cap")
	if err != nil || len(run.Declarations) != 1 || len(run.Coverage) != 1 {
		t.Fatalf("at the cap: %d declarations, %d coverage records, %v", len(run.Declarations), len(run.Coverage), err)
	}
	// No settings are installed here, so the reason is the entry's absence;
	// it must not be a failure of the call itself.
	if r := run.Coverage[0].Reason; r != nil && (*r == store.ReasonPayloadTooLarge || *r == store.ReasonInternalError) {
		t.Fatalf("at the cap: coverage reason %s", *r)
	}

	over := writePayload(MaxPayloadBytes+1, "s-over-cap")
	h = New(st, now)
	err = h.Capture(bytes.NewReader(over))
	h.Close(sig, err)
	if !errors.Is(err, hookio.ErrTooLarge) {
		t.Fatalf("%d bytes: %v, want ErrTooLarge", len(over), err)
	}
	run, err = st.ReadRun("s-over-cap")
	if err != nil || len(run.Declarations) > 0 {
		t.Fatalf("an over-cap payload: %d declarations, %v", len(run.Declarations), err)
	}
	refusedIn(t, st, "s-over-cap")

	h = New(st, now)
	err = h.Capture(bytes.NewReader(writePayload(MaxPayloadBytes+1, "")))
	h.Close(sig, err)
	if !errors.Is(err, hookio.ErrTooLarge) {
		t.Fatalf("no session: %v, want ErrTooLarge", err)
	}
	refusedIn(t, st, UnattributedSession)
}

// refusedIn checks that session's run ends with a payload_too_large
// coverage record.
func refusedIn(t *testing.T, st *store.Store, session string) {
	t.Helper()
	run, err := st.ReadRun(session)
	if err != nil || len(run.Coverage) == 0 {
		t.Fatalf("no coverage in run %q: %v", session, err)
	}
	last := run.Coverage[len(run.Coverage)-1]
	if last.Reason == nil || *last.Reason != store.ReasonPayloadTooLarge {
		t.Fatalf("refusal in %q recorded with reason %v, want %s", session, last.Reason, store.ReasonPayloadTooLarge)
	}
}

// TestRES4_PostRefusesAnOverCapPayload: Post shares hookio.Read's cap,
// closeReason's mapping and refusedSession with Handler, so an over-cap
// payload here is refused and recorded the same way
// TestRES4_CapAtNAndNPlusOne proves it is for a declaration.
func TestRES4_PostRefusesAnOverCapPayload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	st, err := store.Open(home + "/store")
	if err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.UnixMilli(1_760_000_000_000) }
	sig := &Signals{ch: make(chan os.Signal, 1)}

	over := writePayload(MaxPayloadBytes+1, "s-post-over-cap")
	p := NewPost(st, now)
	err = p.Capture(bytes.NewReader(over))
	p.Close(sig, err)
	if !errors.Is(err, hookio.ErrTooLarge) {
		t.Fatalf("%d bytes: %v, want ErrTooLarge", len(over), err)
	}
	refusedIn(t, st, "s-post-over-cap")
}

// TestRES4_ProbeRefusesAnOverCapPayload: RunProbe shares the same cap,
// closeReason mapping and refusedSession, and records the refusal in the
// run the payload's head names exactly as Handler and Post do.
func TestRES4_ProbeRefusesAnOverCapPayload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	st, err := store.Open(home + "/store")
	if err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.UnixMilli(1_760_000_000_000) }
	sig := &Signals{ch: make(chan os.Signal, 1)}

	over := writePayload(MaxPayloadBytes+1, "s-probe-over-cap")
	RunProbe(sig, store.PhaseStart, bytes.NewReader(over), st, now)
	refusedIn(t, st, "s-probe-over-cap")
}

// TestRES4_PausedAttributesAnOverCapPayload: a paused call reads its payload
// only for attribution, and an over-cap payload is attributed from its head
// as the recording entries attribute it.
func TestRES4_PausedAttributesAnOverCapPayload(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	st, err := store.Open(home + "/store")
	if err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.UnixMilli(1_760_000_000_000) }
	RecordPaused(bytes.NewReader(writePayload(MaxPayloadBytes+1, "s-paused-over-cap")), store.PhaseCall, store.ReasonRecordingPaused, st, now)
	run, err := st.ReadRun("s-paused-over-cap")
	if err != nil || len(run.Coverage) != 1 {
		t.Fatalf("paused over-cap payload: %d coverage records in its run, %v", len(run.Coverage), err)
	}
}
