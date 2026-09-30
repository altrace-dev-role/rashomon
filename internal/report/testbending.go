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
	// edit recorded between the two runs (see mayEdit for what one is).
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
	// two was an edit to a file named like a test, and at least one such
	// edit ran.
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
// they neither start nor finish a pair. Two runs are the same command when
// their shape digests are equal; the digest covers the tool name, so equal
// digests are the same tool as well.
//
// A file edit is any call that may change files (mayEdit) and was not denied
// before running. A failed or unrecorded one counts: whether it changed a
// file is exactly what the record cannot say, and assuming it did not would
// complete a pattern on a guess. It is a TEST edit only when it is of verb
// class write, carries the test-file label and ran ok; every other edit -- a
// shell one, which has no label, MultiEdit, which the label layer does not
// read, and every call of another class -- is an edit to a file not named
// like a test, and stops pattern A. Stopping is the under-claim.
//
// A test run is an edit too, for every pair but its own command's: `jest -u`
// rewrites snapshots and has its own digest, so between a failed `jest` and a
// passing one it is the change the pass may owe itself to. An interrupted or
// unrecorded run of the same command counts the same way; only the two runs
// a pair is made of are not between it.
//
// A directory change breaks every pair open across it, and needs no rule of
// its own for that: a lone cd, pushd or popd is a shell call of class execute,
// so it is an edit here like any other. It has to break them. Claude Code's
// shell keeps its working directory from one call to the next, and the digest
// does not cover it, so `go test ./...` before and after a `cd ../other` are
// equal digests run over different code. A `cd DIR && ...` that leaves the
// shell in DIR breaks them only when its class is not read, network or agent
// -- `cd DIR && ls` is read -- and the report states that limit.
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
		// A denied call never ran: it changed no file and no directory, and
		// it is no run with a result.
		if outcome == LinkOutcomeDenied {
			continue
		}
		run := d.Shape.VerbClass == shape.VerbTest && (outcome == store.ExecOK || outcome == store.ExecFailed)
		failed := outcome == store.ExecFailed
		// The pair is decided on the totals BEFORE this call counts as an
		// edit: a run is not between itself and the run it pairs with.
		if prev, ok := last[d.Shape.Digest]; ok && run {
			pair := SeqPair{prev.seq, d.Seq}
			switch {
			case prev.failed && !failed && otherEdits == prev.otherEdits && testEdits > prev.testEdits:
				out.TestsOnlyThenGreen = append(out.TestsOnlyThenGreen, pair)
			case prev.failed != failed && anyEdits == prev.anyEdits:
				out.Flaky = append(out.Flaky, pair)
			}
		}
		if mayEdit(d) {
			anyEdits++
			switch {
			case d.Shape.VerbClass != shape.VerbWrite || d.FileLabel == nil || *d.FileLabel != shape.LabelTestFile:
				otherEdits++
			case outcome == store.ExecOK:
				testEdits++
			}
		}
		// And the mark AFTER it counts, so the next run of this command
		// does not see this run as between.
		if run {
			last[d.Shape.Digest] = mark{
				seq: d.Seq, failed: failed, testEdits: testEdits, otherEdits: otherEdits, anyEdits: anyEdits,
			}
		}
	}
	return out
}

// mayEdit reports a call that may change files: every verb class but read,
// network and agent. Not only write: `git checkout -- f` is vcs, `npm
// install` and `go generate` are package, `sed -i` is execute, an MCP tool is
// mcp, a tool this build does not know is unknown, and `jest -u` is test. A
// pair that completed across any of them would say "no recorded file edit
// between" over a change the record holds a call for.
//
// The three left out are the classes whose calls are not there to write: a
// Read, Grep or Glob, a shell cat or ls; a WebFetch or a shell curl; an
// Agent call, whose subagent's own calls are recorded and counted each on its
// own. Not that none can: `cat a > b` and `curl -o f` do write, and the report
// states that limit beside every pattern. Counting them as well would stop a
// pair at every `ls` an agent runs between two test runs, which is most of
// them.
func mayEdit(d store.Declaration) bool {
	switch d.Shape.VerbClass {
	case shape.VerbRead, shape.VerbNetwork, shape.VerbAgent:
		return false
	}
	return true
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
