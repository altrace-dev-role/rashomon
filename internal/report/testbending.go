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

// FlakyPair is a flaky pair and which of its two runs failed. The seqs alone
// do not say, and a reader told "passed and failed" of a pair that failed
// first is told the order backwards: the fail-then-pass pair is the one that
// reads as a fix, and it matters which it was. A closed bit, like the seqs.
//
// First and second are in declaration order, the order the two runs STARTED
// (seq), not the order they finished. Two identical runs overlapping in
// parallel agents can finish the other way round, so "#10 failed, #11
// passed" says which started first, and the timeline shows where each result
// was recorded.
type FlakyPair struct {
	Seqs        SeqPair `json:"seqs"`
	FirstFailed bool    `json:"first_failed"`
}

// Outcomes names the pair's two runs' outcomes, in seq (declaration) order.
func (p FlakyPair) Outcomes() (first, second string) {
	if p.FirstFailed {
		return "failed", "passed"
	}
	return "passed", "failed"
}

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
	// one failed in either order, with no file edit recorded between them,
	// and says which order it was.
	Flaky []FlakyPair `json:"flaky"`
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
// failed (testOutcome). Interrupted, denied, backgrounded, timed-out and
// unknown runs are not runs with a result, so they neither start nor finish a
// pair. Two runs are the same
// command when their shape digests are equal and they were declared in the
// same directory (equal cwd digests); the digest covers the tool name, so
// equal digests are the same tool as well.
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
// Runs pair only within one directory. Claude Code's shell keeps its working
// directory from one call to the next, and the command digest does not cover
// it, so `go test ./...` before and after a `cd ../other` are equal digests
// run over different code; so are a main agent's run after its `cd pkg` and
// a subagent's, which starts in the directory the main agent had at launch;
// and so are two calls of `cd sub && go test ./...`, the second of which
// fails at its cd, since the first left the shell in sub. The payload's cwd
// follows the shell, so its digest tells each of these apart, and a subagent
// re-running the main agent's tests where they ran still pairs. A lone cd
// between two runs is an edit here as well (class execute), which changes
// nothing: the runs either side of it are in two directories anyway.
func DetectTestBending(run *store.Run, denied map[string]bool) TestBending {
	if run == nil {
		return detectTestBending(nil, nil, denied)
	}
	return detectTestBending(run, executionsByID(run), denied)
}

// detectTestBending is DetectTestBending over run's executions already
// grouped (executionsByID), so that Build groups them once for the timeline,
// the test runs and this.
func detectTestBending(run *store.Run, executed map[string][]store.Execution, denied map[string]bool) TestBending {
	out := TestBending{TestsOnlyThenGreen: []SeqPair{}, Flaky: []FlakyPair{}}
	if run == nil {
		return out
	}

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
	// The same command in the same directory: the shape digest and the cwd
	// digest, compared together.
	type runKey struct{ command, cwd string }
	last := map[runKey]mark{}

	for _, d := range sorted {
		outcome := testOutcome(d, executed, denied)
		// A denied call never ran: it changed no file and no directory, and
		// it is no run with a result.
		if outcome == LinkOutcomeDenied {
			continue
		}
		isRun := d.Shape.VerbClass == shape.VerbTest && (outcome == store.ExecOK || outcome == store.ExecFailed)
		failed := outcome == store.ExecFailed
		// The pair is decided on the totals BEFORE this call counts as an
		// edit: a run is not between itself and the run it pairs with.
		key := runKey{d.Shape.Digest, d.CWDDigest}
		if prev, ok := last[key]; ok && isRun {
			pair := SeqPair{prev.seq, d.Seq}
			switch {
			case prev.failed && !failed && otherEdits == prev.otherEdits && testEdits > prev.testEdits:
				out.TestsOnlyThenGreen = append(out.TestsOnlyThenGreen, pair)
			case prev.failed != failed && anyEdits == prev.anyEdits:
				out.Flaky = append(out.Flaky, FlakyPair{Seqs: pair, FirstFailed: prev.failed})
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
		if isRun {
			last[key] = mark{
				seq: d.Seq, failed: failed, testEdits: testEdits, otherEdits: otherEdits, anyEdits: anyEdits,
			}
		}
	}
	return out
}

// mayEdit reports a call that may change files: every verb class but read,
// network and agent, and a shell call of any class whose shape says it may
// write. Not only write: `git checkout -- f` is vcs, `npm install` and `go
// generate` are package, `sed -i` is execute, an MCP tool is mcp, a tool this
// build does not know is unknown, and `jest -u` is test. A pair that
// completed across any of them would say "no recorded file edit between"
// over a change the record holds a call for.
//
// The three left out are the classes whose calls are not there to write: a
// Read, Grep or Glob, a shell cat or ls; a WebFetch or a shell curl; an
// Agent call, whose subagent's own calls are recorded and counted each on its
// own. Counting them all would stop a pair at every `ls` an agent runs
// between two test runs, which is most of them. The shell ones that do write
// -- `cat a > b`, `curl -o f`, `find -delete`, `find -exec sed -i`, `grep |
// xargs sed -i`, rsync or scp into the tree -- carry shape.may_write
// (shape.mayWrite says what sets it), and count. What that misses, the report
// states beside every pattern: a read or fetch that writes through an option
// the list does not name, such as `find -fprint f` or `curl -D f`.
//
// Nor do Claude Code's own bookkeeping tools (noWrite), whatever class they
// are stored under: a TodoWrite, a TaskUpdate or a ToolSearch between two
// runs is routine, a KillShell or a TaskStop naturally follows a run moved to
// the background, and counting any of them would hide the episode.
// The list is matched on the tool name and changes no stored class.
func mayEdit(d store.Declaration) bool {
	if noWrite[d.ToolName] {
		return false
	}
	if d.Shape.MayWrite {
		return true
	}
	switch d.Shape.VerbClass {
	case shape.VerbRead, shape.VerbNetwork, shape.VerbAgent:
		return false
	}
	return true
}

// noWrite is Claude Code's tools that track or stop tasks, ask the user,
// enter or leave plan mode, read or stop a background shell, load a skill,
// search for tools, message another agent, schedule or list cron jobs, or
// list and read MCP resources: none writes a source or test file (a durable
// CronCreate or CronDelete writes only .claude/scheduled_tasks.json). A fixed
// list of names, so a tool this build does not know still counts as an edit.
var noWrite = map[string]bool{
	"TodoWrite": true, "TaskCreate": true, "TaskUpdate": true, "TaskList": true, "TaskGet": true,
	"TaskOutput": true, "TaskStop": true, "AskUserQuestion": true, "EnterPlanMode": true,
	"ExitPlanMode": true, "BashOutput": true, "KillShell": true, "KillBash": true, "Skill": true,
	"ToolSearch": true, "SendMessage": true, "CronCreate": true, "CronDelete": true, "CronList": true,
	"ListMcpResourcesTool": true, "ReadMcpResourceTool": true,
}

// TestRuns is the session's test runs and the two patterns among them.
type TestRuns struct {
	// Runs counts calls of verb class test that ended ok or failed. A
	// session with no schema 3 declaration has no TestRuns at all
	// (buildTestRuns), so a 0 here was measured.
	Runs   int `json:"runs"`
	OK     int `json:"ok"`
	Failed int `json:"failed"`
	TestBending
}

// buildTestRuns counts run's test runs by the outcomes in executed (run's
// executionsByID) and denied, and carries tb, the patterns detectTestBending
// found over the same three: Build computes each once and shares it with the
// timeline.
//
// Nil when no declaration of run is schema 3 or later: the test class is a
// schema 3 vocabulary, and records written before it say execute or package
// for the same commands, so counting them would state zeros nobody measured.
// One schema 3 declaration is enough, since from it on the class was being
// written; a session that spans an upgrade is counted from the upgrade.
func buildTestRuns(run *store.Run, executed map[string][]store.Execution, denied map[string]bool, tb TestBending) *TestRuns {
	if run == nil || !measuresTests(run) {
		return nil
	}
	out := &TestRuns{TestBending: tb}
	for _, d := range run.Declarations {
		if d.Shape.VerbClass != shape.VerbTest {
			continue
		}
		switch testOutcome(d, executed, denied) {
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

// outcomeTimedOut is testOutcome's word for a run whose `timeout N` prefix
// fired. Never rendered: it is not ok and not failed, and that is all a
// reader of it asks.
const outcomeTimedOut = "timed out"

// testOutcome is linkOutcome for d, with one more case that is no result: a
// call whose program is timeout, and which failed with exit code 124. That is
// timeout's own status when the duration ran out, not the runner's -- the
// tests were stopped, not failed -- and a runner that exits 124 itself cannot
// be told apart from it behind timeout, so neither is read as a result. The
// same code from a run with no timeout prefix is the runner's, and a failure.
func testOutcome(d store.Declaration, executed map[string][]store.Execution, denied map[string]bool) string {
	o, _, _ := linkOutcome(d.ToolUseID, executed, denied)
	if o != store.ExecFailed || d.Shape.Program == nil || *d.Shape.Program != "timeout" {
		return o
	}
	// The exit code of the record linkOutcome read the outcome from: the
	// highest seq, since executionsByID sorts ascending.
	recs := executed[d.ToolUseID]
	if c := recs[len(recs)-1].ExitCode; c != nil && *c == timeoutFired {
		return outcomeTimedOut
	}
	return o
}

// timeoutFired is the exit status timeout(1) gives when the duration ran out
// and the command was stopped.
const timeoutFired = 124

// measuresTests reports a run holding a declaration written at schema 3 or
// later, where the test class exists.
func measuresTests(run *store.Run) bool {
	for _, d := range run.Declarations {
		if d.SchemaVersion >= 3 {
			return true
		}
	}
	return false
}
