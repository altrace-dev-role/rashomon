package gather

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestHarnessUnknownSessions_ReadsTheClaudeUnknownCounter: the report's count
// is harness_counts.json's claude@unknown key (BRD-4 ruling, 2026-10-24), in a
// report scoped to one session as in the whole one; a store with no counter
// reads zero; a counter that cannot be parsed reads unknown (nil), never zero.
func TestHarnessUnknownSessions_ReadsTheClaudeUnknownCounter(t *testing.T) {
	st, now := sessionlessStore(t, 0)
	root := st.Root()
	for _, scope := range []string{"", "sess-1"} {
		rep, err := Build(st, scope, now)
		if err != nil {
			t.Fatal(err)
		}
		if rep.HarnessUnknownSessions == nil || *rep.HarnessUnknownSessions != 0 {
			t.Errorf("Build(%q) with no counter: %v, want 0", scope, deref(rep.HarnessUnknownSessions))
		}
	}

	for _, sess := range []string{"a", "b"} {
		if _, err := store.RecordHarnessSeen(root, "claude@unknown", sess, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.RecordHarnessMismatch(root, "claude@cursor", "c", []string{"cursor_version"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"", "sess-1"} {
		rep, err := Build(st, scope, now)
		if err != nil {
			t.Fatal(err)
		}
		if rep.HarnessUnknownSessions == nil || *rep.HarnessUnknownSessions != 2 {
			t.Errorf("Build(%q): %v, want 2 (claude@unknown only)", scope, deref(rep.HarnessUnknownSessions))
		}
	}

	if err := os.WriteFile(filepath.Join(root, store.FileHarnessCounts), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	rep, err := Build(st, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if rep.HarnessUnknownSessions != nil {
		t.Errorf("a corrupt counter: %d, want unknown (nil)", *rep.HarnessUnknownSessions)
	}
}
