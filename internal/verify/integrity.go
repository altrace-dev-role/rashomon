package verify

import (
	"cmp"
	"slices"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// MarkerTableVersion is the version of markerTable (archive 20.2.6: "closed
// tables versioned by rules_version"). Marks of any EditMarks.Version are read
// the same: ids are only ever added, and one the table does not hold is
// counted in UnknownMarks and read no further.
const MarkerTableVersion = 1

// The two bases on which edit marks are a delta (archive 10.2.8; D42): an
// Edit's old_string against its new_string, and a Write's content against the
// counts read from the file before it. write_before_unread holds the new
// content's whole counts and notebook_added_only says nothing of what went.
const (
	basisEditDelta     = "edit_delta"
	basisWriteVsBefore = "write_vs_before"
)

// markClass is which of the four lexical checks a marker id counts for.
type markClass uint8

const (
	markSkip        markClass = iota + 1 // a skip or focus marker: adverse when its count goes up
	markTestDef                          // a test definition: adverse when it goes down
	markAssert                           // an assertion: adverse when it goes down while the tests do not
	markSuppression                      // a suppression pragma: adverse when it goes up
)

// checkOf is the check an adverse net of each class is unresolved under.
var checkOf = [...]IntegrityKind{markSkip: SkipAdded, markTestDef: TestRemoved, markAssert: AssertionsRemoved, markSuppression: SuppressionAdded}

// markerTable is the closed marker table (2.2.6; D41): archive 20.2.6's
// lang.marker ids, one marker per id, the id an IntegrityEntry and a finding
// carry. The skip and suppression ids are 20.2.6's verbatim. 20.2.6 describes
// test definitions and assertions by pattern and names no id, so they are
// minted here, and the producer (1.8p) adopts them: an id here is its
// contract. <lang>.test_def is minted for the eight languages 20.2.6 gives a
// test-definition pattern for, <lang>.assert for the four it gives an
// assertion pattern for; the other four assertion ids are 1.8p's to mint with
// their patterns. js covers TypeScript, as js.it_skip does; only the
// suppressions have ts ids.
//
// Archive 10.2.8's version-1 list for the same record field (pragma.*,
// skip.*) is superseded; no writer emitted it. Its secret.<rule> and
// stub.not_implemented stay RECORD-only ids in the same map, which integrity
// counts as unknown and reads no further.
func markerTable() map[string]markClass {
	markerTableOnce.Do(func() {
		markerTableOnce.v = map[string]markClass{
			"go.t_skip": markSkip, "py.pytest_skip": markSkip, "py.unittest_skip": markSkip, "py.xfail": markSkip,
			"js.it_skip": markSkip, "js.xit": markSkip, "js.only": markSkip, "rust.ignore": markSkip,
			"jvm.disabled": markSkip, "ruby.skip": markSkip, "php.skip": markSkip,

			"py.nosec": markSuppression, "py.noqa": markSuppression, "py.type_ignore": markSuppression,
			"py.pylint_disable": markSuppression, "go.nolint": markSuppression, "js.eslint_disable": markSuppression,
			"ts.ts_ignore": markSuppression, "ts.ts_expect_error": markSuppression, "rust.allow": markSuppression,
			"jvm.suppress_warnings": markSuppression, "any.nosemgrep": markSuppression, "any.gitleaks_allow": markSuppression,

			"go.test_def": markTestDef, "py.test_def": markTestDef, "js.test_def": markTestDef, "rust.test_def": markTestDef,
			"jvm.test_def": markTestDef, "ruby.test_def": markTestDef, "php.test_def": markTestDef, "dotnet.test_def": markTestDef,

			"go.assert": markAssert, "py.assert": markAssert, "js.assert": markAssert, "rust.assert": markAssert,
		}
	})
	return markerTableOnce.v
}

// markerTableOnce holds what markerTable returns, built on its first call and
// never at init (RES-2).
var markerTableOnce struct {
	sync.Once
	v map[string]markClass
}

// integrity is the Integrity view of the window (from, at] (2.2.6, Unresolved
// at a point; D39, D40): a pure fold over two sources, the edit marks the
// record carries on a write's execution and the tests_only_then_green pairs
// (testPairs, report's definition read over picked, D44). picked and writes
// are classify's: its pick of each call's execution, and its writes,
// project-relative and classed.
//
// Precondition: recording reads the record Complete at at, as evaluate
// establishes by returning first when it does not. Its
// outcome_without_declaration refuses every record holding a call by the
// point whose declaration was lost, a superset of the records
// testsOnlyThenGreen's guard voids (it also counts a flush_failed terminal
// with no id), so the pairs are read without the guard and its declared-id
// set is never built (1.8r2). A caller that has not established Complete
// reads testsOnlyThenGreen.
//
// The marks. A write in the window counts when it names a path and its call
// is a file tool's: a Bash line names its paths in WrittenPaths, one write
// each, and a call's counts cannot be split between them (10.2.8 gives marks
// to file-writing tools only). Its marks are those of the execution classify
// picks for its call, wherever that landed, since the prefix is by
// declaration (D37), and only when it ended ok. Marks on a basis that is not a delta are counted in
// UnreadBases and read no further (D42), as is an id outside markerTable in
// UnknownMarks (D41). Each id moves its (path, id) net by the write's Added
// less its Removed: skip, test-definition and assertion ids on test-class
// paths, suppression ids on source and test paths, and nowhere else. A net
// still adverse at the point is unresolved: a skip or a suppression up, a
// test definition down, an assertion down while the path's test definitions
// did not go down (where they did, test_removed says it). Its Seq is the last
// write that moved it the adverse way, and Marker its id. A later write that
// restores the count resolves it with no gate run; the re-run is the gate's
// Stale state's (D39).
//
// The pairs. One entry per tests_only_then_green pair whose later, passing
// run is in the window, with Marker "" and that run's Seq; it never resolves.
// verify reads no transcript, so a denied call reads as no execution record
// and stops a pattern rather than completing one, as report's detector does
// with denied nil.
//
// Counts are counts, not verdicts (20.2.6): TestsDelta is the signed net of
// test definitions across test-class paths and may be positive while a
// test_removed entry stands (one file lost three, another gained five);
// SkipsAdded and SuppressionsAdded sum each (path, id)'s net floored at zero,
// so each is positive exactly when an entry of its check stands. Unresolved
// is sorted by Seq, then check in IntegrityKinds order, then marker, and is
// never nil; nothing depends on the order of the run's records or of writes.
//
// Not read: generated_edited, gate_weakened and gate_config_edited, whose
// producers (the edit's first 1 KiB, the gate line's flags, a path list) are
// not in the record; and test_removed's second clause, a test file deleted by
// rm or git rm, which no record names. The gate_weakened producer is to
// record notARun's refusal reason (shape/verb.go), not to lex a second list.
// No writer fills edit marks yet (1.8p): until one does, only the pairs speak.
//
// v0 deviation (D40): window = the run until 1.8w; a skip added and never
// removed holds RecordGreen false for the session. Every caller passes from
// = 0 until 1.8w gives Evaluate the point's window start.
//
// Cost: one pass over the executions and declarations for the marks, one
// over writes, one over the declarations for the pairs (testPairs), and a
// sort of the entries; none for the lost-declaration guard, which is
// recording's.
func integrity(run *store.Run, picked map[string]*store.Execution, writes []write, from, at int64) Integrity {
	out := Integrity{Unresolved: []IntegrityEntry{}}
	nets := foldMarks(&out, marksByCall(run), writes, from, at)
	tests := make(map[string]int)
	for k, n := range nets {
		if n.class == markTestDef {
			tests[k.path] += n.net
		}
	}
	for k, n := range nets {
		switch n.class {
		case markTestDef:
			out.Counts.TestsDelta += n.net
		case markSkip:
			out.Counts.SkipsAdded += max(n.net, 0)
		case markSuppression:
			out.Counts.SuppressionsAdded += max(n.net, 0)
		}
		if adverse(n.class, n.net) && (n.class != markAssert || tests[k.path] >= 0) {
			out.Unresolved = append(out.Unresolved, IntegrityEntry{Check: checkOf[n.class], Marker: k.id, Seq: n.last})
		}
	}
	for _, p := range testPairs(run, picked, from, at) {
		out.Unresolved = append(out.Unresolved, IntegrityEntry{Check: TestsOnlyThenGreen, Seq: p[1]})
	}
	slices.SortFunc(out.Unresolved, func(a, b IntegrityEntry) int {
		return cmp.Or(cmp.Compare(a.Seq, b.Seq), cmp.Compare(slices.Index(IntegrityKinds, a.Check), slices.Index(IntegrityKinds, b.Check)),
			cmp.Compare(a.Marker, b.Marker))
	})
	return out
}

// marksByCall is the edit marks of each file-tool call (one whose declaration
// names a FilePath) by its declaration's Seq, read from the execution
// classify picks for it (outranks), and only when that execution's outcome is
// ok: a failed Edit lands nothing, and its marks would make a net no later
// edit can resolve, so a call whose pick is not ok is read no further and
// counted nowhere (1.8p computes marks at PostToolUse only). A call with a
// failed execution and an ok one, a retry under one tool_use_id, picks the
// failed one, classify's pick, and so reads none. Only executions of a call
// that carries marks somewhere are picked among.
func marksByCall(run *store.Run) map[int64]*store.EditMarks {
	marks := make(map[int64]*store.EditMarks)
	if run == nil {
		return marks
	}
	picked := make(map[string]*store.Execution)
	for i := range run.Executions {
		if run.Executions[i].EditMarks != nil {
			picked[run.Executions[i].ToolUseID] = nil
		}
	}
	for i := range run.Executions {
		e := &run.Executions[i]
		if cur, ok := picked[e.ToolUseID]; ok && (cur == nil || !outranks(cur, e)) {
			picked[e.ToolUseID] = e
		}
	}
	for i := range run.Declarations {
		d := &run.Declarations[i]
		if e := picked[d.ToolUseID]; e != nil && d.FilePath != nil && e.Outcome == store.ExecOK {
			marks[d.Seq] = e.EditMarks
		}
	}
	return marks
}

// markKey is one marker id on one path.
type markKey struct{ path, id string }

// markNet is one (path, id)'s class, its net count over the window's writes,
// and the last of them that moved it the adverse way.
type markNet struct {
	class markClass
	net   int
	last  int64
}

// foldMarks is every (path, id)'s net over the writes in (from, at] whose
// call carries marks, counting into out what it reads no further: a call
// whose basis is not a delta, an id outside the table. An id in both Added
// and Removed of one write moves once, by the difference.
func foldMarks(out *Integrity, marks map[int64]*store.EditMarks, writes []write, from, at int64) map[markKey]*markNet {
	nets := make(map[markKey]*markNet)
	for _, w := range writes {
		em := marks[w.Seq]
		if em == nil || w.Path == nil || w.Seq <= from || w.Seq > at {
			continue
		}
		if em.Basis != basisEditDelta && em.Basis != basisWriteVsBefore {
			out.UnreadBases++
			continue
		}
		for id, n := range em.Added {
			moveMark(nets, out, w, id, n-em.Removed[id])
		}
		for id, n := range em.Removed {
			if _, both := em.Added[id]; !both {
				moveMark(nets, out, w, id, -n)
			}
		}
	}
	return nets
}

// moveMark moves w's path's net of id by d where id is in the table and
// counts on w's path class, and records w as the last adverse write when d is
// the adverse way; an id outside the table is counted in out.UnknownMarks,
// once per write that carries it, so the count is how often the table missed.
func moveMark(nets map[markKey]*markNet, out *Integrity, w write, id string, d int) {
	class, ok := markerTable()[id]
	if !ok {
		out.UnknownMarks++
		return
	}
	if !markApplies(class, w.Class) {
		return
	}
	k := markKey{*w.Path, id}
	n := nets[k]
	if n == nil {
		n = &markNet{class: class}
		nets[k] = n
	}
	n.net += d
	if adverse(class, d) {
		n.last = max(n.last, w.Seq)
	}
}

// adverse reports whether a move, or a net, of d in a marker of class c is the
// adverse way: up for a skip or a suppression, down for a test definition or
// an assertion.
func adverse(c markClass, d int) bool {
	if c == markSkip || c == markSuppression {
		return d > 0
	}
	return d < 0
}

// markApplies reports whether a marker of class c counts on a path of class
// p: a suppression on source and test paths (20.2.6: "any code"), every other
// marker on test paths alone (20.2.6: "test-labelled").
func markApplies(c markClass, p PathClass) bool {
	if c == markSuppression {
		return p == ClassSource || p == ClassTest
	}
	return p == ClassTest
}
