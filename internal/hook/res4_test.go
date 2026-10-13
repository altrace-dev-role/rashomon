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
// writes a coverage record in the unattributed run carrying the reason
// payload_too_large, not internal_error. Break: refuse at N, or record the
// refusal as an internal error, and this fails.
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
	if run, err := st.ReadRun("s-over-cap"); err == nil && len(run.Declarations) > 0 {
		t.Fatalf("an over-cap payload was attributed and recorded")
	}
	run, err = st.ReadRun(UnattributedSession)
	if err != nil || len(run.Coverage) == 0 {
		t.Fatalf("no coverage in the unattributed run: %v", err)
	}
	last := run.Coverage[len(run.Coverage)-1]
	if last.Reason == nil || *last.Reason != store.ReasonPayloadTooLarge {
		t.Fatalf("refusal recorded with reason %v, want %s", last.Reason, store.ReasonPayloadTooLarge)
	}
}
