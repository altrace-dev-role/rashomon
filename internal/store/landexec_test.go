package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestREC_LandExecutionReturnsTheSeq: LandExecution returns the seq the
// ordered stream gave the execution, on a clean append and on a degraded one
// (the line is written, fitLine says so after), and 0 where the execution
// holds no place in the ordered stream: spilled because the lock was held, or
// refused whole as unfittable (card 2.3 R2, A5). The steer step cites the
// seq, so a seq for a record that is not in the ordered stream would cite a
// record that is not there.
func TestREC_LandExecutionReturnsTheSeq(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const session = "sess-land"
	exec := func(id string) Execution {
		return Execution{Type: TypeExecution, SchemaVersion: SchemaVersion, RecordedAtMS: time.Now().UnixMilli(),
			ToolUseID: id, SessionID: session, ToolName: "Bash", Outcome: ExecOK}
	}
	if err := st.AppendDeclaration(testDecl(session, "toolu_a")); err != nil {
		t.Fatal(err)
	}

	seq, err := st.LandExecution(exec("toolu_a"))
	if err != nil || seq != 2 {
		t.Errorf("an ordered execution after one declaration: (%d, %v), want (2, nil)", seq, err)
	}

	huge := strings.Repeat("<", MaxRecordBytes)
	deg := exec("toolu_b")
	deg.ExecutedCommand = &huge
	seq, err = st.LandExecution(deg)
	if !errors.Is(err, ErrRecordDegraded) || seq != 3 {
		t.Errorf("a degraded execution: (%d, %v), want (3, ErrRecordDegraded): the line was written", seq, err)
	}

	bad := exec("toolu_c")
	bad.Outcome = huge
	seq, err = st.LandExecution(bad)
	if !errors.Is(err, ErrRecordOversize) || seq != 0 {
		t.Errorf("an unfittable execution: (%d, %v), want (0, ErrRecordOversize)", seq, err)
	}

	dir := st.RunDir(session)
	unlock, _, err := lockFileWait(openForAppend(t, filepath.Join(dir, FileRecords)), lockBudget)
	if err != nil {
		t.Fatalf("taking the records lock: %v", err)
	}
	seq, err = st.LandExecution(exec("toolu_d"))
	unlock()
	if err != nil || seq != 0 {
		t.Errorf("an execution spilled on a held lock: (%d, %v), want (0, nil)", seq, err)
	}

	run, err := st.ReadRun(session)
	if err != nil {
		t.Fatal(err)
	}
	seqs := map[string]int64{}
	for _, x := range run.Executions {
		if x.Seq != nil {
			seqs[x.ToolUseID] = *x.Seq
		} else {
			seqs[x.ToolUseID] = 0
		}
	}
	if len(run.Executions) != 3 || seqs["toolu_a"] != 2 || seqs["toolu_b"] != 3 || seqs["toolu_d"] != 0 {
		t.Errorf("read back %v from %d executions; want toolu_a at 2, toolu_b at 3, toolu_d spilled, nothing for toolu_c",
			seqs, len(run.Executions))
	}
}
