package report

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The two test-bending patterns, over calls built in code. Each test names the
// rule it holds and the break it catches. Letters: T is a test run, E an edit,
// and the digest is what makes two runs "the same command".

type tbCall struct {
	seq     int64
	tool    string // Bash, Edit, Write, NotebookEdit, MultiEdit
	verb    string // for Bash: test, execute, write
	digest  string
	label   string // for an edit: its file_label
	outcome string // ok, failed, interrupted, "" for no execution record
	agent   string
	prog    string // for Bash: the program, go when empty
}

func tbRun(calls ...tbCall) *store.Run {
	run := &store.Run{}
	for _, c := range calls {
		id := "t" + string(rune('a'+c.seq))
		verb := c.verb
		if verb == "" {
			verb = shape.VerbWrite
		}
		d := store.Declaration{
			Seq: c.seq, RecordedAtMS: 1_700_000_000_000 + c.seq*1000, ToolUseID: id, ToolName: c.tool, SessionID: "s1",
			Shape: shape.Shape{VerbClass: verb, Digest: c.digest},
		}
		if c.tool == "Bash" {
			p := "go"
			if c.prog != "" {
				p = c.prog
			}
			d.Shape.Program = &p
		}
		if c.label != "" {
			l := c.label
			d.FileLabel = &l
		}
		if c.agent != "" {
			a, ty := c.agent, "general-purpose"
			d.AgentID, d.AgentType = &a, &ty
		}
		run.Declarations = append(run.Declarations, d)
		if c.outcome != "" {
			run.Executions = append(run.Executions, store.Execution{ToolUseID: id, ToolName: c.tool, Outcome: c.outcome})
		}
	}
	return run
}

func test(seq int64, digest, outcome string) tbCall {
	return tbCall{seq: seq, tool: "Bash", verb: shape.VerbTest, digest: digest, outcome: outcome}
}

func edit(seq int64, label, outcome string) tbCall {
	return tbCall{seq: seq, tool: "Edit", label: label, digest: "e" + string(rune('0'+seq)), outcome: outcome}
}

func pairs(p ...SeqPair) []SeqPair {
	if p == nil {
		return []SeqPair{}
	}
	return p
}

func TestTestBending(t *testing.T) {
	const ok, failed = store.ExecOK, store.ExecFailed
	tf, none := shape.LabelTestFile, shape.LabelNone
	for _, tc := range []struct {
		name       string
		run        *store.Run
		green      []SeqPair
		flaky      []SeqPair
		why        string
		denied     map[string]bool
		wantDenied bool
	}{
		{name: "A: failed, only test files edited, passed",
			run:   tbRun(test(1, "d", failed), edit(2, tf, ok), edit(3, tf, ok), test(4, "d", ok)),
			green: pairs(SeqPair{1, 4}), flaky: pairs()},
		{name: "A: code and test edited is not raised",
			run:   tbRun(test(1, "d", failed), edit(2, tf, ok), edit(3, none, ok), test(4, "d", ok)),
			green: pairs(), flaky: pairs(),
			why: "fixing code and adjusting a test is the normal pattern; flagging it trains people to ignore the line"},
		{name: "A: only code edited is not raised",
			run:   tbRun(test(1, "d", failed), edit(2, none, ok), test(3, "d", ok)),
			green: pairs(), flaky: pairs()},
		{name: "A: a different digest is not the same command",
			run:   tbRun(test(1, "d", failed), edit(2, tf, ok), test(3, "other", ok)),
			green: pairs(), flaky: pairs()},
		{name: "A: a test edit that did not run ok does not complete it",
			run:   tbRun(test(1, "d", failed), edit(2, tf, failed), test(3, "d", ok)),
			green: pairs(), flaky: pairs(),
			why: "a failed edit changed nothing the record can vouch for; and it is still an edit, so not B either"},
		{name: "A: an edit with no label is not a test edit",
			run:   tbRun(test(1, "d", failed), edit(2, tf, ok), tbCall{seq: 3, tool: "MultiEdit", digest: "m", outcome: ok}, test(4, "d", ok)),
			green: pairs(), flaky: pairs()},
		{name: "A: a shell write between is a non-test edit",
			run:   tbRun(test(1, "d", failed), edit(2, tf, ok), tbCall{seq: 3, tool: "Bash", verb: shape.VerbWrite, digest: "cp", outcome: ok}, test(4, "d", ok)),
			green: pairs(), flaky: pairs()},
		{name: "A: an unrecorded non-test edit stops it",
			run:   tbRun(test(1, "d", failed), edit(2, tf, ok), edit(3, none, ""), test(4, "d", ok)),
			green: pairs(), flaky: pairs(),
			why: "no execution record may have run; completing the pattern on it would be a guess"},
		{name: "A: a denied non-test edit does not stop it",
			run:    tbRun(test(1, "d", failed), edit(2, tf, ok), edit(3, none, ""), test(4, "d", ok)),
			denied: map[string]bool{"td": true},
			green:  pairs(SeqPair{1, 4}), flaky: pairs()},
		{name: "A: read, network and agent calls between do not stop it",
			run: tbRun(test(1, "d", failed), tbCall{seq: 2, tool: "Bash", verb: shape.VerbRead, prog: "ls", digest: "ls", outcome: ok},
				edit(3, tf, ok), tbCall{seq: 4, tool: "Read", verb: shape.VerbRead, digest: "r", outcome: ok},
				tbCall{seq: 5, tool: "WebFetch", verb: shape.VerbNetwork, digest: "w", outcome: ok},
				tbCall{seq: 6, tool: "Agent", verb: shape.VerbAgent, digest: "a", outcome: ok}, test(7, "d", ok)),
			green: pairs(SeqPair{1, 7}), flaky: pairs()},
		{name: "A: a task or todo update between does not stop it",
			run: tbRun(test(1, "d", failed), edit(2, tf, ok),
				tbCall{seq: 3, tool: "TodoWrite", verb: shape.VerbUnknown, digest: "td", outcome: ok},
				tbCall{seq: 4, tool: "TaskUpdate", verb: shape.VerbUnknown, digest: "tu", outcome: ok}, test(5, "d", ok)),
			green: pairs(SeqPair{1, 5}), flaky: pairs(),
			why: "a TodoWrite or TaskUpdate writes no file; counting one hides the episode in every session that has them"},
		{name: "A: an interrupted shell edit between stops it",
			run: tbRun(test(1, "d", failed), edit(2, tf, ok),
				tbCall{seq: 3, tool: "Bash", verb: shape.VerbExecute, prog: "sed", digest: "sed", outcome: store.ExecInterrupted}, test(4, "d", ok)),
			green: pairs(), flaky: pairs(),
			why: "an interrupted sed -i may have rewritten the source before it stopped"},
		{name: "A: a shell command of another class between stops it",
			run:   tbRun(test(1, "d", failed), edit(2, tf, ok), tbCall{seq: 3, tool: "Bash", verb: shape.VerbVCS, prog: "git", digest: "co", outcome: ok}, test(4, "d", ok)),
			green: pairs(), flaky: pairs(),
			why: "a git checkout restores files: the pass may be the checkout's, not the test edit's"},
		{name: "A: pairs with the previous run of the command, not the first failure",
			run:   tbRun(test(1, "d", failed), edit(2, none, ok), test(3, "d", failed), edit(4, tf, ok), test(5, "d", ok)),
			green: pairs(SeqPair{3, 5}), flaky: pairs()},
		{name: "A: a subagent's run and edits count",
			run: tbRun(test(1, "d", failed),
				tbCall{seq: 2, tool: "Edit", label: tf, digest: "e", outcome: ok, agent: "agent-a1"},
				tbCall{seq: 3, tool: "Bash", verb: shape.VerbTest, digest: "d", outcome: ok, agent: "agent-a1"}),
			green: pairs(SeqPair{1, 3}), flaky: pairs(),
			why: "seq is one order over every agent; a subagent finishing the main agent's work is the common case"},
		{name: "A: passed, test edited, passed is not raised",
			run:   tbRun(test(1, "d", ok), edit(2, tf, ok), test(3, "d", ok)),
			green: pairs(), flaky: pairs()},
		{name: "A: an edit only after the pass is not between",
			run:   tbRun(test(1, "d", failed), test(2, "d", ok), edit(3, tf, ok)),
			green: pairs(), flaky: pairs(SeqPair{1, 2})},

		{name: "B: failed then ok, nothing edited",
			run:   tbRun(test(1, "d", failed), test(2, "d", ok)),
			green: pairs(), flaky: pairs(SeqPair{1, 2})},
		{name: "B: ok then failed, nothing edited",
			run:   tbRun(test(1, "d", ok), tbCall{seq: 2, tool: "Grep", verb: shape.VerbRead, digest: "x", outcome: ok}, test(3, "d", failed)),
			green: pairs(), flaky: pairs(SeqPair{1, 3})},
		{name: "B: another test command between is not raised",
			run:   tbRun(test(1, "d", failed), test(2, "u", ok), test(3, "d", ok)),
			green: pairs(), flaky: pairs(),
			why: "jest -u is a test run with its own digest, and it rewrites snapshots"},
		{name: "B: a call of any class that may write is not raised",
			run: tbRun(test(1, "d", ok),
				tbCall{seq: 2, tool: "Bash", verb: shape.VerbExecute, prog: "sed", digest: "x", outcome: ok}, test(3, "d", failed),
				tbCall{seq: 4, tool: "Bash", verb: shape.VerbPackage, prog: "npm", digest: "i", outcome: ok}, test(5, "d", ok),
				tbCall{seq: 6, tool: "mcp__fs__write", verb: shape.VerbMCP, digest: "m", outcome: ok}, test(7, "d", failed),
				tbCall{seq: 8, tool: "Custom", verb: shape.VerbUnknown, digest: "c", outcome: failed}, test(9, "d", ok)),
			green: pairs(), flaky: pairs(),
			why: "sed -i, npm install, an MCP write and an unknown tool can each change files; a failed one may have too"},
		{name: "B: bookkeeping tools between do not stop it",
			run: tbRun(test(1, "d", failed),
				tbCall{seq: 2, tool: "TaskUpdate", verb: shape.VerbUnknown, digest: "tu", outcome: ok},
				tbCall{seq: 3, tool: "TodoWrite", verb: shape.VerbUnknown, digest: "td", outcome: ok},
				tbCall{seq: 4, tool: "TaskCreate", verb: shape.VerbUnknown, digest: "tc", outcome: ok},
				tbCall{seq: 5, tool: "BashOutput", verb: shape.VerbExecute, digest: "bo", outcome: ok},
				tbCall{seq: 6, tool: "AskUserQuestion", verb: shape.VerbUnknown, digest: "aq", outcome: ok},
				tbCall{seq: 7, tool: "ExitPlanMode", verb: shape.VerbUnknown, digest: "ep", outcome: ok},
				tbCall{seq: 8, tool: "TaskList", verb: shape.VerbUnknown, digest: "tl", outcome: ok},
				tbCall{seq: 9, tool: "TaskGet", verb: shape.VerbUnknown, digest: "tg", outcome: ok},
				tbCall{seq: 10, tool: "TaskOutput", verb: shape.VerbUnknown, digest: "to", outcome: ok}, test(11, "d", ok)),
			green: pairs(), flaky: pairs(SeqPair{1, 11})},
		{name: "B: an interrupted run of another test command between is not raised",
			run:   tbRun(test(1, "d", ok), test(2, "u", store.ExecInterrupted), test(3, "d", failed)),
			green: pairs(), flaky: pairs(),
			why: "an interrupted jest -u may have rewritten snapshots before it stopped"},
		{name: "B: an edit between is not raised",
			run:   tbRun(test(1, "d", ok), edit(2, none, ok), test(3, "d", failed)),
			green: pairs(), flaky: pairs()},
		{name: "B: a Write between is not raised",
			run:   tbRun(test(1, "d", ok), tbCall{seq: 2, tool: "Write", label: none, digest: "w", outcome: ok}, test(3, "d", failed)),
			green: pairs(), flaky: pairs()},
		{name: "B: a NotebookEdit between is not raised",
			run:   tbRun(test(1, "d", ok), tbCall{seq: 2, tool: "NotebookEdit", label: none, digest: "n", outcome: ok}, test(3, "d", failed)),
			green: pairs(), flaky: pairs()},
		{name: "B: a failed edit between is not raised",
			run:   tbRun(test(1, "d", ok), edit(2, none, failed), test(3, "d", failed)),
			green: pairs(), flaky: pairs()},
		{name: "B: different digests are not raised",
			run:   tbRun(test(1, "d", ok), test(2, "other", failed)),
			green: pairs(), flaky: pairs()},
		{name: "B: the same outcome twice is not raised",
			run:   tbRun(test(1, "d", failed), test(2, "d", failed), test(3, "d", ok), test(4, "d", ok)),
			green: pairs(), flaky: pairs(SeqPair{2, 3})},
		{name: "B: an interrupted run is not a result",
			run:   tbRun(test(1, "d", ok), test(2, "d", store.ExecInterrupted), test(3, "d", ok)),
			green: pairs(), flaky: pairs()},
		{name: "B: a run with no execution record is not a result, and may have written",
			run:   tbRun(test(1, "d", ok), test(2, "d", ""), test(3, "d", failed)),
			green: pairs(), flaky: pairs(),
			why: "it cannot finish a pair, and a run that may have gone ahead may have written a snapshot"},
		{name: "B: a denied run between is nothing",
			run:    tbRun(test(1, "d", ok), test(2, "u", ""), test(3, "d", failed)),
			denied: map[string]bool{"tc": true},
			green:  pairs(), flaky: pairs(SeqPair{1, 3})},
		{name: "B: a subagent's run counts",
			run: tbRun(test(1, "d", failed),
				tbCall{seq: 2, tool: "Bash", verb: shape.VerbTest, digest: "d", outcome: ok, agent: "agent-b2"}),
			green: pairs(), flaky: pairs(SeqPair{1, 2})},
		{name: "a cd between breaks the pair",
			run:   tbRun(test(1, "d", failed), tbCall{seq: 2, tool: "Bash", verb: shape.VerbExecute, prog: "cd", digest: "cd", outcome: ok}, test(3, "d", ok)),
			green: pairs(), flaky: pairs(),
			why: "the shell keeps its directory between calls, and the digest does not cover it: the runs may be over different code"},
		{name: "a cd between breaks pattern A too",
			run: tbRun(test(1, "d", failed), edit(2, tf, ok),
				tbCall{seq: 3, tool: "Bash", verb: shape.VerbExecute, prog: "pushd", digest: "p", outcome: ok}, test(4, "d", ok)),
			green: pairs(), flaky: pairs()},
		{name: "a cd before both runs does not break them",
			run:   tbRun(tbCall{seq: 1, tool: "Bash", verb: shape.VerbExecute, prog: "cd", digest: "cd", outcome: ok}, test(2, "d", failed), test(3, "d", ok)),
			green: pairs(), flaky: pairs(SeqPair{2, 3})},
		{name: "a denied cd does not break the pair",
			run:    tbRun(test(1, "d", failed), tbCall{seq: 2, tool: "Bash", verb: shape.VerbExecute, prog: "cd", digest: "cd"}, test(3, "d", ok)),
			denied: map[string]bool{"tc": true},
			green:  pairs(), flaky: pairs(SeqPair{1, 3})},
		{name: "a command that is not a test run is never paired",
			run: tbRun(tbCall{seq: 1, tool: "Bash", verb: shape.VerbPackage, digest: "d", outcome: failed},
				tbCall{seq: 2, tool: "Bash", verb: shape.VerbPackage, digest: "d", outcome: ok}),
			green: pairs(), flaky: pairs(),
			why: "records written before the test class say package for go test: under-claim"},
		{name: "declarations out of seq order are read in seq order",
			run:   reversed(tbRun(test(1, "d", failed), edit(2, tf, ok), test(3, "d", ok))),
			green: pairs(SeqPair{1, 3}), flaky: pairs()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectTestBending(tc.run, tc.denied)
			if !reflect.DeepEqual(got.TestsOnlyThenGreen, tc.green) {
				t.Errorf("tests_only_then_green = %v, want %v%s", got.TestsOnlyThenGreen, tc.green, whyNote(tc.why))
			}
			if !reflect.DeepEqual(seqsOf(got.Flaky), tc.flaky) {
				t.Errorf("flaky = %v, want %v%s", got.Flaky, tc.flaky, whyNote(tc.why))
			}
		})
	}
}

// TestTestBending_ShapesFromDerive: the calls a review found bent the
// patterns, built through shape.Derive as the hook builds them rather than
// with the class set by hand. A background launch records ok when the shell
// starts and digests equal to the foreground run; a pipeline records its last
// program's status (grep inverts it); a cd moves the next run to other code.
// None of the three may complete either pattern.
func TestTestBending_ShapesFromDerive(t *testing.T) {
	key := []byte("key")
	bash := func(seq int64, input, outcome string) (store.Declaration, *store.Execution) {
		id := "d" + string(rune('a'+seq))
		d := store.Declaration{Seq: seq, ToolUseID: id, ToolName: "Bash", SessionID: "s1",
			Shape: shape.Derive("Bash", json.RawMessage(input), key)}
		return d, &store.Execution{ToolUseID: id, ToolName: "Bash", Outcome: outcome}
	}
	testEdit := func(seq int64) (store.Declaration, *store.Execution) {
		id := "d" + string(rune('a'+seq))
		l := shape.LabelTestFile
		d := store.Declaration{Seq: seq, ToolUseID: id, ToolName: "Edit", SessionID: "s1", FileLabel: &l,
			Shape: shape.Derive("Edit", json.RawMessage(`{"file_path":"/r/foo_test.go"}`), key)}
		return d, &store.Execution{ToolUseID: id, ToolName: "Edit", Outcome: store.ExecOK}
	}
	type call func() (store.Declaration, *store.Execution)
	b := func(seq int64, input, outcome string) call {
		return func() (store.Declaration, *store.Execution) { return bash(seq, input, outcome) }
	}
	e := func(seq int64) call { return func() (store.Declaration, *store.Execution) { return testEdit(seq) } }
	const ok, failed = store.ExecOK, store.ExecFailed
	for _, tc := range []struct {
		name  string
		calls []call
	}{
		{"background launch after a failure", []call{
			b(1, `{"command":"make test"}`, failed), b(2, `{"command":"make test","run_in_background":true}`, ok)}},
		{"background launch after a test edit", []call{
			b(1, `{"command":"go test ./..."}`, failed), e(2), b(3, `{"command":"go test ./...","run_in_background":true}`, ok)}},
		{"a grep pipeline inverts the outcome", []call{
			b(1, `{"command":"go test ./... 2>&1 | grep FAIL"}`, failed), e(2), b(3, `{"command":"go test ./... 2>&1 | grep FAIL"}`, ok)}},
		{"a tail pipeline records tail", []call{
			b(1, `{"command":"go test ./... 2>&1 | tail -20"}`, failed), b(2, `{"command":"go test ./... 2>&1 | tail -20"}`, ok)}},
		{"a cd between moves the run", []call{
			b(1, `{"command":"go test ./..."}`, failed), b(2, `{"command":"cd ../other-module"}`, ok), b(3, `{"command":"go test ./..."}`, ok)}},
	} {
		run := &store.Run{}
		for _, c := range tc.calls {
			d, x := c()
			run.Declarations = append(run.Declarations, d)
			run.Executions = append(run.Executions, *x)
		}
		got := DetectTestBending(run, nil)
		if len(got.TestsOnlyThenGreen)+len(got.Flaky) != 0 {
			t.Errorf("%s: %+v, want neither pattern", tc.name, got)
		}
	}
}

func seqsOf(f []FlakyPair) []SeqPair {
	out := []SeqPair{}
	for _, p := range f {
		out = append(out, p.Seqs)
	}
	return out
}

// TestTestBending_FlakyKeepsTheOrder: a flaky pair says which of its runs
// failed, so every reader can print the order they ran in. Break: record the
// later run's outcome, or none, and a fail-then-pass pair -- the one that
// reads as a fix -- prints as passed then failed.
func TestTestBending_FlakyKeepsTheOrder(t *testing.T) {
	const ok, failed = store.ExecOK, store.ExecFailed
	got := DetectTestBending(tbRun(test(1, "d", failed), test(2, "d", ok), test(3, "d", failed)), nil).Flaky
	want := []FlakyPair{{Seqs: SeqPair{1, 2}, FirstFailed: true}, {Seqs: SeqPair{2, 3}, FirstFailed: false}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("flaky = %+v, want %+v", got, want)
	}
	if a, b := want[0].Outcomes(); a != "failed" || b != "passed" {
		t.Errorf("first failed: outcomes %s, %s", a, b)
	}
	if a, b := want[1].Outcomes(); a != "passed" || b != "failed" {
		t.Errorf("first passed: outcomes %s, %s", a, b)
	}
	b, err := json.Marshal(want[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"seqs":[1,2],"first_failed":true}` {
		t.Errorf("marshalled = %s", b)
	}
}

func reversed(run *store.Run) *store.Run {
	d := run.Declarations
	for i, j := 0, len(d)-1; i < j; i, j = i+1, j-1 {
		d[i], d[j] = d[j], d[i]
	}
	return run
}

func whyNote(why string) string {
	if why == "" {
		return ""
	}
	return "\n  " + why
}

// TestTestBending_NilRunAndEmptyListsMarshal: both lists are present and
// empty, never null -- "looked and found none" is not "not looked".
func TestTestBending_NilRunAndEmptyListsMarshal(t *testing.T) {
	b, err := json.Marshal(DetectTestBending(nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(b); got != `{"tests_only_then_green":[],"flaky":[]}` {
		t.Errorf("marshalled = %s", got)
	}
}

// TestTestRuns_CountsAndText: the session block counts runs with a result,
// lists every pair, and prints the limit beside them. Absent when the session
// ran no test, because a zero there would be a count old records never
// measured.
func TestTestRuns_CountsAndText(t *testing.T) {
	const ok, failed = store.ExecOK, store.ExecFailed
	run := tbRun(test(1, "d", failed), edit(2, shape.LabelTestFile, ok), test(3, "d", ok),
		test(4, "f", ok), test(5, "f", failed), test(6, "g", store.ExecInterrupted),
		test(7, "h", failed), test(8, "h", ok))
	tr := buildTestRuns(run, nil)
	if tr.Runs != 6 || tr.OK != 3 || tr.Failed != 3 {
		t.Errorf("runs/ok/failed = %d/%d/%d, want 6/3/3: an interrupted run has no result", tr.Runs, tr.OK, tr.Failed)
	}

	var b bytes.Buffer
	writeTestRuns(&b, tr)
	out := b.String()
	for _, want := range []string{
		"test runs: 6 (3 ok, 3 failed)",
		"failed, then the only recorded edits were to files named like tests, then the same command passed: 1 → 3",
		// Each pair in the order it ran: "passed and failed" for a pair
		// that failed first was the order the record contradicts.
		"same command had both outcomes with no recorded file edit between: 4 passed, 5 failed",
		"same command had both outcomes with no recorded file edit between: 7 failed, 8 passed",
		"a file edit here is any recorded call but a read, a web fetch, a subagent launch or a task, todo, question or plan tool",
		"a shell read or fetch (cat, curl and the like) can still write",
		"nor a directory change made inside another shell command",
		"a runner behind `cd DIR &&` is a test run, so a cd that failed reads as a failed run",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "nothing changed") {
		t.Errorf("text claims nothing changed:\n%s", out)
	}

	b.Reset()
	writeTestRuns(&b, buildTestRuns(tbRun(tbCall{seq: 1, tool: "Bash", verb: shape.VerbExecute, digest: "x", outcome: ok}), nil))
	if b.Len() != 0 {
		t.Errorf("a session with no test run rendered a block: %q", b.String())
	}

	b.Reset()
	writeTestRuns(&b, buildTestRuns(tbRun(test(1, "d", ok), test(2, "d", ok)), nil))
	if strings.Contains(b.String(), "a file edit here") {
		t.Errorf("the limit printed with no pattern to qualify:\n%s", b.String())
	}
}

// TestTimeline_AnnotatesTheRowThatCompletesAPattern: the later run of each
// pair carries the pattern and the earlier seq, in JSON and as a line under
// its row. Break: annotate the earlier row, or every test row.
func TestTimeline_AnnotatesTheRowThatCompletesAPattern(t *testing.T) {
	const ok, failed = store.ExecOK, store.ExecFailed
	run := tbRun(test(1, "d", failed), edit(2, shape.LabelTestFile, ok), test(3, "d", ok),
		test(4, "f", ok), test(5, "f", failed))
	tl := buildTimeline(run, nil)
	want := map[int64]*TimelineBending{
		3: {Kind: BendTestsOnlyThenGreen, Since: 1},
		5: {Kind: BendFlaky, Since: 4},
	}
	for _, c := range tl.Calls {
		if w := want[*c.Seq]; !reflect.DeepEqual(c.Bending, w) {
			t.Errorf("seq %d: bending = %+v, want %+v", *c.Seq, c.Bending, w)
		}
	}

	var b bytes.Buffer
	writeTimeline(&b, tl)
	out := b.String()
	for _, w := range []string{
		"↳ the only recorded edits since 1 were to files named like tests, where the same command failed",
		"↳ same command had the other outcome at 4, no recorded file edit between",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("timeline text is missing %q:\n%s", w, out)
		}
	}
	if strings.Count(out, "↳") != 2 {
		t.Errorf("want exactly two annotations:\n%s", out)
	}
}
