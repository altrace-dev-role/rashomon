package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestStatusSize_NotesAnUnlistableRunRatherThanHidingTheTotal: RunsBytes can
// fail on one run's directory while still knowing the size of every other
// one. statusSize shows that partial total, with a count of what it could
// not list, instead of printing "unknown" for a figure it mostly has.
func TestStatusSize_NotesAnUnlistableRunRatherThanHidingTheTotal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod 0 does not block a root process")
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"sess-fine", "sess-unlistable"} {
		if err := st.AppendDeclaration(store.Declaration{Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
			ToolUseID: "toolu_" + s, SessionID: s}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(st.RunDir("sess-unlistable"), 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(st.RunDir("sess-unlistable"), 0o700) //nolint:errcheck // best-effort cleanup

	var buf bytes.Buffer
	statusSize(&buf, st)
	got := buf.String()
	if strings.Contains(got, statusUnknown) {
		t.Errorf("statusSize with one unlistable run prints %q, want the listable total rather than unknown", got)
	}
	if !strings.Contains(got, "1 run not listed") {
		t.Errorf("statusSize with one unlistable run prints %q, want it to say 1 run not listed", got)
	}
}
