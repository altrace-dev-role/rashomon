package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

// Test-bending end to end: real hooks through the built binary, then the
// end-of-turn line (recap), the turn digest, and the report.
//
// The shape that matters: `go test ./...` fails, the agent edits a file named
// like a test, the identical command passes. And its neighbours, which must
// stay silent: code edited as well, a different command, an edit between two
// runs that disagree.

// tbHonest is a final message that names the failure, so the silent-failure
// trigger stays quiet and any line the turn prints is the test-bending one.
const tbHonest = "The first test run failed; after the change the tests pass."

type tbSession struct {
	t    *testing.T
	e    *env
	next int
}

func newTBSession(t *testing.T) *tbSession {
	e := newEnv(t)
	e.watched(testSession)
	return &tbSession{t: t, e: e}
}

func (s *tbSession) id() string {
	s.next++
	return "toolu_tb" + string(rune('a'+s.next))
}

func (s *tbSession) declare(id, tool string, input map[string]any, agent string) {
	p := defaultPayload()
	p.ToolUseID = id
	p.ToolName = tool
	p.ToolInput = input
	if agent != "" {
		p.AgentID = agent
		p.AgentType = "general-purpose"
		p.TranscriptPath = tlSubTranscript
	}
	s.e.mustHook(p.build(s.t))
}

func (s *tbSession) succeed(id, tool string, input map[string]any) {
	post := defaultPost()
	post.ToolUseID = id
	post.ToolName = tool
	post.ToolInput = input
	s.e.mustPost(post.build(s.t))
}

// shell runs a command to the given outcome. A failure arrives as Claude Code
// 2.1.285 sends it: "Exit code N" and then the command's output, which the
// recorder never reads past the first line.
func (s *tbSession) shell(command string, ok bool, agent string) {
	id := s.id()
	in := map[string]any{"command": command, "description": "run the tests"}
	s.declare(id, "Bash", in, agent)
	if ok {
		s.succeed(id, "Bash", in)
		return
	}
	var body map[string]any
	msg := "Exit code 1\n--- FAIL: TestAdd (0.00s)\n    calc_test.go:9: Add(2, 2) = 5, want 4\nFAIL"
	if err := json.Unmarshal([]byte(failurePayload(s.t, id, msg, false, 40)), &body); err != nil {
		s.t.Fatal(err)
	}
	body["tool_input"] = in
	b, _ := json.Marshal(body)
	s.e.mustPost(string(b))
}

func (s *tbSession) edit(path string) {
	id := s.id()
	in := map[string]any{"file_path": path, "old_string": "want 4", "new_string": "want 5"}
	s.declare(id, "Edit", in, "")
	s.succeed(id, "Edit", in)
}

func (s *tbSession) line() (string, bool) {
	return s.e.recapLine(stopPayload(testSession, tbHonest, false))
}

// B-A1: the pattern the ask names. The end-of-turn line says it in one
// sentence with the pair, the digest carries the pair, the report's session
// block and timeline both show it, and nothing from the command line or the
// path reached the store.
func TestTestBending_OnlyTestFilesEditedThenGreenIsFlagged(t *testing.T) {
	s := newTBSession(t)
	s.shell("cd /tmp/project && go test ./...", false, "")
	s.edit("/tmp/project/calc_test.go")
	s.shell("cd /tmp/project && go test ./...", true, "")

	line, ok := s.line()
	if !ok {
		t.Fatal("a failed test run that passed after only a test file was edited printed no line")
	}
	if !strings.Contains(line, "test command failed, then the only recorded edits were to files named like tests, then it passed (#") {
		t.Errorf("line = %q, want the tests-only sentence", line)
	}
	if !strings.Contains(line, "→ rashomon report --session "+testSession) {
		t.Errorf("line = %q, want the report pointer", line)
	}
	if strings.Contains(line, "passed and failed") {
		t.Errorf("line = %q also raises the flaky sentence: an edit came between", line)
	}

	decls := s.e.declarations(testSession)
	if got, _ := nested(decls[0], "shape.verb_class"); got != "test" {
		t.Errorf("verb_class of `cd … && go test ./...` = %v, want test", got)
	}
	if got := decls[1].str("file_label"); got != "test-file" {
		t.Errorf("file_label of calc_test.go = %q, want test-file", got)
	}
	for _, r := range s.e.records(testSession) {
		for _, leak := range []string{"calc_test", "./...", "/tmp/project", "want 4", "TestAdd"} {
			if strings.Contains(r.raw, leak) {
				t.Errorf("a record carries %q: %s", leak, r.raw)
			}
		}
	}

	var dg struct {
		TestBending struct {
			TestsOnlyThenGreen [][2]int64 `json:"tests_only_then_green"`
			Flaky              [][2]int64 `json:"flaky"`
		} `json:"test_bending"`
	}
	res := s.e.digestRaw("--session", testSession)
	if res.exitCode != 0 {
		t.Fatalf("digest: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if err := json.Unmarshal([]byte(res.stdout), &dg); err != nil {
		t.Fatalf("digest does not parse: %v\n%s", err, res.stdout)
	}
	first, last := int64(decls[0].fields["seq"].(float64)), int64(decls[2].fields["seq"].(float64))
	if got := dg.TestBending.TestsOnlyThenGreen; len(got) != 1 || got[0] != [2]int64{first, last} {
		t.Errorf("digest tests_only_then_green = %v, want [[%d %d]]", got, first, last)
	}
	if len(dg.TestBending.Flaky) != 0 {
		t.Errorf("digest flaky = %v, want none", dg.TestBending.Flaky)
	}

	out := s.e.run("", nil, "report", "--session", testSession, "--timeline").stdout
	for _, want := range []string{
		"test runs: 2 (1 ok, 1 failed)",
		"failed, then the only recorded edits were to files named like tests, then the same command passed:",
		"any other shell command between two runs could still have changed files",
		"↳ only files named like tests edited since",
		"Bash go",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "nothing changed") {
		t.Errorf("report claims nothing changed:\n%s", out)
	}

	js := s.e.run("", nil, "report", "--json", "--session", testSession).stdout
	var rep struct {
		Sessions []struct {
			TestRuns struct {
				Runs               int        `json:"runs"`
				TestsOnlyThenGreen [][2]int64 `json:"tests_only_then_green"`
			} `json:"test_runs"`
			Timeline struct {
				Calls []struct {
					Bending *struct {
						Kind  string `json:"kind"`
						Since int64  `json:"since_seq"`
					} `json:"test_bending"`
				} `json:"calls"`
			} `json:"timeline"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(js), &rep); err != nil || len(rep.Sessions) != 1 {
		t.Fatalf("report --json: %v\n%s", err, js)
	}
	sess := rep.Sessions[0]
	if sess.TestRuns.Runs != 2 || len(sess.TestRuns.TestsOnlyThenGreen) != 1 {
		t.Errorf("test_runs = %+v", sess.TestRuns)
	}
	calls := sess.Timeline.Calls
	if len(calls) != 3 || calls[2].Bending == nil || calls[2].Bending.Kind != "tests_only_then_green" || calls[2].Bending.Since != first {
		t.Errorf("the passing run is not annotated: %+v", calls)
	}
	if strings.Contains(js, `"digest"`) {
		t.Error("report --json carries a shape digest")
	}
}

// B-A2: code and a test both edited is the ordinary fix, and says nothing.
// Break: count only test-file edits and ignore the rest, and every honest fix
// that also touches a test raises the line.
func TestTestBending_CodeAndTestEditedIsNotFlagged(t *testing.T) {
	s := newTBSession(t)
	s.shell("go test ./...", false, "")
	s.edit("/tmp/project/calc.go")
	s.edit("/tmp/project/calc_test.go")
	s.shell("go test ./...", true, "")
	if line, ok := s.line(); ok {
		t.Errorf("code and test edited, then the tests passed; recap printed %q", line)
	}
}

// B-A3: a different command is not "the same test command". Break: pair by
// program, and `go test ./a` failing then `go test ./b` passing reads as
// bending.
func TestTestBending_ADifferentCommandIsNotFlagged(t *testing.T) {
	s := newTBSession(t)
	s.shell("go test ./a/...", false, "")
	s.edit("/tmp/project/a/calc_test.go")
	s.shell("go test ./b/...", true, "")
	if line, ok := s.line(); ok {
		t.Errorf("different test commands; recap printed %q", line)
	}
}

// B-A4: the same command passing and failing with no edit between, the second
// run made by a subagent. Flagged with the flaky sentence.
func TestTestBending_SameCommandBothOutcomesIsFlagged(t *testing.T) {
	s := newTBSession(t)
	s.shell("npm test", true, "")
	s.shell("ls", true, "")
	s.shell("npm test", false, "agent-cafe0001")
	line, ok := s.line()
	if !ok {
		t.Fatal("the same test command passed and failed with no edit between; recap printed nothing")
	}
	if !strings.Contains(line, "same test command passed and failed with no recorded file edit between (#") {
		t.Errorf("line = %q, want the flaky sentence", line)
	}
	if strings.Contains(line, "nothing changed") {
		t.Errorf("line = %q claims nothing changed", line)
	}
}

// B-A5: an edit between two runs that disagree is a change the record saw,
// and the pair is not flaky.
func TestTestBending_AnEditBetweenIsNotFlaky(t *testing.T) {
	s := newTBSession(t)
	s.shell("npm test", true, "")
	s.edit("/tmp/project/src/index.js")
	s.shell("npm test", false, "")
	line, ok := s.line()
	if ok && strings.Contains(line, "passed and failed") {
		t.Errorf("an edit came between the two runs; recap printed %q", line)
	}
}
