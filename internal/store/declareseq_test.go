package store

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestStore_DeclareSeqIsTheRecordsSeq: the seq DeclareSeq returns is the seq
// of the line it wrote to records.ndjson, for two sessions whose declarations
// interleave. A hook cites the seq in an overlay group's marker, so a seq that
// is not the line's names a record that is not there.
func TestStore_DeclareSeqIsTheRecordsSeq(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, c := range []struct{ session, id string }{
		{"sess-a", "toolu_a1"}, {"sess-b", "toolu_b1"}, {"sess-a", "toolu_a2"}, {"sess-b", "toolu_b2"}, {"sess-a", "toolu_a3"},
	} {
		seq, err := st.DeclareSeq(testDecl(c.session, c.id))
		if err != nil || seq <= 0 {
			t.Fatalf("%s: (%d, %v), want a seq and no error", c.id, seq, err)
		}
		got[c.id] = seq
	}
	for _, session := range []string{"sess-a", "sess-b"} {
		run, err := st.ReadRun(session)
		if err != nil {
			t.Fatal(err)
		}
		if len(run.Declarations) == 0 {
			t.Fatalf("%s holds no declaration", session)
		}
		for _, d := range run.Declarations {
			if got[d.ToolUseID] != d.Seq {
				t.Errorf("%s: DeclareSeq said %d, the line in records.ndjson holds %d", d.ToolUseID, got[d.ToolUseID], d.Seq)
			}
		}
	}
	if got["toolu_a1"] != 1 || got["toolu_a2"] != 2 || got["toolu_a3"] != 3 || got["toolu_b2"] != 2 {
		t.Errorf("seqs %v: each session counts from 1 on its own", got)
	}
}

// TestStore_DeclareSeqZeroWhenTheStreamHoldsNothing: with the records lock
// held elsewhere and a short lock wait, DeclareSeq returns 0 with the error,
// never a seq the ordered stream does not hold (LandExecution's own pin,
// TestREC_LandExecutionReturnsTheSeq).
func TestStore_DeclareSeqZeroWhenTheStreamHoldsNothing(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const session = "sess-held"
	if _, err := st.DeclareSeq(testDecl(session, "toolu_first")); err != nil {
		t.Fatal(err)
	}
	unlock, _, err := lockFileWait(openForAppend(t, filepath.Join(st.RunDir(session), FileRecords)), lockBudget)
	if err != nil {
		t.Fatalf("taking the records lock: %v", err)
	}
	st.LimitLockWaits(50 * time.Millisecond)
	seq, err := st.DeclareSeq(testDecl(session, "toolu_held"))
	unlock()
	if seq != 0 || !errors.Is(err, ErrLockTimeout) {
		t.Errorf("a declaration on a held lock: (%d, %v), want (0, ErrLockTimeout)", seq, err)
	}
	run, err := st.ReadRun(session)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Declarations) != 1 {
		t.Errorf("%d declarations on disk, want only the first", len(run.Declarations))
	}
}
