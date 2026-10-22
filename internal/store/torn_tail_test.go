package store

import (
	"path/filepath"
	"testing"
	"time"
)

// tornFragment is the front of a record a killed or failing writer cut off
// before its newline.
const tornFragment = `{"type":"declaration","schema_version":4,"tool_use_id":"toolu_torn"`

// TestRubricERRORS1_TornTailsNeverSwallowTheNextRecord: an append used to go
// straight after whatever the file ended with, so after a torn line the next
// healthy record was glued onto it and both read as one unparsable line --
// a record lost, and blamed on the lock. Every append that holds its file's
// lock now starts a line of its own: the fragment reads as one skipped line
// and the record after it reads intact. records.ndjson is REC-32's half
// (TestREC32_TornTailNeverSwallowsTheNextRecord drives it through the hook);
// this covers the files appendLine and Evict write.
func TestRubricERRORS1_TornTailsNeverSwallowTheNextRecord(t *testing.T) {
	const session = "sess-torn"
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	t.Run("records", func(t *testing.T) {
		if err := st.AppendDeclaration(testDecl(session, "toolu_1")); err != nil {
			t.Fatal(err)
		}
		appendRaw(t, filepath.Join(st.RunDir(session), FileRecords), []byte(tornFragment))
		if err := st.AppendDeclaration(testDecl(session, "toolu_2")); err != nil {
			t.Fatal(err)
		}
		run, err := st.ReadRun(session)
		if err != nil {
			t.Fatal(err)
		}
		if len(run.Declarations) != 2 || run.Skipped != 1 {
			t.Errorf("read %d declarations and %d skipped lines, want 2 and 1", len(run.Declarations), run.Skipped)
		}
	})

	t.Run("coverage", func(t *testing.T) {
		cov := Coverage{Type: TypeCoverage, SchemaVersion: SessionSchemaVersion, SessionID: session, State: StateVerified}
		if err := st.AppendCoverage(cov); err != nil {
			t.Fatal(err)
		}
		appendRaw(t, filepath.Join(st.RunDir(session), FileCoverage), []byte(`{"type":"coverage"`))
		if err := st.AppendCoverage(cov); err != nil {
			t.Fatal(err)
		}
		run, err := st.ReadRun(session)
		if err != nil {
			t.Fatal(err)
		}
		if len(run.Coverage) != 2 {
			t.Errorf("read %d coverage records around a torn one, want 2", len(run.Coverage))
		}
	})

	t.Run("gaps", func(t *testing.T) {
		gap := Gap{Type: TypeGap, SchemaVersion: SessionSchemaVersion, SessionID: "other", Reason: GapForget}
		appendRaw(t, filepath.Join(st.Root(), FileGaps), []byte(`{"type":"gap"`))
		if err := st.AppendGap(gap); err != nil {
			t.Fatal(err)
		}
		gaps, skipped, err := st.ReadGaps()
		if err != nil {
			t.Fatal(err)
		}
		if len(gaps) != 1 || skipped != 1 {
			t.Errorf("read %d gaps and %d skipped lines after a torn tail, want 1 and 1", len(gaps), skipped)
		}
	})

	t.Run("eviction", func(t *testing.T) {
		appendRaw(t, filepath.Join(st.Root(), FileGaps), []byte(`{"type":"gap"`))
		old := time.Now().Add(-48 * time.Hour)
		if err := chtimesTree(st.RunDir(session), old); err != nil {
			t.Fatal(err)
		}
		evicted, err := st.Evict(1, "", time.Now())
		if err != nil || len(evicted) != 1 {
			t.Fatalf("Evict: %d gaps, %v", len(evicted), err)
		}
		gaps, skipped, err := st.ReadGaps()
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, g := range gaps {
			found = found || g.Reason == GapSizeCap
		}
		if !found || skipped != 2 {
			t.Errorf("after a torn tail the eviction gap reads %v with %d skipped lines, want it read and 2 skipped", found, skipped)
		}
	})
}
