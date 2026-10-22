package store

import (
	"testing"
	"time"
)

// TestRubricNAMING10_AppendGapStampsTypeAndVersion: a gap's type and schema
// version are the store's to set, not each caller's. Five sites built gap
// records by hand, and four of them could drop schema_version without a test
// noticing; a gap without its type is skipped by every reader, which counts
// it as an unreadable gap and marks every session unverified (REC-33).
func TestRubricNAMING10_AppendGapStampsTypeAndVersion(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// A literal with neither field set, and one with both set wrong.
	if err := st.AppendGap(Gap{SessionID: "bare", Reason: GapForget}); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendGap(Gap{Type: TypeDeclaration, SchemaVersion: SchemaVersion, SessionID: "wrong", Reason: GapForget}); err != nil {
		t.Fatal(err)
	}
	gaps, skipped, err := st.ReadGaps()
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 0 || len(gaps) != 2 {
		t.Fatalf("read %d gaps and skipped %d lines, want 2 and 0", len(gaps), skipped)
	}
	for _, g := range gaps {
		if g.Type != TypeGap || g.SchemaVersion != SessionSchemaVersion {
			t.Errorf("gap %q was written as type %q at version %d, want %q at %d",
				g.SessionID, g.Type, g.SchemaVersion, TypeGap, SessionSchemaVersion)
		}
	}
}

// TestRubricNAMING10_NewGapIsAWholeGap: the one constructor every site uses
// sets what every gap carries, so a gap a caller is handed back -- forget's,
// eviction's -- reads the same as the one on disk.
func TestRubricNAMING10_NewGapIsAWholeGap(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)
	g := NewGap(GapPaused, "sess", now)
	want := Gap{
		Type:          TypeGap,
		SchemaVersion: SessionSchemaVersion,
		RecordedAtMS:  now.UnixMilli(),
		SessionID:     "sess",
		Reason:        GapPaused,
	}
	if g != want {
		t.Errorf("NewGap = %+v, want %+v", g, want)
	}
}
