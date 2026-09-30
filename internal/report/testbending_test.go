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
		{name: "A: other commands between do not stop it",
			run: tbRun(test(1, "d", failed), tbCall{seq: 2, tool: "Bash", verb: shape.VerbExecute, digest: "ls", outcome: ok},
				edit(3, tf, ok), tbCall{seq: 4, tool: "Read", verb: shape.VerbRead, digest: "r", outcome: ok}, test(5, "d", ok)),
			green: pairs(SeqPair{1, 5}), flaky: pairs()},
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
			run:   tbRun(test(1, "d", ok), tbCall{seq: 2, tool: "Bash", verb: shape.VerbExecute, digest: "x", outcome: ok}, test(3, "d", failed)),
			green: pairs(), flaky: pairs(SeqPair{1, 3})},
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
		{name: "B: a run with no execution record is not a result",
			run:   tbRun(test(1, "d", ok), test(2, "d", ""), test(3, "d", failed)),
			green: pairs(), flaky: pairs(SeqPair{1, 3})},
		{name: "B: a subagent's run counts",
			run: tbRun(test(1, "d", failed),
				tbCall{seq: 2, tool: "Bash", verb: shape.VerbTest, digest: "d", outcome: ok, agent: "agent-b2"}),
			green: pairs(), flaky: pairs(SeqPair{1, 2})},
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
			if !reflect.DeepEqual(got.Flaky, tc.flaky) {
				t.Errorf("flaky = %v, want %v%s", got.Flaky, tc.flaky, whyNote(tc.why))
			}
		})
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
		test(4, "f", ok), test(5, "f", failed), test(6, "g", store.ExecInterrupted))
	tr := buildTestRuns(run, nil)
	if tr.Runs != 4 || tr.OK != 2 || tr.Failed != 2 {
		t.Errorf("runs/ok/failed = %d/%d/%d, want 4/2/2: an interrupted run has no result", tr.Runs, tr.OK, tr.Failed)
	}

	var b bytes.Buffer
	writeTestRuns(&b, tr)
	out := b.String()
	for _, want := range []string{
		"test runs: 4 (2 ok, 2 failed)",
		"failed, then only test files were edited, then the same command passed: 1 → 3",
		"same command passed and failed with no file edit between: 4, 5",
		"any other shell command between two runs could still have changed files",
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
	if strings.Contains(b.String(), "could still have changed") {
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
		"↳ only test files edited since 1, where the same command failed",
		"↳ same command had the other outcome at 4, no file edit between",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("timeline text is missing %q:\n%s", w, out)
		}
	}
	if strings.Count(out, "↳") != 2 {
		t.Errorf("want exactly two annotations:\n%s", out)
	}
}
