package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestRubricDOCS6_AFailedTerminalNeverReadsVerified: Close fixed the
// coverage reason before it wrote the terminal and threw the terminal's
// error away, so a call whose terminal never landed was still covered by a
// verified record. A terminal that cannot be written now makes the call's
// coverage internal_error. (The end probe would also have found the
// declaration unterminated, but only if the session ever ended.)
func TestRubricDOCS6_AFailedTerminalNeverReadsVerified(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st := mustOpen(t)
	h := New(st, time.Now)
	captureErr := h.Capture(strings.NewReader(capPayload(t, "tool_use_id", "toolu_noterm")))
	if captureErr != nil {
		t.Fatal(captureErr)
	}
	// The records file stops taking writes between the declaration and its
	// terminal: a full disk, a revoked permission.
	records := filepath.Join(st.RunDir("sess-caps"), store.FileRecords)
	if err := os.Chmod(records, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(records, 0o600) })
	h.Close(&Signals{}, captureErr)

	run, err := st.ReadRun("sess-caps")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Terminals) != 0 {
		t.Fatalf("premise broken: the terminal landed")
	}
	if len(run.Coverage) != 1 || run.Coverage[0].Reason == nil || *run.Coverage[0].Reason != store.ReasonInternalError {
		t.Errorf("coverage %+v, want internal_error for a call whose terminal was not written", run.Coverage)
	}
}
