package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// A call whose shape says a build or test runner's exit status was masked by
// the rest of its line: `make test 2>&1 | tail -40` and the like. Its own
// outcome is the line's, and says nothing about the runner.

// maskedCall is a shell call of make, class execute, whose runner's status
// was masked; kind is test or build, runner its runner digest.
func maskedCall(seq int64, kind, runner, outcome string) tbCall {
	return tbCall{seq: seq, tool: "Bash", verb: shape.VerbExecute, prog: "make",
		digest: "m" + runner, outcome: outcome, masked: kind, runner: runner}
}

// plainRun is a test run whose status its line returns, of runner digest
// runner: `make test`.
func plainRun(seq int64, runner, outcome string) tbCall {
	c := test(seq, "p"+runner, outcome)
	c.prog, c.masked, c.runner = "make", shape.MaskedNone, runner
	return c
}

// TestMasked_TestRunsCountThemAsUnobserved: a masked test run is a test run,
// counted apart, and neither ok nor failed whatever its line recorded: a
// failed line may be the tail that failed, and an ok one is no pass. A
// masked build is no test run, and one moved to the background has no
// outcome at all.
func TestMasked_TestRunsCountThemAsUnobserved(t *testing.T) {
	const ok, failed = store.ExecOK, store.ExecFailed
	bg := maskedCall(5, shape.MaskedTest, "r", ok)
	bg.bg = true
	tr := testRunsOf(tbRun(maskedCall(1, shape.MaskedTest, "r", ok), maskedCall(2, shape.MaskedTest, "r", failed),
		test(3, "d", ok), maskedCall(4, shape.MaskedBuild, "b", ok), bg))
	if tr.Runs != 3 || tr.OK != 1 || tr.Failed != 0 || tr.Masked == nil || *tr.Masked != 2 {
		t.Errorf("runs/ok/failed/masked = %d/%d/%d/%v, want 3/1/0/2", tr.Runs, tr.OK, tr.Failed, tr.Masked)
	}
	var b bytes.Buffer
	writeTestRuns(&b, tr, "s1")
	if want := "test runs: 3 (1 ok, 0 failed, 2 whose line did not return the test run's exit status, so neither)"; !strings.Contains(b.String(), want) {
		t.Errorf("text is missing %q:\n%s", want, b.String())
	}
	j, _ := json.Marshal(tr)
	if !strings.Contains(string(j), `"status_masked":2`) {
		t.Errorf("json does not carry the count: %s", j)
	}

	// A session whose only test runs were masked still has the block.
	b.Reset()
	writeTestRuns(&b, testRunsOf(tbRun(maskedCall(1, shape.MaskedTest, "r", ok))), "s1")
	if !strings.Contains(b.String(), "test runs: 1 (0 ok, 0 failed, 1 whose line did not return") {
		t.Errorf("a session of masked runs only:\n%s", b.String())
	}
	// None masked: the line reads as it did.
	b.Reset()
	writeTestRuns(&b, testRunsOf(tbRun(test(1, "d", ok))), "s1")
	if !strings.Contains(b.String(), "test runs: 1 (1 ok, 0 failed)\n") {
		t.Errorf("with nothing masked:\n%s", b.String())
	}
}

// TestMasked_NeverAPair: a masked run is not a run with a result, so it
// neither starts nor finishes a pattern, and it is an edit between two runs
// as any test run is. Break: read a masked line's ok as a pass, and `make
// test | tail` failing at the tail, then ok, is "flaky"; or read through one
// between a failing and a passing run, and a pair completes over a run that
// may have rewritten files.
func TestMasked_NeverAPair(t *testing.T) {
	const ok, failed = store.ExecOK, store.ExecFailed
	// Two masked runs of one line with both outcomes, nothing between. The
	// class is test here, as a reader that took the masked line for a test
	// run would have it.
	a, b := maskedCall(1, shape.MaskedTest, "r", failed), maskedCall(2, shape.MaskedTest, "r", ok)
	a.verb, b.verb = shape.VerbTest, shape.VerbTest
	if tb := DetectTestBending(tbRun(a, b), nil); len(tb.Flaky)+len(tb.TestsOnlyThenGreen) != 0 {
		t.Errorf("two masked runs made a pair: %+v", tb)
	}
	// And a failed masked run, a test-file edit, an ok one.
	a, b = maskedCall(1, shape.MaskedTest, "r", failed), maskedCall(3, shape.MaskedTest, "r", ok)
	a.verb, b.verb = shape.VerbTest, shape.VerbTest
	if tb := DetectTestBending(tbRun(a, edit(2, shape.LabelTestFile, ok), b), nil); len(tb.Flaky)+len(tb.TestsOnlyThenGreen) != 0 {
		t.Errorf("masked runs made a tests-only pair: %+v", tb)
	}

	// Between two plain runs: a masked line is an edit, even one whose
	// class is read and whose shape does not say it may write.
	between := maskedCall(2, shape.MaskedTest, "r", ok)
	between.verb = shape.VerbRead
	if tb := DetectTestBending(tbRun(test(1, "d", failed), between, test(3, "d", ok)), nil); len(tb.Flaky) != 0 {
		t.Errorf("a pair completed across a masked run: %+v", tb.Flaky)
	}
	// The control: the same read, not masked, is no edit.
	between.masked = shape.MaskedNone
	if tb := DetectTestBending(tbRun(test(1, "d", failed), between, test(3, "d", ok)), nil); len(tb.Flaky) != 1 {
		t.Errorf("the control found no pair across a plain read: %+v", tb.Flaky)
	}
}

// TestMasked_TimelineRow: a masked call that recorded ok is not an ok row:
// its outcome says the status was masked, in the unknown group, and it is no
// later success for a failure. One that recorded failed is failed, as the
// line was.
func TestMasked_TimelineRow(t *testing.T) {
	const ok, failed = store.ExecOK, store.ExecFailed
	run := tbRun(maskedCall(1, shape.MaskedTest, "r", failed), maskedCall(2, shape.MaskedTest, "r", ok),
		maskedCall(3, shape.MaskedBuild, "b", ok),
		tbCall{seq: 4, tool: "Bash", verb: shape.VerbExecute, digest: "x", outcome: ok, masked: shape.MaskedNone})
	// Each result recorded where its call was, so a later one is later.
	for i := range run.Executions {
		seq := int64(10 + i)
		run.Executions[i].Seq = &seq
	}
	tl := buildTimeline(run, nil)
	want := []struct{ group, outcome string }{
		{GroupFailed, failed},
		{GroupUnknown, LinkOutcomeStatusMasked},
		{GroupUnknown, LinkOutcomeStatusMasked},
		{GroupOK, ok},
	}
	for i, w := range want {
		c := tl.Calls[i]
		if c.Group != w.group || c.Outcome != w.outcome {
			t.Errorf("row %d: group %s outcome %q, want %s %q", i+1, c.Group, c.Outcome, w.group, w.outcome)
		}
	}
	if c := tl.Calls[0]; c.Later != nil || !c.LaterChecked {
		t.Errorf("the failed masked line's later success = %+v (checked %v); the identical masked line's ok is no success", c.Later, c.LaterChecked)
	}
	if n := tl.Counts; n.OK != 1 || n.Unknown != 2 || n.Failed != 1 {
		t.Errorf("counts ok/unknown/failed = %d/%d/%d, want 1/2/1", n.OK, n.Unknown, n.Failed)
	}

	var b bytes.Buffer
	writeTimeline(&b, tl)
	out := b.String()
	if !strings.Contains(out, LinkOutcomeStatusMasked) {
		t.Errorf("no row says the status was masked:\n%s", out)
	}
	if !strings.Contains(out, "or ok as a line that did not return a build or test run's exit status") {
		t.Errorf("the unknown legend does not name masked rows:\n%s", out)
	}
}

// TestMasked_Runs: the masked runs the end-of-turn line counts, and when it
// fires. Counted: a masked call that recorded ok and not in the background,
// with no later call that ran the same runner (runner digest) in the same
// directory with its status the line's, and recorded a result. Fires: the
// final message was read, claims a pass, and says no failure word.
func TestMasked_Runs(t *testing.T) {
	const ok, failed = store.ExecOK, store.ExecFailed
	const pass = "Done. The changes are in, the build passes, all tests pass, and it's ready to merge."
	m := maskedCall(1, shape.MaskedTest, "r", ok)
	bgPlain := plainRun(2, "r", ok)
	bgPlain.bg = true
	otherDir := plainRun(2, "r", ok)
	otherDir.cwd = "elsewhere"
	bgMasked := maskedCall(1, shape.MaskedTest, "r", ok)
	bgMasked.bg = true
	for _, tc := range []struct {
		name  string
		run   *store.Run
		msg   string
		runs  int
		fires bool
	}{
		{"a masked test, a pass claimed", tbRun(m), pass, 1, true},
		{"a masked build counts too", tbRun(maskedCall(1, shape.MaskedBuild, "b", ok)), pass, 1, true},
		{"re-run plainly later, passed", tbRun(m, plainRun(2, "r", ok)), pass, 0, false},
		{"re-run plainly later, failed: that failure is the failed-calls line's", tbRun(m, plainRun(2, "r", failed)), pass, 0, false},
		{"the plain run came first", tbRun(plainRun(0, "r", ok), m), pass, 1, true},
		{"a later plain run of another runner", tbRun(m, plainRun(2, "other", ok)), pass, 1, true},
		{"a later plain run in another directory", tbRun(m, otherDir), pass, 1, true},
		{"a later plain run moved to the background", tbRun(m, bgPlain), pass, 1, true},
		{"a later plain run with no execution record", tbRun(m, plainRun(2, "r", "")), pass, 1, true},
		{"a later masked run of the same runner", tbRun(m, maskedCall(2, shape.MaskedTest, "r", ok)), pass, 2, true},
		{"the masked line failed: a recorded failure, not a masked one", tbRun(maskedCall(1, shape.MaskedTest, "r", failed)), pass, 0, false},
		{"moved to the background: unobserved", tbRun(bgMasked), pass, 0, false},
		{"the message names a failure", tbRun(m), "Two tests still fail; the rest pass.", 1, false},
		{"the message claims no pass", tbRun(m), "I updated the README and the docs.", 1, false},
		{"no message", tbRun(m), "", 1, false},
		{"nothing masked", tbRun(plainRun(1, "r", ok)), pass, 0, false},
	} {
		got := BuildMaskedRuns(tc.run, AccountFromMessage(tc.msg))
		if got.Runs != tc.runs || got.Fires != tc.fires {
			t.Errorf("%s: runs %d fires %v, want %d %v", tc.name, got.Runs, got.Fires, tc.runs, tc.fires)
		}
	}
	got := BuildMaskedRuns(tbRun(m), AccountFromMessage(pass))
	if !got.PassClaimed || !got.FinalMessageAvailable {
		t.Errorf("pass_claimed %v, final_message_available %v; want both", got.PassClaimed, got.FinalMessageAvailable)
	}
	if got := BuildMaskedRuns(nil, AccountFromMessage(pass)); got.Runs != 0 || got.Fires {
		t.Errorf("a nil run: %+v", got)
	}
}

// TestMasked_SessionText: the report says how many runs were masked beside
// the failed calls, and, when the line would fire, why.
func TestMasked_SessionText(t *testing.T) {
	var b bytes.Buffer
	writeMaskedRuns(&b, MaskedRuns{Runs: 2, PassClaimed: true, FinalMessageAvailable: true, Fires: true})
	out := b.String()
	for _, want := range []string{
		"masked exit status: 2 build or test call(s) recorded ok while their line did not return the runner's exit status, and no later run matched to the same runner in the same directory returned it",
		"the final message claims a pass and uses no failure word",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("text is missing %q:\n%s", want, out)
		}
	}
	b.Reset()
	writeMaskedRuns(&b, MaskedRuns{})
	if b.Len() != 0 {
		t.Errorf("rendered with nothing masked: %q", b.String())
	}
}

// derivedRun is a session of shell calls as the hooks record them: each
// call's shape from shape.Derive, its cwd digest from the payload's cwd with
// the line's leading `cd DIR &&` steps folded in, and its execution's outcome.
func derivedRun(t *testing.T, calls ...[3]string) *store.Run {
	t.Helper()
	key := []byte("install key")
	run := &store.Run{}
	for n, c := range calls {
		cmd, cwd, outcome := c[0], c[1], c[2]
		raw, err := json.Marshal(map[string]string{"command": cmd})
		if err != nil {
			t.Fatal(err)
		}
		dirs, _ := shape.LeadingDirectory("Bash", raw)
		for _, d := range dirs {
			if filepath.IsAbs(d) {
				cwd = filepath.Clean(d)
			} else {
				cwd = filepath.Join(cwd, d)
			}
		}
		id := fmt.Sprintf("toolu_%d", n+1)
		seq := int64(n + 1)
		run.Declarations = append(run.Declarations, store.Declaration{
			SchemaVersion: store.SchemaVersion, Seq: seq, ToolUseID: id, ToolName: "Bash", SessionID: "s1",
			Shape: shape.Derive("Bash", raw, key), CWDDigest: shape.CWDDigest(key, cwd),
		})
		run.Executions = append(run.Executions, store.Execution{
			SchemaVersion: store.SchemaVersion, ToolUseID: id, ToolName: "Bash", Outcome: outcome,
			ExecutedDigest: shape.Derive("Bash", raw, key).Digest,
		})
	}
	return run
}

// TestMasked_RunsFromDerive: which later run clears a masked run, judged on
// the shapes Derive records. The first call is masked and recorded ok, the
// second recorded ok, the final message claims a pass. A later run clears it
// only when its line returns the same runner's failure, from the same
// directory. Break: carry no runner digest on `make test && echo ok`, or
// none for words the tokenizer cannot vouch for; carry one runner's digest
// for a line that hid two; read `make test &` as returning make's status;
// give a runner after an unfolded cd a digest; or read as none a line that
// returns make's failure only in a shell where errexit fires or PIPESTATUS
// is set.
func TestMasked_RunsFromDerive(t *testing.T) {
	const ok = store.ExecOK
	const msg = "Done. All tests pass."
	for _, tc := range []struct {
		name          string
		masked, later [3]string
		fires         bool
	}{
		{"a later && whose failure is make's",
			[3]string{"make test 2>&1 | tail -40", "/repo", ok}, [3]string{"make test && echo ok", "/repo", ok}, false},
		{"the same words with an expansion in them",
			[3]string{"go test $(go list ./...) | tail", "/repo", ok}, [3]string{"go test $(go list ./...)", "/repo", ok}, false},
		{"a later line under pipefail",
			[3]string{"go test ./... 2>&1 | tail -5", "/repo", ok}, [3]string{"set -o pipefail; go test ./... 2>&1 | tail -5", "/repo", ok}, false},
		{"make lint never re-ran",
			[3]string{"make lint 2>&1 | tail -20; make test 2>&1 | tail -40", "/repo", ok}, [3]string{"make test", "/repo", ok}, true},
		{"a later launch with &",
			[3]string{"make test | tail", "/repo", ok}, [3]string{"make test &", "/repo", ok}, true},
		{"the later run is in another directory",
			[3]string{"cd sub; make test | tail", "/repo", ok}, [3]string{"cd /repo && make test", "/repo/sub", ok}, true},
		{"after an unfolded cd the runner's directory is not known",
			[3]string{"cd sub; make test | tail", "/repo", ok}, [3]string{"make test", "/repo/sub", ok}, true},
		{"a plain re-run",
			[3]string{"make test 2>&1 | tail -40", "/repo", ok}, [3]string{"make test", "/repo", ok}, false},
		{"a later exit with PIPESTATUS, which zsh does not have",
			[3]string{"make test 2>&1 | tail -40", "/repo", ok}, [3]string{"make test 2>&1 | tail -40; exit ${PIPESTATUS[0]}", "/repo", ok}, true},
		{"a later set -e line whose errexit zsh in the wrapper never fires",
			[3]string{"make test 2>&1 | tail -40", "/repo", ok}, [3]string{"set -euo pipefail; make test 2>&1 | tail -40; echo \"exit=$?\"", "/repo", ok}, true},
		{"a later set -euo pipefail line that returns make's failure in every shell",
			[3]string{"make test 2>&1 | tail -40", "/repo", ok}, [3]string{"set -euo pipefail; make test 2>&1 | tail -40", "/repo", ok}, false},
	} {
		run := derivedRun(t, tc.masked, tc.later)
		if m := run.Declarations[0].Shape.StatusMasked; m == nil || *m == shape.MaskedNone {
			t.Errorf("%s: %q is not masked", tc.name, tc.masked[0])
			continue
		}
		got := BuildMaskedRuns(run, AccountFromMessage(msg))
		if got.Fires != tc.fires {
			t.Errorf("%s: fires %v (runs %d), want %v", tc.name, got.Fires, got.Runs, tc.fires)
		}
	}
}

// TestMasked_AsRan: MaskingAsRan takes status_masked and runner_digest from
// the call's outcome record where it is at schema 4 and saw a tool_input,
// and leaves the declaration's otherwise. Break: take them from a record that
// could not carry them, or from one that saw no input, and a v3 record or a
// payload without tool_input wipes a declaration's masking to null.
func TestMasked_AsRan(t *testing.T) {
	str := func(s string) *string { return &s }
	decl := func(id string) store.Declaration {
		return store.Declaration{SchemaVersion: store.SchemaVersion, ToolUseID: id, ToolName: "Bash",
			Shape: shape.Shape{StatusMasked: str(shape.MaskedTest), RunnerDigest: str("declared")}}
	}
	run := &store.Run{
		Declarations: []store.Declaration{decl("a"), decl("b"), decl("c")},
		Executions: []store.Execution{
			{SchemaVersion: 4, ToolUseID: "a", Outcome: store.ExecOK, ExecutedDigest: "x",
				StatusMasked: str(shape.MaskedNone), RunnerDigest: str("ran")},
			{SchemaVersion: 3, ToolUseID: "b", Outcome: store.ExecOK, ExecutedDigest: "x"},
			{SchemaVersion: 4, ToolUseID: "c", Outcome: store.ExecOK},
		},
	}
	MaskingAsRan(run)
	d := run.Declarations
	if m, r := d[0].Shape.StatusMasked, d[0].Shape.RunnerDigest; m == nil || *m != shape.MaskedNone || r == nil || *r != "ran" {
		t.Errorf("a v4 record that saw the input: status_masked %v, runner_digest %v; want the execution's", m, r)
	}
	for i, why := range map[int]string{1: "a v3 record", 2: "a record that saw no tool_input"} {
		if m, r := d[i].Shape.StatusMasked, d[i].Shape.RunnerDigest; m == nil || *m != shape.MaskedTest || r == nil || *r != "declared" {
			t.Errorf("%s replaced the declaration's masking: %v, %v", why, m, r)
		}
	}
	MaskingAsRan(nil)
}

// TestMasked_NotMeasuredBeforeSchema4: a session whose declarations are all
// from before schema 4 never measured masking, so its masked runs and its
// test runs' masked count are null, never 0 -- the same bytes as a measured
// zero. Break: build them for any session, or count a schema-3 session as
// measured.
func TestMasked_NotMeasuredBeforeSchema4(t *testing.T) {
	const pass = "Done. All tests pass."
	v3 := tbRun(test(1, "d", store.ExecOK), tbCall{seq: 2, tool: "Bash", verb: shape.VerbExecute, digest: "x", outcome: store.ExecOK})
	for i := range v3.Declarations {
		v3.Declarations[i].SchemaVersion = 3
	}
	j, _ := json.Marshal(Session{MaskedRuns: sessionMaskedRuns(v3, AccountFromMessage(pass))})
	if !strings.Contains(string(j), `"masked_runs":null`) {
		t.Errorf("a schema-3 session's masked runs: %s", j)
	}
	tr := testRunsOf(v3)
	if tr == nil {
		t.Fatal("a schema-3 session with a test run has no test runs")
	}
	if j, _ := json.Marshal(tr); !strings.Contains(string(j), `"status_masked":null`) {
		t.Errorf("a schema-3 session's test runs: %s", j)
	}
	var b bytes.Buffer
	writeTestRuns(&b, tr, "s1")
	if !strings.Contains(b.String(), "test runs: 1 (1 ok, 0 failed)\n") {
		t.Errorf("a schema-3 session's test runs line:\n%s", b.String())
	}

	// At schema 4 both are measured, and a zero is a zero.
	v4 := tbRun(test(1, "d", store.ExecOK))
	mr := sessionMaskedRuns(v4, AccountFromMessage(pass))
	if mr == nil || mr.Runs != 0 {
		t.Errorf("a schema-4 session's masked runs = %+v, want a measured 0", mr)
	}
	if tr := testRunsOf(v4); tr.Masked == nil || *tr.Masked != 0 {
		t.Errorf("a schema-4 session's masked test runs = %v, want a measured 0", tr.Masked)
	}
}

// TestMasked_PassWordsAreWords: a pass is claimed by a whole word of
// passVocabulary that no negation before it in its clause negates, where
// "and" and "but" end a negation. Break: match a pass word inside another
// word, and "password", "bypass" and "greenfield" claim a pass; read a
// negated one as a claim, and "the tests do not pass" does; let a negation
// reach across a clause, or past "and" or "but", and "No regressions, all
// tests pass." claims none; or drop none, nothing, neither, nor and without,
// and "None of the tests pass yet." claims one.
func TestMasked_PassWordsAreWords(t *testing.T) {
	run := tbRun(maskedCall(1, shape.MaskedTest, "r", store.ExecOK))
	for _, tc := range []struct {
		msg   string
		fires bool
	}{
		{"I rotated the password in the config.", false},
		{"Added a bypass for the cache.", false},
		{"Set up the greenfield service skeleton.", false},
		{"The tests don't pass yet.", false},
		{"The tests do not pass.", false},
		{"The suite never passes on this branch.", false},
		{"The tests don\u2019t pass yet.", false},
		{"The checks are not all green.", false},
		{"Not all tests pass yet.", false},
		{"None of the tests pass yet.", false},
		{"I'm not sure the tests pass.", false},
		{"Nothing passes yet.", false},
		{"Neither suite passes.", false},
		{"Not every test passes.", false},
		{"Not a single test passes.", false},

		{"All tests pass.", true},
		{"The build is green.", true},
		{"Tests passed successfully.", true},
		{"It builds now.", true},
		{"Done: \"pass\" on every package.", true},
		{"The password field is fixed and the tests pass.", true},
		{"No regressions, all tests pass.", true},
		{"No API changes and all tests pass.", true},
		{"Not all of it is done, but the build is green.", true},
		{"Nothing else changed; all tests pass.", true},
		{"If the tests pass, merge it.", true},
	} {
		got := BuildMaskedRuns(run, AccountFromMessage(tc.msg))
		if got.Fires != tc.fires || got.PassClaimed != tc.fires {
			t.Errorf("%q: fires %v pass_claimed %v, want %v", tc.msg, got.Fires, got.PassClaimed, tc.fires)
		}
	}
}
