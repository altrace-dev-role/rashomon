package verify

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// unended drops the execution of declaration i's call, as a call with no
// recorded ending leaves the record: its declaration, its PreToolUse
// terminal where the call literal built one, and nothing after them.
func unended(r *store.Run, i int) *store.Run {
	id := r.Declarations[i].ToolUseID
	r.Executions = slices.DeleteFunc(r.Executions, func(e store.Execution) bool { return e.ToolUseID == id })
	return r
}

// TestWEND1_EachEndingIsClassifiedOnce: a write is an edit only where its
// call's execution ended ok; every other may-write call is held with its
// ending and is in neither writes nor unnamed (classify). The table is the
// rule's: a failed file tool did not apply; a failed shell line or MCP call,
// and any call interrupted, may have written, the interrupt kept apart so
// the line can say the call was interrupted rather than that it failed; a
// call with no execution has no ending. Break: classify a call by its
// declaration alone, read a failed file tool as one that may have written,
// or read an interrupt as a failure.
func TestWEND1_EachEndingIsClassifiedOnce(t *testing.T) {
	interrupted := edit(1, "a.go")
	interrupted.outcome = store.ExecInterrupted
	failedSed := bashWrite(1, "a.go")
	failedSed.outcome, failedSed.exit = store.ExecFailed, 2
	failedBlind := unnamedWrite(1)
	failedBlind.outcome, failedBlind.exit = store.ExecFailed, 1
	partial := bashWrite(1, "a.go")
	partial.incomplete, partial.outcome, partial.exit = true, store.ExecFailed, 1
	mcp := call{seq: 1, tool: "mcp__fs__write", class: shape.VerbMCP, outcome: store.ExecFailed}
	failedWrite := writeCall(1, "a.go")
	failedWrite.outcome = store.ExecFailed
	type entry struct {
		named bool
		end   WriteEnd
	}
	for _, c := range []struct {
		name    string
		run     *store.Run
		writes  int
		unnamed int
		held    []entry
	}{
		{"an Edit that ended ok is an edit", runOf(t, edit(1, "a.go")), 1, 0, nil},
		{"a failed Edit did not apply", runOf(t, failedEdit(1, "a.go")), 0, 0, []entry{{true, EndFailedEdit}}},
		{"a failed Write did not apply", runOf(t, failedWrite), 0, 0, []entry{{true, EndFailedEdit}}},
		{"an interrupted Edit may have written, and was interrupted", runOf(t, interrupted), 0, 0, []entry{{true, EndInterrupted}}},
		{"an Edit with no execution has no ending", unended(runOf(t, edit(1, "a.go")), 0), 0, 0, []entry{{true, EndNone}}},
		{"a failed sed naming its file may have written", runOf(t, failedSed), 0, 0, []entry{{true, EndFailedCall}}},
		{"a failed may-write line naming nothing may have written", runOf(t, failedBlind), 0, 0, []entry{{false, EndFailedCall}}},
		{"a failed line whose list is partial holds its path and an unnamed entry", runOf(t, partial), 0, 0,
			[]entry{{true, EndFailedCall}, {false, EndFailedCall}}},
		{"a failed MCP call may have written", runOf(t, mcp), 0, 0, []entry{{false, EndFailedCall}}},
		{"an unended may-write line has no ending", unended(runOf(t, unnamedWrite(1)), 0), 0, 0, []entry{{false, EndNone}}},
		{"a failed read writes nothing", runOf(t, call{seq: 1, tool: "Read", class: shape.VerbRead, outcome: store.ExecFailed}), 0, 0, nil},
	} {
		got := classifiedOf(c.run)
		var held []entry
		for _, h := range got.held {
			held = append(held, entry{h.Path != nil, h.End})
			if h.Seq != declSeq(c.run, 0) || h.decl != &c.run.Declarations[0] {
				t.Errorf("%s: held %+v, want the call's own seq %d and declaration", c.name, h, declSeq(c.run, 0))
			}
		}
		if len(got.writes) != c.writes || len(got.unnamed) != c.unnamed || !slices.Equal(held, c.held) {
			t.Errorf("%s: writes %d, unnamed %d, held %+v; want %d, %d, %+v", c.name, len(got.writes), len(got.unnamed), held, c.writes, c.unnamed, c.held)
		}
	}
}

// TestWEND2_AHeldWriteMovesNoGate: go test passes after an edit, then a
// write that did not end ok: the gate stays green with nothing written since,
// and the snapshot's write fields count none of it; the same write ended ok
// stales the gate (the positive twin). Break: count held writes in the
// gate's writes.
func TestWEND2_AHeldWriteMovesNoGate(t *testing.T) {
	for _, c := range []struct {
		name string
		last call
		run  func(*store.Run) *store.Run
	}{
		{"a failed Edit", failedEdit(3, "pkg/a.go"), nil},
		{"an Edit with no ending", call{seq: 3, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{"pkg/a.go"}, term: store.OutcomeOK},
			func(r *store.Run) *store.Run { return unended(r, 2) }},
	} {
		r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), c.last)
		if c.run != nil {
			r = c.run(r)
		}
		snap := Evaluate(r, nil, nil, Point{Kind: PointStop, Seq: lastSeqOf(r)}, stopNow)
		if len(snap.Gates) != 1 || snap.Gates[0].State != Green || snap.Gates[0].WritesSinceGreen != 0 || snap.Gates[0].LastWrite != nil {
			t.Errorf("%s: gates %+v, want one green with no write since", c.name, snap.Gates)
		}
	}
	r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), edit(3, "pkg/a.go"))
	snap := Evaluate(r, nil, nil, Point{Kind: PointStop, Seq: lastSeqOf(r)}, stopNow)
	if len(snap.Gates) != 1 || snap.Gates[0].State != Stale || snap.Gates[0].WritesSinceGreen != 1 {
		t.Errorf("twin, an Edit that applied: gates %+v, want one stale by the write", snap.Gates)
	}
}

// TestWEND10_AHeldCallAfterTheGreenHoldsTheRecordGreenPoint: go test passes
// after an edit, then a call that may have written a file it covers and did
// not end ok. The gate stays green, since the record does not say the call
// wrote, but the record does not back a "done" (D36): the point, which the
// checkpoint and the next session's start line read, is not green. A failed
// Edit applied nothing and leaves it green, as does a call naming no path,
// which an edit naming none would not move either (D30), and one naming a
// file no gate covers. Break: read record green from the gates alone, or
// hold it on a failed edit.
func TestWEND10_AHeldCallAfterTheGreenHoldsTheRecordGreenPoint(t *testing.T) {
	sed := bashWrite(3, "pkg/a.go")
	sed.outcome, sed.exit = store.ExecFailed, 2
	readme := bashWrite(3, "README.md")
	readme.outcome, readme.exit = store.ExecFailed, 2
	interrupted := edit(3, "pkg/a.go")
	interrupted.outcome = store.ExecInterrupted
	open := call{seq: 3, tool: "Edit", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{"pkg/a.go"}, term: store.OutcomeOK}
	blind := unnamedWrite(3)
	blind.outcome, blind.exit = store.ExecFailed, 1
	for _, c := range []struct {
		name  string
		last  call
		unend bool
		green bool
	}{
		{"a failed sed of a covered file", sed, false, false},
		{"an interrupted Edit", interrupted, false, false},
		{"an Edit with no ending", open, true, false},
		{"a failed Edit, which applied nothing", failedEdit(3, "pkg/a.go"), false, true},
		{"a failed call naming no path", blind, false, true},
		{"a failed sed of a file no gate covers", readme, false, true},
	} {
		r := runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK), c.last)
		if c.unend {
			unended(r, 2)
		}
		snap := Evaluate(r, nil, nil, Point{Kind: PointStop, Seq: lastSeqOf(r)}, stopNow)
		if len(snap.Gates) != 1 || snap.Gates[0].State != Green || snap.RecordGreen != c.green || (snap.RecordGreenAtSeq != nil) != c.green {
			t.Errorf("%s: gates %+v, record green %v at %v; want one green gate and record green %v", c.name, snap.Gates, snap.RecordGreen, snap.RecordGreenAtSeq, c.green)
		}
	}

	// The turn's row does not wait on it: a later turn that edits a file the
	// gate does not cover and passes the gate that does reads verified, its
	// last edit tested, and never the findings row with nothing to count.
	snap := Snapshot{Recording: Complete, Gates: []GateState{
		{Gate: "a", Required: true, State: Green, LastGreen: &RunRef{Seq: 4}, heldSince: 6},
		{Gate: "b", Required: true, State: Green, LastGreen: &RunRef{Seq: 12}},
	}}
	snap.RecordGreen, _ = recordGreenOf(&snap)
	if got := TurnSummary(snap, TurnCounts{Writes: 1, PathsWritten: 1, GateRuns: 1, LastWriteSeq: 10, FirstSeq: 9}); snap.RecordGreen || got.Row != RowVerified {
		t.Errorf("an earlier turn's held call: record green %v, row %q; want false, verified", snap.RecordGreen, got.Row)
	}
}

// TestWEND3_TheTurnCountsHoldWhatBears: the turn's counts take a held write
// where an edit would bear (relevant and covered, or unnamed), the last of
// each kind, and count none of it as an edit. Break: count held writes as
// Writes, or drop the coverage test.
func TestWEND3_TheTurnCountsHoldWhatBears(t *testing.T) {
	sed := bashWrite(4, "pkg/b.go")
	sed.outcome, sed.exit = store.ExecFailed, 1
	r := inPrompt(runOf(t, edit(1, "pkg/a.go"), failedEdit(2, "pkg/a.go"), failedEdit(3, "README.md"), sed,
		edit(5, "pkg/c.go"), failedEdit(6, "pkg/d.go")), "p1")
	unended(r, 4)
	r.Declarations[5].FilePath = ptr("/elsewhere/d.go")
	r.Declarations[5].Project = ptr("/elsewhere")
	got := countIn(classifiedOf(r), windowOf(r), []string{"/repo"})
	want := TurnCounts{Writes: 1, PathsWritten: 1, LastWriteSeq: declSeq(r, 0), FailedEditSeq: declSeq(r, 1),
		FailedWriteSeq: declSeq(r, 3), FailedWriteEnd: EndFailedCall, UnendedWriteSeq: declSeq(r, 4)}
	if got != want {
		t.Errorf("counts %+v, want %+v: the failed sed is the last failed call a gate covers, kept apart from the failed Edit of pkg/a.go; the README.md Edit is no relevant write, and the failed Edit outside the project is no gate's", got, want)
	}
}

// windowOf is every declaration's seq of r, one turn.
func windowOf(r *store.Run) map[int64]bool {
	w := map[int64]bool{}
	for _, d := range r.Declarations {
		w[d.Seq] = true
	}
	return w
}

// TestWEND4_TheRowGuard: the row that would read verified, or ran_only's "no
// edits", names a call that did not end ok and bears on its green: a failed
// edit, or a call that failed or was interrupted and may have written,
// declared after the earliest last green of the turn's gates; one with no
// ending anywhere in the window; and under ran_only, which speaks for the
// whole turn, a call that may have written wherever it stands. A call that
// may have written is named before a failed edit, which applied nothing;
// of two that may have written, the later. A failed one before that green
// leaves verified as it is, the fail-then-retry turn. A turn that ran no
// gate and edited nothing has no line where it only failed to edit, names
// the call where one may have written, and reads unknown where the record
// is incomplete (D80). The row carries the green it placed the call by.
// Break: read a failed call wherever it stands, drop the unended call's
// reach before the green, let a failed edit hide a call that may have
// written, read ran_only's "no edits" past a call before the green, or
// leave the turn that ran no gate silent.
func TestWEND4_TheRowGuard(t *testing.T) {
	green := func(name string, lastGreen int64) GateState {
		return GateState{Gate: name, Required: true, State: Green, LastGreen: &RunRef{Seq: lastGreen}}
	}
	one := Snapshot{Recording: Complete, Gates: []GateState{green("a", 9)}, RecordGreen: true}
	two := Snapshot{Recording: Complete, Gates: []GateState{green("a", 5), green("b", 20)}, RecordGreen: true}
	for _, c := range []struct {
		name  string
		snap  Snapshot
		turn  TurnCounts
		row   Row
		seq   int64
		end   WriteEnd
		green int64
	}{
		{"a failed edit after the green", one, TurnCounts{Writes: 1, GateRuns: 1, FailedEditSeq: 12}, RowUnconfirmedWrite, 12, EndFailedEdit, 9},
		{"a failed edit before the green", one, TurnCounts{Writes: 1, GateRuns: 1, FailedEditSeq: 7}, RowVerified, 0, EndOK, 0},
		{"a failed call after the green", one, TurnCounts{Writes: 1, GateRuns: 1, FailedWriteSeq: 12, FailedWriteEnd: EndFailedCall}, RowUnconfirmedWrite, 12, EndFailedCall, 9},
		{"an interrupted call after the green", one, TurnCounts{Writes: 1, GateRuns: 1, FailedWriteSeq: 12, FailedWriteEnd: EndInterrupted}, RowUnconfirmedWrite, 12, EndInterrupted, 9},
		{"a failed call before the green, beside an edit", one, TurnCounts{Writes: 1, GateRuns: 1, FailedWriteSeq: 7, FailedWriteEnd: EndFailedCall}, RowVerified, 0, EndOK, 0},
		{"no ending, before the green", one, TurnCounts{Writes: 1, GateRuns: 1, UnendedWriteSeq: 3}, RowUnconfirmedWrite, 3, EndNone, 9},
		{"of two that may have written, the later", one, TurnCounts{Writes: 1, GateRuns: 1, UnendedWriteSeq: 3, FailedWriteSeq: 12, FailedWriteEnd: EndFailedCall}, RowUnconfirmedWrite, 12, EndFailedCall, 9},
		{"one that may have written before a later failed edit", one, TurnCounts{Writes: 1, GateRuns: 1, UnendedWriteSeq: 3, FailedEditSeq: 12}, RowUnconfirmedWrite, 3, EndNone, 9},
		{"a failed sed, then a failed edit, after the green", one, TurnCounts{Writes: 1, GateRuns: 1, FailedWriteSeq: 10, FailedWriteEnd: EndFailedCall, FailedEditSeq: 12}, RowUnconfirmedWrite, 10, EndFailedCall, 9},
		{"two greens at 5 and 20, a failed edit at 10: after the earliest", two, TurnCounts{Writes: 1, GateRuns: 2, FailedEditSeq: 10}, RowUnconfirmedWrite, 10, EndFailedEdit, 5},
		{"two greens at 5 and 20, a failed edit at 3: before both", two, TurnCounts{Writes: 1, GateRuns: 2, FailedEditSeq: 3}, RowVerified, 0, EndOK, 0},
		{"no edit, a failed edit after the green: never no edits", one, TurnCounts{GateRuns: 1, FailedEditSeq: 12}, RowUnconfirmedWrite, 12, EndFailedEdit, 9},
		{"no edit, a failed edit before the green", one, TurnCounts{GateRuns: 1, FailedEditSeq: 7}, RowRanOnly, 0, EndOK, 0},
		{"no edit, a failed call before the green: never no edits", one, TurnCounts{GateRuns: 1, FailedWriteSeq: 7, FailedWriteEnd: EndFailedCall}, RowUnconfirmedWrite, 7, EndFailedCall, 9},
		{"no edit, an interrupted call before the green", one, TurnCounts{GateRuns: 1, FailedWriteSeq: 7, FailedWriteEnd: EndInterrupted}, RowUnconfirmedWrite, 7, EndInterrupted, 9},
		{"an unnamed edit after the green keeps its row", one, TurnCounts{Writes: 1, GateRuns: 1, UnnamedWriteSeq: 11, FailedEditSeq: 12}, RowUnnamedWrite, 0, EndOK, 9},
		{"a red gate keeps its row", Snapshot{Recording: Complete, Gates: []GateState{{Gate: "a", Required: true, State: Red}}}, TurnCounts{Writes: 1, GateRuns: 1, UnendedWriteSeq: 12}, RowRed, 0, EndOK, 0},
		{"only a failed edit, no gate run: no line", Snapshot{Recording: Complete}, TurnCounts{FailedEditSeq: 2}, RowNone, 0, EndOK, 0},
		{"only a failed call, no gate run: named", Snapshot{Recording: Complete}, TurnCounts{FailedWriteSeq: 2, FailedWriteEnd: EndFailedCall}, RowUnconfirmedWrite, 2, EndFailedCall, 0},
		{"only an interrupted call, no gate run, a gate known: named", one, TurnCounts{FailedWriteSeq: 12, FailedWriteEnd: EndInterrupted}, RowUnconfirmedWrite, 12, EndInterrupted, 0},
		{"only an unended call in a complete record, no gate run: named", Snapshot{Recording: Complete}, TurnCounts{UnendedWriteSeq: 4, FailedEditSeq: 6}, RowUnconfirmedWrite, 4, EndNone, 0},
		{"only an unended edit in an incomplete record: unknown (D80)", Snapshot{Recording: Incomplete}, TurnCounts{UnendedWriteSeq: 1}, RowUnknown, 0, EndOK, 0},
		{"only a failed edit in an incomplete record: unknown (D80)", Snapshot{Recording: Incomplete}, TurnCounts{FailedEditSeq: 1}, RowUnknown, 0, EndOK, 0},
		{"nothing at all in an incomplete record: no line", Snapshot{Recording: Incomplete}, TurnCounts{}, RowNone, 0, EndOK, 0},
	} {
		got := TurnSummary(c.snap, c.turn)
		if got.Row != c.row || got.Unconfirmed != c.seq || got.Ending != c.end || got.GreenSeq != c.green {
			t.Errorf("%s: row %q call %d ending %d green %d, want %q, %d, %d, %d", c.name, got.Row, got.Unconfirmed, got.Ending, got.GreenSeq, c.row, c.seq, c.end, c.green)
		}
	}
}
