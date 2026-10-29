package verify

import (
	"bytes"
	"cmp"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// stopNow is the Stop tests' clock: the index reads it, nothing else does.
var stopNow = time.UnixMilli(1_790_000_000_000)

// inPrompt names prompt id on every declaration of r and returns r. runOf
// names none.
func inPrompt(r *store.Run, id string) *store.Run {
	for i := range r.Declarations {
		r.Declarations[i].PromptID = ptr(id)
	}
	return r
}

// lastSeqOf is the greatest seq among r's records: the Stop point, computed
// here rather than through store's LastSeq, which these tests do not test.
func lastSeqOf(r *store.Run) int64 {
	var at int64
	for _, d := range r.Declarations {
		at = max(at, d.Seq)
	}
	for _, e := range r.Executions {
		if e.Seq != nil {
			at = max(at, *e.Seq)
		}
	}
	for _, x := range r.Terminals {
		if x.Seq != nil {
			at = max(at, *x.Seq)
		}
	}
	return at
}

// stopOf is EvaluateStop at r's last seq, for session, with no spec.
func stopOf(r *store.Run, idx *ProjectIndex, session string, in StopInput) StopView {
	return EvaluateStop(r, idx, nil, Point{Kind: PointStop, Seq: lastSeqOf(r), SessionID: session}, stopNow, in)
}

// classifiedOf is what evaluate classifies over r, with the two slices
// card 2.4 adds.
func classifiedOf(r *store.Run) classified {
	var c classified
	c.runs, c.writes, c.unnamed, c.rewritten = classifyWith(r, pickExecutions(r))
	return c
}

// TestVER65_ValueLineRowsAreOneFunctionOverTheSnapshot: every Row is reached
// from a fixture through EvaluateStop, whose Summary is TurnSummary over its
// own Snap and Turn, never a second reading; and Programs names every gate's
// program (card 2.4 R4, VER-65).
func TestVER65_ValueLineRowsAreOneFunctionOverTheSnapshot(t *testing.T) {
	masked := maskedCall(2)
	bg := gateCall(2, "go-test", store.ExecOK)
	bg.bg = true
	interrupted := gateCall(2, "go-test", store.ExecOK)
	interrupted.outcome = store.ExecInterrupted
	const testFile = "pkg/calc_test.go"
	for _, tc := range []struct {
		name  string
		calls []call
		want  Row
	}{
		{"verified", []call{edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)}, RowVerified},
		{"red", []call{edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecFailed)}, RowRed},
		{"masked", []call{edit(1, "pkg/a.go"), masked}, RowMasked},
		{"backgrounded", []call{edit(1, "pkg/a.go"), bg}, RowBackgrounded},
		{"ran only", []call{gateCall(1, "go-test", store.ExecOK)}, RowRanOnly},
		{"unverified", []call{gateCall(1, "go-test", store.ExecOK), edit(2, "pkg/a.go")}, RowUnverified},
		{"unknown", []call{edit(1, "pkg/a.go"), interrupted}, RowUnknown},
		{"no gate", []call{edit(1, "pkg/a.go")}, RowNoGate},
		{"unnamed write", []call{edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), unnamedWrite(3)}, RowUnnamedWrite},
		{"findings", []call{edit(1, testFile).marks(ids{"go.t_skip": 1}, nil), gateCall(2, "go-test", store.ExecOK)}, RowFindings},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := inPrompt(runOf(t, tc.calls...), "p1")
			v := stopOf(r, nil, "s1", StopInput{PromptID: "p1"})
			if v.Summary.Row != tc.want {
				t.Fatalf("row %q, want %q: turn %+v gates %+v", v.Summary.Row, tc.want, v.Turn, v.Snap.Gates)
			}
			if want := TurnSummary(v.Snap, v.Turn); v.Summary != want {
				t.Errorf("Summary %+v, want TurnSummary over the view's own Snap and Turn, %+v", v.Summary, want)
			}
			for _, g := range v.Snap.Gates {
				if v.Programs[g.Gate] != "go" {
					t.Errorf("Programs[%q] = %q, want go, its last run's program", g.Gate, v.Programs[g.Gate])
				}
			}
		})
	}
	t.Run("none", func(t *testing.T) {
		// The work is the previous prompt's; this prompt only read a file.
		r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK),
			call{seq: 3, tool: "Read", class: shape.VerbRead, outcome: store.ExecOK})
		inPrompt(r, "p0")
		r.Declarations[2].PromptID = ptr("p1")
		v := stopOf(r, nil, "s1", StopInput{PromptID: "p1"})
		if len(v.Snap.Gates) != 1 {
			t.Fatalf("gates %+v, want the earlier prompt's one gate in the snapshot", v.Snap.Gates)
		}
		if v.Summary.Row != RowNone || v.Turn != (TurnCounts{FirstSeq: 5}) {
			t.Errorf("row %q turn %+v, want none and nothing counted but the Read's seq: the prompt wrote nothing and ran no gate", v.Summary.Row, v.Turn)
		}
	})
}

// TestVER65e_IncompleteRecordingReachesTheUnknownRow: with the recording
// incomplete, evaluate classifies nothing, so EvaluateStop classifies the run
// itself for the turn's counts alone (D80): the row is unknown, not none, and
// the snapshot keeps no gate (card 2.4 R4, Gate A g16).
func TestVER65e_IncompleteRecordingReachesTheUnknownRow(t *testing.T) {
	r := inPrompt(runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)), "p1")
	r.Skipped = 1
	v := stopOf(r, nil, "s1", StopInput{PromptID: "p1"})
	if v.Summary.Row != RowUnknown || v.Summary.Sentence != sentenceRecordingIncomplete {
		t.Fatalf("row %q sentence %q, want unknown with the recording sentence: the turn wrote and ran a gate (turn %+v)",
			v.Summary.Row, v.Summary.Sentence, v.Turn)
	}
	if v.Turn.Writes != 1 || v.Turn.GateRuns != 1 {
		t.Errorf("turn %+v, want one write and one gate run counted", v.Turn)
	}
	if v.Snap.Recording != Incomplete || len(v.Snap.Gates) != 0 {
		t.Errorf("recording %s gates %+v, want incomplete with no gate: the extra classification feeds the counts and nothing else",
			v.Snap.Recording, v.Snap.Gates)
	}
}

// writeRunFile writes r's records to session's records.ndjson in st in seq
// order, spilled records last, as the store's reader expects to find them.
func writeRunFile(t *testing.T, st *store.Store, session string, r *store.Run) {
	t.Helper()
	type line struct {
		seq int64
		rec any
	}
	var lines []line
	for _, d := range r.Declarations {
		lines = append(lines, line{d.Seq, d})
	}
	for _, e := range r.Executions {
		lines = append(lines, line{intOr0(e.Seq), e})
	}
	for _, x := range r.Terminals {
		lines = append(lines, line{intOr0(x.Seq), x})
	}
	slices.SortStableFunc(lines, func(a, b line) int { return cmp.Compare(a.seq, b.seq) })
	var buf bytes.Buffer
	for _, l := range lines {
		b, err := json.Marshal(l.rec)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(append(b, '\n'))
	}
	for _, c := range r.Coverage {
		b, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(append(b, '\n'))
	}
	dir := st.RunDir(session)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, store.FileRecords), buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestVER67_CountTurnMatchesDigestsWindow: with no prompt named, countTurn
// counts the prompt digest's selectTurn chooses, the latest by first seq,
// not the one with the latest declaration (D72); and it counts only up to
// the point (card 2.4 R5).
func TestVER67_CountTurnMatchesDigestsWindow(t *testing.T) {
	// Declarations at seqs 1, 3, 5, 7, 9 and 11: prompt-a at 1; prompt-b at
	// 3 and 5 and again at 11 (a subagent's late call); prompt-c at 7 and 9.
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), edit(3, "pkg/b.go"),
		edit(4, "pkg/c.go"), edit(5, "pkg/d.go"), edit(6, "pkg/e.go"))
	for i, id := range []string{"prompt-a", "prompt-b", "prompt-b", "prompt-c", "prompt-c", "prompt-b"} {
		r.Declarations[i].PromptID = ptr(id)
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	writeRunFile(t, st, "s1", r)
	read, err := st.ReadRun("s1")
	if err != nil || len(read.Declarations) != 6 || read.Skipped != 0 {
		t.Fatalf("the fixture did not read back whole: %v, %d declarations, %d skipped", err, len(read.Declarations), read.Skipped)
	}
	d, err := digest.Build(st, "s1", "", "", stopNow)
	if err != nil {
		t.Fatal(err)
	}
	if d.PromptID != "prompt-c" {
		t.Fatalf("premise: digest chose %q, want prompt-c, the latest by first seq", d.PromptID)
	}

	c := classifiedOf(read)
	at := lastSeqOf(read)
	want := TurnCounts{Writes: 2, PathsWritten: 2, LastWriteSeq: 9, FirstSeq: 7}
	if got := countTurn(read, c, "", at); got != want {
		t.Errorf("countTurn with no prompt named = %+v, want prompt-c's %+v", got, want)
	}
	if got, named := countTurn(read, c, "", at), countTurn(read, c, d.PromptID, at); got != named {
		t.Errorf("countTurn with no prompt named = %+v, with digest's choice named = %+v: the two windows differ", got, named)
	}
	if got, want := countTurn(read, c, "prompt-b", at), (TurnCounts{Writes: 2, PathsWritten: 2, GateRuns: 1, LastWriteSeq: 11, FirstSeq: 3}); got != want {
		t.Errorf("countTurn for prompt-b = %+v, want %+v: its late declaration is in its window", got, want)
	}
	if got, want := countTurn(read, c, "", 8), (TurnCounts{Writes: 1, PathsWritten: 1, LastWriteSeq: 7, FirstSeq: 7}); got != want {
		t.Errorf("countTurn at seq 8 = %+v, want %+v: nothing after the point counts", got, want)
	}
}

// TestVER67b_NoPromptIdsIsAnEmptyWindow: a declaration whose prompt id is nil
// or "" is in no window and is never dereferenced; a run with no prompt id
// counts nothing, never the whole run (card 2.4 R5, Gate A g17).
func TestVER67b_NoPromptIdsIsAnEmptyWindow(t *testing.T) {
	r := runOf(t, edit(1, "pkg/a.go"), edit(2, "pkg/b.go"), gateCall(3, "go-test", store.ExecOK), edit(4, "pkg/c.go"))
	r.Declarations[1].PromptID = ptr("")
	r.Declarations[3].PromptID = ptr("p1")
	c := classifiedOf(r)
	at := lastSeqOf(r)
	if got, want := countTurn(r, c, "", at), (TurnCounts{Writes: 1, PathsWritten: 1, LastWriteSeq: 7, FirstSeq: 7}); got != want {
		t.Errorf("countTurn = %+v, want only p1's write %+v: a nil or empty prompt id is in no window", got, want)
	}

	r = runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK))
	c = classifiedOf(r)
	for _, id := range []string{"", "p1"} {
		if got := countTurn(r, c, id, lastSeqOf(r)); got != (TurnCounts{}) {
			t.Errorf("countTurn(%q) over a run with no prompt id = %+v, want nothing counted", id, got)
		}
	}
}

// TestVER68_EvaluateStopObservesTheIndex: EvaluateStop observes the turn's
// gate runs into the index under the stop's session, so a family run in a
// second session becomes required; with no index it starts one under the
// project key; with no project key it observes nothing (card 2.4 R4, R10).
func TestVER68_EvaluateStopObservesTheIndex(t *testing.T) {
	in := StopInput{PromptID: "p1", ProjectKey: "/work/repo"}
	v1 := stopOf(inPrompt(runOfSession(t, "s1", edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)), "p1"), nil, "s1", in)
	if v1.Index == nil || v1.Index.ProjectKey != in.ProjectKey || len(v1.Index.Entries) != 1 {
		t.Fatalf("index after the first session %+v, want one entry under %q", v1.Index, in.ProjectKey)
	}
	family := v1.Index.Entries[0].Family
	if e := v1.Index.Entries[0]; e.Sessions != 1 || e.LastSession != "s1" || v1.Index.Required(family, stopNow.UnixMilli()) {
		t.Fatalf("entry %+v, want one session, s1, not yet required", e)
	}

	v2 := stopOf(inPrompt(runOfSession(t, "s2", gateCall(1, "go-test", store.ExecOK)), "p1"), v1.Index, "s2", in)
	if v2.Index != v1.Index {
		t.Errorf("the second view's index is a new one, want the index it was given, observed in place")
	}
	if e := v2.Index.Entries[0]; e.Sessions != 2 || e.LastSession != "s2" || !v2.Index.Required(family, stopNow.UnixMilli()) {
		t.Errorf("entry %+v after the second session, want two sessions, s2, required", e)
	}

	v3 := stopOf(inPrompt(runOf(t, gateCall(1, "go-test", store.ExecOK)), "p1"), nil, "s1", StopInput{PromptID: "p1"})
	if v3.Index == nil || len(v3.Index.Entries) != 0 {
		t.Errorf("index with no project key %+v, want an empty one: nothing observed", v3.Index)
	}
}

// deliveredNote is a note as the model received it, bare; the claim words
// make a stripped line visible as a missing claim.
const deliveredNote = "tests pass after the last edit"

// TestVER69_ScanStripsDeliveredLines: EvaluateStop hands the delivered notes
// to Claims, which strips a line that quotes one, with the user mirror's "※"
// or bare (D81), and reads every other line (card 2.4 R4, R8b).
func TestVER69_ScanStripsDeliveredLines(t *testing.T) {
	r := inPrompt(runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)), "p1")
	delivered := [][]byte{[]byte(deliveredNote)}
	for _, c := range []struct {
		msg       string
		delivered [][]byte
		want      int
		why       string
	}{
		{deliveredNote, nil, 1, "nothing delivered: the line is a claim"},
		{"※ " + deliveredNote, delivered, 0, "the note behind the mirror's mark"},
		{deliveredNote, delivered, 0, "the note bare, as the model received it"},
		{"All 42 tests pass.\n" + deliveredNote, delivered, 1, "the agent's own claim beside the note"},
	} {
		v := stopOf(r, nil, "s1", StopInput{PromptID: "p1", Head: []byte(c.msg), Key: claimKey, Delivered: c.delivered})
		if len(v.Scan.Claims) != c.want {
			t.Errorf("%q: %d claims, want %d (%s)", c.msg, len(v.Scan.Claims), c.want, c.why)
		}
	}
}

// TestVER69b_BareQuotedNoteIsStripped: isDelivered's mark is optional (D81):
// a line that is, past its spaces, a delivered note byte for byte is no
// claim, bare as with the mark; a line that only contains the note, or is a
// prefix of it, is still read (card 2.4 R8b, Gate A g5).
func TestVER69b_BareQuotedNoteIsStripped(t *testing.T) {
	delivered := [][]byte{[]byte(deliveredNote)}
	for _, c := range []struct {
		msg  string
		want int
		why  string
	}{
		{deliveredNote, 0, "the bare note"},
		{"  " + deliveredNote + " \r", 0, "the bare note between spaces"},
		{"※ " + deliveredNote, 0, "the note behind the mark, as before"},
		{"Earlier: " + deliveredNote, 1, "the note inside a longer line"},
		{deliveredNote + ", twice", 1, "the note a prefix of the line"},
		{"tests pass after the last", 1, "the line a prefix of the note"},
	} {
		s := Claims([]byte(c.msg), nil, claimKey, allKinds(), true, SourceFinalMessage, delivered)
		if len(s.Claims) != c.want {
			t.Errorf("%q: %d claims, want %d (%s)", c.msg, len(s.Claims), c.want, c.why)
		}
	}
}
