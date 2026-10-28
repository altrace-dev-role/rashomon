package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testSeqRecord stands in for verify.Checkpoint, which this package cannot
// import (verify is the caller): an event header whose kind and version the
// record names itself, and the seq AppendRecord fills.
type testSeqRecord struct {
	EventHeader
	Point string `json:"point"`
}

func (r *testSeqRecord) Kind() (string, int) { return r.Type, r.SchemaVersion }
func (r *testSeqRecord) SetSeq(seq *int64)   { r.Seq = seq }

func verifyRecord(session string) *testSeqRecord {
	return &testSeqRecord{EventHeader: EventHeader{Type: TypeVerify, SchemaVersion: EventSchemaVersion, SessionID: session}, Point: "stop"}
}

// recordLines is the run's records.ndjson, one entry per line.
func recordLines(t *testing.T, st *Store, session string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(st.RunDir(session), FileRecords))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}

// TestREC_AppendRecordSetsTheSeq: a record appended through AppendRecord
// takes the next seq under the run's lock, as every ordered record does, and
// its line carries it (card 2.4 R9).
func TestREC_AppendRecordSetsTheSeq(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendDeclaration(testDecl("s", "toolu_1")); err != nil {
		t.Fatal(err)
	}
	rec := verifyRecord("s")
	if err := st.AppendRecord("s", rec); err != nil {
		t.Fatalf("AppendRecord: %v", err)
	}
	lines := recordLines(t, st, "s")
	if len(lines) != 2 {
		t.Fatalf("records.ndjson holds %d lines, want the declaration and the record: %q", len(lines), lines)
	}
	var h EventHeader
	if err := json.Unmarshal([]byte(lines[1]), &h); err != nil {
		t.Fatalf("the appended line does not parse: %v: %q", err, lines[1])
	}
	if h.Type != TypeVerify || h.Seq == nil || *h.Seq != 2 {
		t.Fatalf("appended %q at seq %v, want %q at seq 2", h.Type, h.Seq, TypeVerify)
	}
	if rec.Seq == nil || *rec.Seq != 2 {
		t.Errorf("the record's own seq is %v after the append, want 2 (SetSeq)", rec.Seq)
	}
}

// TestREC_AppendRecordRefusesAnUnknownKind: a kind AcceptsKind does not know,
// or a known kind at a version it does not accept, is ErrUnknownKind and
// writes nothing.
func TestREC_AppendRecordRefusesAnUnknownKind(t *testing.T) {
	for _, c := range []struct {
		kind    string
		version int
	}{
		{"not-a-kind", EventSchemaVersion},
		{TypeVerify, EventSchemaVersion - 1},
	} {
		st, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		rec := &testSeqRecord{EventHeader: EventHeader{Type: c.kind, SchemaVersion: c.version, SessionID: "s"}}
		if err := st.AppendRecord("s", rec); !errors.Is(err, ErrUnknownKind) {
			t.Errorf("AppendRecord(%q, %d) = %v, want ErrUnknownKind", c.kind, c.version, err)
		}
		if lines := recordLines(t, st, "s"); len(lines) != 0 {
			t.Errorf("AppendRecord(%q, %d) wrote %q, want nothing", c.kind, c.version, lines)
		}
	}
}

// TestREC_LastSeqIsTheGreatestOverTheRun: LastSeq is the greatest seq over
// declarations, executions and terminals; a spilled execution or terminal
// carries no seq and is skipped, never dereferenced; an empty run, or one
// whose every seq is nil, is 0 (card 2.4 R3, QA B3).
func TestREC_LastSeqIsTheGreatestOverTheRun(t *testing.T) {
	seq := func(n int64) *int64 { return &n }
	run := &Run{
		Declarations: []Declaration{{Seq: 1}, {Seq: 3}},
		Executions:   []Execution{{Seq: seq(2)}, {Seq: nil}, {Seq: seq(4)}},
		Terminals:    []Terminal{{Seq: nil}, {Seq: seq(7)}},
	}
	if got := run.LastSeq(); got != 7 {
		t.Errorf("LastSeq = %d, want 7, the terminal's", got)
	}
	run.Terminals = []Terminal{{Seq: nil}}
	if got := run.LastSeq(); got != 4 {
		t.Errorf("LastSeq = %d with the terminal spilled, want 4, the execution's", got)
	}
	run.Executions = []Execution{{Seq: nil}}
	if got := run.LastSeq(); got != 3 {
		t.Errorf("LastSeq = %d with the execution spilled, want 3, the declaration's", got)
	}
	if got := (&Run{}).LastSeq(); got != 0 {
		t.Errorf("LastSeq of an empty run = %d, want 0", got)
	}
	if got := (&Run{Executions: []Execution{{}}, Terminals: []Terminal{{}}}).LastSeq(); got != 0 {
		t.Errorf("LastSeq of a run whose every seq is nil = %d, want 0", got)
	}
}

// TestREC_ReadRunPassesAVerifyLineUncounted: a checkpoint on the record
// changes no Run: ReadRun passes the "verify" line through its default
// branch, neither collected nor skipped nor ignored (card 2.4 R9).
func TestREC_ReadRunPassesAVerifyLineUncounted(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendDeclaration(testDecl("s", "toolu_1")); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendRecord("s", verifyRecord("s")); err != nil {
		t.Fatalf("AppendRecord: %v", err)
	}
	if lines := recordLines(t, st, "s"); len(lines) != 2 {
		t.Fatalf("records.ndjson holds %d lines, want the declaration and the checkpoint: %q", len(lines), lines)
	}
	run, err := st.ReadRun("s")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Declarations) != 1 || len(run.Executions) != 0 || len(run.Terminals) != 0 || len(run.Coverage) != 0 {
		t.Errorf("run holds %d declarations, %d executions, %d terminals, %d coverage, want the one declaration",
			len(run.Declarations), len(run.Executions), len(run.Terminals), len(run.Coverage))
	}
	if run.Skipped != 0 || run.Ignored != 0 {
		t.Errorf("Skipped %d Ignored %d, want 0 and 0: a verify line is an accepted kind", run.Skipped, run.Ignored)
	}
}
