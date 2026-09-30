package report

import (
	"sort"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Test-bending kinds, as the timeline's per-row annotation names them.
const (
	// BendTestsOnlyThenGreen: a test command failed, only files named like
	// tests were edited, and the same command then passed.
	BendTestsOnlyThenGreen = "tests_only_then_green"
	// BendFlaky: the same test command both passed and failed with no file
	// edit recorded between the two runs.
	BendFlaky = "flaky"
)

// SeqPair is two calls by their seq, earlier first. Seqs and nothing else: the
// pairs are found by comparing digests, and a digest is compared, never
// printed.
type SeqPair [2]int64

// TestBending is what two patterns in a list of calls look like, and nothing
// more than that. Neither says why: a test edited to pass may have been
// wrong, and a command that passes and fails may depend on the clock. They
// are the places a reader checking a "done" would want to look first.
//
// Both lists are in seq order of their later call, and are never nil, so a
// reader can tell "looked and found none" from a field that was not there.
type TestBending struct {
	// TestsOnlyThenGreen pairs a failed test run with the later run of the
	// SAME command that passed, where every file edit recorded between the
	// two was to a file named like a test, and at least one such edit ran.
	TestsOnlyThenGreen []SeqPair `json:"tests_only_then_green"`
	// Flaky pairs two consecutive runs of the SAME test command, one ok and
	// one failed in either order, with no file edit recorded between them.
	Flaky []SeqPair `json:"flaky"`
}

// DetectTestBending finds both patterns over run's declarations, in seq order,
// whatever agent made each call: a subagent re-running the main agent's tests
// is one session's work, and the seq is one total order over every agent. run
// is a whole session for the report and one turn's calls for the digest.
//
// Outcomes come from linkOutcome over run's executions and denied, so a
// call's outcome here is the one the timeline and the chains show for it.
// denied may be nil -- the turn digest reads no transcript -- and a denied
// call then reads as "no execution record", which below stops a pattern
// rather than completing one.
//
// A test run is a declaration of verb class test whose outcome is ok or
// failed. Interrupted, denied and unknown runs are not runs with a result, so
// they neither start nor finish a pair, and they do not separate two runs
// either: consecutive means consecutive among the runs with a result. Two runs
// are the same command when their shape digests are equal; the digest covers
// the tool name, so equal digests are the same tool as well.
//
// A file edit is any call of verb class write that was not denied before
// running: Edit, Write, MultiEdit and NotebookEdit, and a shell rm, mv, cp,
// tee and the like. A failed or unrecorded edit counts: whether it changed a
// file is exactly what the record cannot say, and assuming it did not would
// complete a pattern on a guess. It is a TEST edit only when it carries the
// test-file label and ran ok; every other edit -- a shell one, which has no
// label, and MultiEdit, which the label layer does not read -- is an edit to
// a file not named like a test, and stops pattern A. Stopping is the
// under-claim. Other shell commands are not seen as edits at all; the report
// states that limit.
func DetectTestBending(run *store.Run, denied map[string]bool) TestBending {
	out := TestBending{TestsOnlyThenGreen: []SeqPair{}, Flaky: []SeqPair{}}
	if run == nil {
		return out
	}
	executed := executionsByID(run)

	sorted := append([]store.Declaration(nil), run.Declarations...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Seq < sorted[j].Seq })

	// Running totals over the list so far, and what they were at each
	// command's last run with a result: the edits between two runs are the
	// difference.
	type mark struct {
		seq        int64
		failed     bool
		testEdits  int // test-file edits that ran ok
		otherEdits int // any other edit that may have run
		anyEdits   int // any edit that may have run
	}
	var testEdits, otherEdits, anyEdits int
	last := map[string]mark{}

	for _, d := range sorted {
		outcome, _, _ := linkOutcome(d.ToolUseID, executed, denied)
		switch d.Shape.VerbClass {
		case shape.VerbWrite:
			if outcome == LinkOutcomeDenied {
				continue
			}
			anyEdits++
			switch {
			case d.FileLabel == nil || *d.FileLabel != shape.LabelTestFile:
				otherEdits++
			case outcome == store.ExecOK:
				testEdits++
			}
			continue
		case shape.VerbTest:
		default:
			continue
		}
		if outcome != store.ExecOK && outcome != store.ExecFailed {
			continue
		}
		failed := outcome == store.ExecFailed
		if prev, ok := last[d.Shape.Digest]; ok {
			pair := SeqPair{prev.seq, d.Seq}
			switch {
			case prev.failed && !failed && otherEdits == prev.otherEdits && testEdits > prev.testEdits:
				out.TestsOnlyThenGreen = append(out.TestsOnlyThenGreen, pair)
			case prev.failed != failed && anyEdits == prev.anyEdits:
				out.Flaky = append(out.Flaky, pair)
			}
		}
		last[d.Shape.Digest] = mark{
			seq: d.Seq, failed: failed, testEdits: testEdits, otherEdits: otherEdits, anyEdits: anyEdits,
		}
	}
	return out
}

// TestRuns is the session's test runs and the two patterns among them.
type TestRuns struct {
	// Runs counts calls of verb class test that ended ok or failed. Records
	// written before the class existed say execute or package for the same
	// commands, so an old session reads 0 here: under-claimed, never
	// over-claimed.
	Runs   int `json:"runs"`
	OK     int `json:"ok"`
	Failed int `json:"failed"`
	TestBending
}

func buildTestRuns(run *store.Run, denied map[string]bool) TestRuns {
	executed := executionsByID(run)
	out := TestRuns{TestBending: DetectTestBending(run, denied)}
	for _, d := range run.Declarations {
		if d.Shape.VerbClass != shape.VerbTest {
			continue
		}
		switch o, _, _ := linkOutcome(d.ToolUseID, executed, denied); o {
		case store.ExecOK:
			out.Runs++
			out.OK++
		case store.ExecFailed:
			out.Runs++
			out.Failed++
		}
	}
	return out
}
