package verify

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Seqs below are the builder's counter: with no terminal, call i has
// declaration 2i-1 and execution 2i, and a write's Seq is its declaration's.

// ids is a set of marker counts, for marks.
type ids = map[string]int

// integrityOf is integrity over r for the window (from, at], from classify's
// writes, as Evaluate will call it.
func integrityOf(r *store.Run, from, at int64) Integrity {
	_, writes, _, _ := classify(r)
	return integrity(r, pickExecutions(r), writes, from, at)
}

// labelTest gives declaration i the test-file label the hook derives for a
// path named like a test (shape.LabelTestFile), which report.DetectTestBending
// reads; the builder sets no label.
func labelTest(r *store.Run, i int) {
	l := shape.LabelTestFile
	r.Declarations[i].FileLabel = &l
}

// writeCall is a Write of path, whose marks are on write_vs_before.
func writeCall(seq int64, path string) call {
	return call{seq: seq, tool: "Write", class: shape.VerbWrite, outcome: store.ExecOK, paths: []string{path}}
}

func wantIntegrity(t *testing.T, got Integrity, counts IntegrityCounts, want ...IntegrityEntry) {
	t.Helper()
	if got.Unresolved == nil {
		t.Fatal("Unresolved is nil, want a list")
	}
	if !slices.Equal(got.Unresolved, want) {
		t.Errorf("unresolved\n got %+v\nwant %+v", got.Unresolved, want)
	}
	if got.Counts != counts {
		t.Errorf("counts %+v, want %+v", got.Counts, counts)
	}
}

func TestINT1_ATestDeletedAfterRedIsUnresolvedTestRemoved(t *testing.T) {
	r := runOf(t, gateCall(1, "go-test", store.ExecFailed), edit(2, "pkg/calc_test.go").marks(nil, ids{"go.test_def": 1}))
	wantIntegrity(t, integrityOf(r, 0, 3), IntegrityCounts{TestsDelta: -1}, IntegrityEntry{Check: TestRemoved, Marker: "go.test_def", Seq: 3})
	// Before the edit's declaration, the edit is not in the window.
	wantIntegrity(t, integrityOf(r, 0, 2), IntegrityCounts{})
}

func TestINT2_ASkipAddedIsSkipAddedWithItsMarker(t *testing.T) {
	// An Edit (edit_delta) and a Write (write_vs_before): both bases are a
	// delta.
	r := runOf(t, edit(1, "pkg/calc_test.go").marks(ids{"go.t_skip": 1}, nil), writeCall(2, "tests/test_calc.py").marks(ids{"py.xfail": 1}, nil))
	wantIntegrity(t, integrityOf(r, 0, 4), IntegrityCounts{SkipsAdded: 2},
		IntegrityEntry{Check: SkipAdded, Marker: "go.t_skip", Seq: 1}, IntegrityEntry{Check: SkipAdded, Marker: "py.xfail", Seq: 3})
}

func TestINT3_ARestoringEditResolves(t *testing.T) {
	const p = "pkg/calc_test.go"
	add, remove := ids{"go.t_skip": 1}, ids{"go.t_skip": 1}
	t.Run("one add", func(t *testing.T) {
		r := runOf(t, gateCall(1, "go-test", store.ExecFailed), edit(2, p).marks(add, nil), edit(3, p).marks(nil, remove))
		wantIntegrity(t, integrityOf(r, 0, 4), IntegrityCounts{SkipsAdded: 1}, IntegrityEntry{Check: SkipAdded, Marker: "go.t_skip", Seq: 3})
		// Restored with no gate run between: resolved (D39).
		wantIntegrity(t, integrityOf(r, 0, 6), IntegrityCounts{})
	})
	t.Run("two adds", func(t *testing.T) {
		r := runOf(t, gateCall(1, "go-test", store.ExecFailed), edit(2, p).marks(add, nil), edit(3, p).marks(add, nil),
			edit(4, p).marks(nil, remove), edit(5, p).marks(nil, remove))
		// The entry's Seq is the last write that moved it the adverse way, 5,
		// and it stays 5 while one removal leaves the net adverse.
		wantIntegrity(t, integrityOf(r, 0, 6), IntegrityCounts{SkipsAdded: 2}, IntegrityEntry{Check: SkipAdded, Marker: "go.t_skip", Seq: 5})
		wantIntegrity(t, integrityOf(r, 0, 8), IntegrityCounts{SkipsAdded: 1}, IntegrityEntry{Check: SkipAdded, Marker: "go.t_skip", Seq: 5})
		wantIntegrity(t, integrityOf(r, 0, 10), IntegrityCounts{})
	})
}

func TestINT4_TestsOnlyThenGreenIsUnresolvedAndNeverResolves(t *testing.T) {
	r := runOf(t, gateCall(1, "go-test", store.ExecFailed), edit(2, "pkg/calc_test.go"), gateCall(3, "go-test", store.ExecOK), edit(4, "pkg/calc.go"))
	labelTest(r, 1)
	pair := IntegrityEntry{Check: TestsOnlyThenGreen, Marker: "", Seq: 5}
	wantIntegrity(t, integrityOf(r, 0, 4), IntegrityCounts{})
	wantIntegrity(t, integrityOf(r, 0, 5), IntegrityCounts{}, pair)
	// A later edit to a source file does not undo it.
	wantIntegrity(t, integrityOf(r, 0, 8), IntegrityCounts{}, pair)
}

// int5Run is four edits with marks around a tests_only_then_green pair.
func int5Run(t *testing.T) *store.Run {
	const p = "pkg/calc_test.go"
	r := runOf(t,
		gateCall(1, "go-test", store.ExecFailed),
		edit(2, p).marks(ids{"go.t_skip": 1}, nil),
		gateCall(3, "go-test", store.ExecOK),
		edit(4, "pkg/calc.go").marks(ids{"go.nolint": 1}, nil),
		edit(5, p).marks(nil, ids{"go.t_skip": 1, "go.test_def": 1}),
		edit(6, p).marks(ids{"go.test_def": 1}, ids{"go.assert": 2}),
	)
	labelTest(r, 1)
	return r
}

func TestINT5_EvaluateAtAPointEqualsThePrefix(t *testing.T) {
	full := int5Run(t)
	for _, from := range []int64{0, 3} {
		for s := int64(0); s <= 13; s++ {
			a, err := json.Marshal(integrityOf(full, from, s))
			if err != nil {
				t.Fatal(err)
			}
			b, err := json.Marshal(integrityOf(prefixOf(full, s), from, s))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(a, b) {
				t.Errorf("from %d at %d: the full record gives %s, the prefix %s", from, s, a, b)
			}
		}
	}
	// Not vacuous: the run speaks at its end.
	wantIntegrity(t, integrityOf(full, 0, 12), IntegrityCounts{SuppressionsAdded: 1},
		IntegrityEntry{Check: TestsOnlyThenGreen, Seq: 5}, IntegrityEntry{Check: SuppressionAdded, Marker: "go.nolint", Seq: 7},
		IntegrityEntry{Check: AssertionsRemoved, Marker: "go.assert", Seq: 11})
}

func TestINT6_UnknownMarkerAndNilMarksAreIgnored(t *testing.T) {
	const p = "pkg/calc_test.go"
	t.Run("an unknown id and nil marks", func(t *testing.T) {
		// skip.go is archive 10.2.8's version-1 id for a Go skip, which D41
		// supersedes: counted as unknown, never read as go.t_skip.
		r := runOf(t, edit(1, p).marks(ids{"skip.go": 1}, nil), edit(2, p))
		got := integrityOf(r, 0, 4)
		wantIntegrity(t, got, IntegrityCounts{})
		if got.UnknownMarks != 1 || got.UnreadBases != 0 {
			t.Errorf("unknown marks %d, unread bases %d, want 1 and 0", got.UnknownMarks, got.UnreadBases)
		}
		b, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), `"unresolved":[]`) {
			t.Errorf("an empty Unresolved marshals as %s, want []", b)
		}
	})
	t.Run("a RECORD-only id taken away", func(t *testing.T) {
		// stub.not_implemented stays a RECORD id in the same map (D41): an id
		// outside the table is counted and read no further, in Removed as in
		// Added.
		r := runOf(t, edit(1, p).marks(nil, ids{"stub.not_implemented": 1}))
		got := integrityOf(r, 0, 2)
		wantIntegrity(t, got, IntegrityCounts{})
		if got.UnknownMarks != 1 {
			t.Errorf("unknown marks %d, want 1", got.UnknownMarks)
		}
	})
	t.Run("a basis that is not a delta", func(t *testing.T) {
		// write_before_unread holds the new content's whole counts, and
		// notebook_added_only and an unknown basis say nothing of what went:
		// neither is a delta (D42).
		r := runOf(t, writeCall(1, p).marks(ids{"go.t_skip": 1}, ids{"go.test_def": 3}), writeCall(2, p).marks(ids{"go.t_skip": 1}, nil), edit(3, p).marks(ids{"go.t_skip": 1}, nil))
		r.Executions[0].EditMarks.Basis = "write_before_unread"
		r.Executions[1].EditMarks.Basis = "notebook_added_only"
		r.Executions[2].EditMarks.Basis = "line_delta"
		got := integrityOf(r, 0, 6)
		wantIntegrity(t, got, IntegrityCounts{})
		if got.UnreadBases != 3 || got.UnknownMarks != 0 {
			t.Errorf("unread bases %d, unknown marks %d, want 3 and 0", got.UnreadBases, got.UnknownMarks)
		}
	})
	t.Run("marks on a shell line", func(t *testing.T) {
		// One write per WrittenPath, and a call's counts cannot be split
		// between them; 10.2.8 gives marks to file-writing tools only.
		r := runOf(t, bashWrite(1, p, "pkg/other_test.go"))
		r.Executions[0].EditMarks = &store.EditMarks{Added: ids{"go.t_skip": 1}, Basis: "edit_delta", Version: 1}
		got := integrityOf(r, 0, 2)
		wantIntegrity(t, got, IntegrityCounts{})
		if got.UnknownMarks != 0 || got.UnreadBases != 0 {
			t.Errorf("unknown marks %d, unread bases %d, want 0 and 0", got.UnknownMarks, got.UnreadBases)
		}
	})
	t.Run("marks on a call classify cannot name", func(t *testing.T) {
		// An MCP call is an unnamed write (D30) whatever its declaration
		// carries: no path, so its marks are read no further, as a shell
		// line's are; not even an unknown id is counted.
		r := runOf(t, call{seq: 1, tool: "mcp__fs__write", class: shape.VerbMCP, outcome: store.ExecOK})
		fp := p
		r.Declarations[0].FilePath = &fp
		r.Executions[0].EditMarks = &store.EditMarks{Added: ids{"go.t_skip": 1, "skip.go": 1}, Basis: "edit_delta", Version: 1}
		got := integrityOf(r, 0, 2)
		wantIntegrity(t, got, IntegrityCounts{})
		if got.UnknownMarks != 0 || got.UnreadBases != 0 {
			t.Errorf("unknown marks %d, unread bases %d, want 0 and 0", got.UnknownMarks, got.UnreadBases)
		}
	})
	t.Run("a failed execution beside an ok one", func(t *testing.T) {
		// The marks are the picked execution's, classify's pick (outranks):
		// the failed one, which carries none, whichever was read last.
		r := runOf(t, edit(1, p).marks(ids{"go.t_skip": 1}, nil))
		failed := r.Executions[0]
		failed.Outcome, failed.EditMarks = store.ExecFailed, nil
		r.Executions = []store.Execution{failed, r.Executions[0]}
		wantIntegrity(t, integrityOf(r, 0, 2), IntegrityCounts{})
	})
	t.Run("a failed edit carrying marks", func(t *testing.T) {
		// A failed Edit lands nothing: marks on it would make a net no later
		// edit resolves, so they are read no further and counted nowhere,
		// not even an unknown id.
		c := edit(1, p).marks(ids{"go.t_skip": 1, "skip.go": 1}, nil)
		c.outcome = store.ExecFailed
		got := integrityOf(runOf(t, c), 0, 2)
		wantIntegrity(t, got, IntegrityCounts{})
		if got.UnknownMarks != 0 || got.UnreadBases != 0 {
			t.Errorf("unknown marks %d, unread bases %d, want 0 and 0", got.UnknownMarks, got.UnreadBases)
		}
	})
	t.Run("a nil run", func(t *testing.T) {
		wantIntegrity(t, integrity(nil, nil, nil, 0, 10), IntegrityCounts{})
	})
}

func TestINT7_ClassDecidesWhichChecksApply(t *testing.T) {
	r := runOf(t,
		edit(1, "pkg/calc.go").marks(ids{"go.t_skip": 1, "go.nolint": 1}, ids{"go.test_def": 1, "go.assert": 1}),
		edit(2, "pkg/calc_test.go").marks(ids{"go.nolint": 1}, nil),
		edit(3, "docs/guide.md").marks(ids{"go.nolint": 1, "go.t_skip": 1}, ids{"go.test_def": 1}),
	)
	// A skip, a test definition and an assertion count on test paths alone;
	// a suppression on source and test paths; nothing on docs.
	wantIntegrity(t, integrityOf(r, 0, 6), IntegrityCounts{SuppressionsAdded: 2},
		IntegrityEntry{Check: SuppressionAdded, Marker: "go.nolint", Seq: 1}, IntegrityEntry{Check: SuppressionAdded, Marker: "go.nolint", Seq: 3})
}

// int8Run has entries that tie on Seq and differ by check, and tie on Seq
// and check and differ by marker, beside a pair and a removal.
func int8Run(t *testing.T) *store.Run {
	r := runOf(t,
		gateCall(1, "go-test", store.ExecFailed),
		edit(2, "pkg/calc_test.go"),
		gateCall(3, "go-test", store.ExecOK),
		edit(4, "pkg/a_test.go").marks(nil, ids{"go.test_def": 2}),
		edit(5, "pkg/b_test.go").marks(ids{"py.xfail": 1, "go.nolint": 1, "go.t_skip": 1}, nil),
		edit(6, "pkg/c.go").marks(ids{"rust.allow": 1}, nil),
	)
	labelTest(r, 1)
	return r
}

func TestINT8_RecordOrderDoesNotChangeTheOutput(t *testing.T) {
	r := int8Run(t)
	_, writes, _, _ := classify(r)
	got := integrity(r, pickExecutions(r), writes, 0, 12)
	wantIntegrity(t, got, IntegrityCounts{TestsDelta: -2, SkipsAdded: 2, SuppressionsAdded: 2},
		IntegrityEntry{Check: TestsOnlyThenGreen, Seq: 5},
		IntegrityEntry{Check: TestRemoved, Marker: "go.test_def", Seq: 7},
		IntegrityEntry{Check: SkipAdded, Marker: "go.t_skip", Seq: 9},
		IntegrityEntry{Check: SkipAdded, Marker: "py.xfail", Seq: 9},
		IntegrityEntry{Check: SuppressionAdded, Marker: "go.nolint", Seq: 9},
		IntegrityEntry{Check: SuppressionAdded, Marker: "rust.allow", Seq: 11})
	want, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(1))
	for i := range 100 {
		rng.Shuffle(len(r.Declarations), func(i, j int) { r.Declarations[i], r.Declarations[j] = r.Declarations[j], r.Declarations[i] })
		rng.Shuffle(len(r.Executions), func(i, j int) { r.Executions[i], r.Executions[j] = r.Executions[j], r.Executions[i] })
		rng.Shuffle(len(writes), func(i, j int) { writes[i], writes[j] = writes[j], writes[i] })
		// picked points into r.Executions, which the shuffle moves: it is
		// picked again over the shuffled record.
		b, err := json.Marshal(integrity(r, pickExecutions(r), writes, 0, 12))
		if err != nil || !bytes.Equal(b, want) {
			t.Fatalf("shuffle %d: %s, want %s", i, b, want)
		}
	}
}

func TestINT9_CountsAreNetPerPathAndMarker(t *testing.T) {
	const a, b = "pkg/a_test.go", "pkg/b_test.go"
	r := runOf(t,
		edit(1, a).marks(ids{"go.t_skip": 2, "go.nolint": 1}, ids{"go.test_def": 3}),
		edit(2, b).marks(ids{"go.test_def": 5, "go.t_skip": 1}, ids{"go.nolint": 1}),
		edit(3, a).marks(nil, ids{"go.t_skip": 3}),
		edit(4, "pkg/a.go").marks(ids{"go.nolint": 2}, ids{"go.test_def": 4}),
		// One skip swapped for another moves B's net by nothing, and is not
		// the write that moved it the adverse way.
		edit(5, b).marks(ids{"go.t_skip": 1}, ids{"go.t_skip": 1}),
	)
	// TestsDelta is the signed headcount across test paths, A's -3 and B's +5,
	// positive beside A's test_removed; a source file's test definitions are
	// not counted. SkipsAdded floors A's -1 at zero and takes B's 1;
	// SuppressionsAdded floors B's -1 and takes A's 1 and a.go's 2. Each of
	// the two is positive exactly when an entry of its check stands.
	wantIntegrity(t, integrityOf(r, 0, 10), IntegrityCounts{TestsDelta: 2, SkipsAdded: 1, SuppressionsAdded: 3},
		IntegrityEntry{Check: TestRemoved, Marker: "go.test_def", Seq: 1},
		IntegrityEntry{Check: SuppressionAdded, Marker: "go.nolint", Seq: 1},
		IntegrityEntry{Check: SkipAdded, Marker: "go.t_skip", Seq: 3},
		IntegrityEntry{Check: SuppressionAdded, Marker: "go.nolint", Seq: 7})
}

func TestINT10_UnresolvedBlocksRecordGreenAndRowsFindings(t *testing.T) {
	const p = "pkg/calc_test.go"
	turn := TurnCounts{Writes: 2, PathsWritten: 1, GateRuns: 1}
	t.Run("unresolved", func(t *testing.T) {
		// A skip and a suppression added, then the gate green after the edit:
		// every gate green, and no record-green point.
		snap := evalAt(runOf(t, edit(1, p).marks(ids{"go.t_skip": 1, "go.nolint": 1}, nil), gateCall(2, "go-test", store.ExecOK)), 4)
		if len(snap.Gates) != 1 || snap.Gates[0].State != Green || len(snap.Integrity.Unresolved) != 2 {
			t.Fatalf("gates %+v and integrity %+v, want one green gate and two unresolved", snap.Gates, snap.Integrity)
		}
		wantNoGreen(t, snap)
		// The sentence is a count, never a check or a marker (Gate B F1).
		if s := TurnSummary(snap, turn); s.Row != RowFindings || s.Sentence != "2 record finding(s) in this turn" {
			t.Errorf("row %q sentence %q, want %q and %q", s.Row, s.Sentence, RowFindings, "2 record finding(s) in this turn")
		}
	})
	t.Run("resolved", func(t *testing.T) {
		snap := evalAt(runOf(t, edit(1, p).marks(ids{"go.t_skip": 1}, nil), edit(2, p).marks(nil, ids{"go.t_skip": 1}), gateCall(3, "go-test", store.ExecOK)), 6)
		if len(snap.Integrity.Unresolved) != 0 {
			t.Fatalf("integrity %+v, want none unresolved", snap.Integrity)
		}
		wantGreenAt(t, snap, 5)
		if s := TurnSummary(snap, turn); s.Row != RowVerified || s.Sentence != "" {
			t.Errorf("row %q sentence %q, want %q and none", s.Row, s.Sentence, RowVerified)
		}
	})
}

func TestINT11_AssertionsRemovedOnlyWhileTestsStay(t *testing.T) {
	const zero, fell, rose = "pkg/zero_test.go", "pkg/fell_test.go", "pkg/rose_test.go"
	r := runOf(t,
		// Net 0 by a test added and another taken away: an entry.
		edit(1, zero).marks(ids{"go.test_def": 1}, ids{"go.assert": 1}),
		edit(2, zero).marks(nil, ids{"go.test_def": 1, "go.assert": 1}),
		// Net -1: test_removed says it, and assertions_removed does not.
		edit(3, fell).marks(nil, ids{"go.test_def": 1, "go.assert": 2}),
		// Net +5, five tests added and two assertions taken: the count of
		// tests did not go down, so an entry.
		edit(4, rose).marks(ids{"go.test_def": 5}, ids{"go.assert": 2}),
	)
	wantIntegrity(t, integrityOf(r, 0, 8), IntegrityCounts{TestsDelta: 4},
		IntegrityEntry{Check: AssertionsRemoved, Marker: "go.assert", Seq: 3},
		IntegrityEntry{Check: TestRemoved, Marker: "go.test_def", Seq: 5},
		IntegrityEntry{Check: AssertionsRemoved, Marker: "go.assert", Seq: 7})
}

func TestINT12_EverySpecMarkerIdIsInTheTableAndValid(t *testing.T) {
	want := map[markClass][]string{
		markSkip: {"go.t_skip", "py.pytest_skip", "py.unittest_skip", "py.xfail", "js.it_skip", "js.xit", "js.only", "rust.ignore",
			"jvm.disabled", "ruby.skip", "php.skip"},
		markSuppression: {"py.nosec", "py.noqa", "py.type_ignore", "py.pylint_disable", "go.nolint", "js.eslint_disable", "ts.ts_ignore",
			"ts.ts_expect_error", "rust.allow", "jvm.suppress_warnings", "any.nosemgrep", "any.gitleaks_allow"},
		markTestDef: {"go.test_def", "py.test_def", "js.test_def", "rust.test_def", "jvm.test_def", "ruby.test_def", "php.test_def", "dotnet.test_def"},
		markAssert:  {"go.assert", "py.assert", "js.assert", "rust.assert"},
	}
	n := 0
	seen := map[string]bool{}
	for class, list := range want {
		for _, id := range list {
			if seen[id] {
				t.Errorf("%s is listed twice", id)
			}
			seen[id] = true
			n++
			if got, ok := markerTable()[id]; !ok || got != class {
				t.Errorf("%s: class %d (present %t), want %d", id, got, ok, class)
			}
			if !MarkerID(id).Valid() {
				t.Errorf("%s is outside MarkerID's charset", id)
			}
		}
	}
	if n != 35 || len(markerTable()) != n {
		t.Errorf("the table holds %d ids, the list %d, want 35 and 35: 11 skip, 12 suppression, 8 test_def, 4 assert", len(markerTable()), n)
	}
	if MarkerTableVersion != 1 {
		t.Errorf("MarkerTableVersion %d, want 1", MarkerTableVersion)
	}
}

func TestINT13_AWriteAtOrBeforeTheWindowStartIsNotInTheWindow(t *testing.T) {
	const p = "pkg/calc_test.go"
	r := runOf(t,
		gateCall(1, "go-test", store.ExecFailed),
		edit(2, p).marks(ids{"go.t_skip": 1, "skip.go": 1}, nil),
		gateCall(3, "go-test", store.ExecOK),
		writeCall(4, p).marks(ids{"go.nolint": 1}, nil),
	)
	labelTest(r, 1)
	r.Executions[3].EditMarks.Basis = "write_before_unread"
	skip, pair := IntegrityEntry{Check: SkipAdded, Marker: "go.t_skip", Seq: 3}, IntegrityEntry{Check: TestsOnlyThenGreen, Seq: 5}
	for _, c := range []struct {
		from          int64
		want          []IntegrityEntry
		counts        IntegrityCounts
		unknown, base int
	}{
		{2, []IntegrityEntry{skip, pair}, IntegrityCounts{SkipsAdded: 1}, 1, 1},
		// The window is (from, at]: the skip at 3 and the pair's passing run
		// at 5 are each out of it once from reaches them.
		{3, []IntegrityEntry{pair}, IntegrityCounts{}, 0, 1},
		{4, []IntegrityEntry{pair}, IntegrityCounts{}, 0, 1},
		{5, nil, IntegrityCounts{}, 0, 1},
		{7, nil, IntegrityCounts{}, 0, 0},
	} {
		got := integrityOf(r, c.from, 8)
		if c.want == nil {
			c.want = []IntegrityEntry{}
		}
		if !slices.Equal(got.Unresolved, c.want) || got.Counts != c.counts || got.UnknownMarks != c.unknown || got.UnreadBases != c.base {
			t.Errorf("from %d: %+v, want %+v with counts %+v, %d unknown, %d unread", c.from, got, c.want, c.counts, c.unknown, c.base)
		}
	}
}

// benchIntegrityRun is 100 edits with marks over two test files, between a
// failed and a passing go test, and the point after its last record.
func benchIntegrityRun(tb testing.TB) (*store.Run, int64) {
	calls := []call{gateCall(1, "go-test", store.ExecFailed)}
	for i := range 100 {
		p := "pkg/a_test.go"
		if i%2 == 1 {
			p = "pkg/b_test.go"
		}
		added, removed := ids{"go.t_skip": 1, "go.nolint": 1}, ids{"go.assert": 1}
		if i%4 >= 2 {
			added, removed = ids{"go.test_def": 1}, ids{"go.t_skip": 1, "go.nolint": 1}
		}
		calls = append(calls, edit(int64(i+2), p).marks(added, removed))
	}
	calls = append(calls, gateCall(102, "go-test", store.ExecOK))
	r := runOf(tb, calls...)
	for i := 1; i <= 100; i++ {
		labelTest(r, i)
	}
	return r, int64(2 * len(calls))
}

// BenchmarkIntegrity is integrity at the end of benchIntegrityRun: the fold
// over every write and the pair detector over every declaration, with the
// pick Evaluate shares with classify built once outside the loop. Evaluate
// computes it once per point.
func BenchmarkIntegrity(b *testing.B) {
	// A top-level benchmark, not a b.Run: the name with a sub-benchmark's
	// suffix reads as a secret to internal/secretshape's entropy test, which
	// TestH113 runs over test/perf/budgets.json.
	r, at := benchIntegrityRun(b)
	_, writes, _, _ := classify(r)
	picked := pickExecutions(r)
	if got := integrity(r, picked, writes, 0, at); len(got.Unresolved) != 3 || got.Counts.TestsDelta != 50 {
		b.Fatalf("the fixture gives %+v, want three entries (the pair and two paths' assertions) and TestsDelta 50", got)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		integrity(r, picked, writes, 0, at)
	}
	b.ReportMetric(float64(len(writes)), "writes")
}
