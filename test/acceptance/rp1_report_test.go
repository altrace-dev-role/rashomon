package acceptance

// REPORT1a: the session's impact, on the Stop line (the AutoSteer clause) and
// in `rashomon report --session` (a first screen and an autosteer block).
// One derivation, recap.ImpactOf, feeds both, so these tests drive the real
// hooks and the real binary and read the two places against each other.
// Every number is a count of records with seqs beside it (D162: evidence,
// never a score).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// rpFinding writes a regression-location finding line to sess-1's steer log:
// delivered with a model text, naming the gate go, cited at evidence seq.
func (e *env) rpFinding(t *testing.T, prompt string, seq int) {
	t.Helper()
	line, err := json.Marshal(map[string]any{"type": "steer", "id": "st-" + prompt, "state": "delivered", "prompt_id": prompt,
		"event": "PostToolUseFailure", "pattern": "regression-location", "level": "nudge", "model_text": "go test failed",
		"after_seq": seq, "evidence_seqs": []int{seq}, "slots": map[string]any{"gate": "go"}})
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(filepath.Join(e.home, "runs", testSession, "steer.ndjson"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		t.Fatal(err)
	}
}

// rpSession records a failed go test with its finding, then a green turn: the
// finding is resolved by the later green. It returns the first Stop's line.
func (e *env) rpSession(t *testing.T) string {
	t.Helper()
	e.watched(testSession)
	e.rpCall(t, "prompt-1", "toolu_red", goTestRun("failed"))
	e.rpFinding(t, "prompt-1", 1)
	e.rpGreen(t, "prompt-2")
	return e.mustSpeak(stopEvent("Stop", "Updated main.go.", e.cwd))
}

// rpCall is turnCall with the declared cwd the test's own directory, so the
// checkpoint's project root is the one `--session last` asks about.
func (e *env) rpCall(t *testing.T, prompt, id string, c fixtureCall) {
	t.Helper()
	pre := defaultPayload()
	pre.CWD, pre.PromptID, pre.ToolUseID, pre.ToolName, pre.ToolInput = e.cwd, prompt, id, c.Tool, c.Input
	e.mustHook(pre.build(t))
	post := defaultPost()
	post.ToolUseID, post.ToolName, post.ToolInput = id, c.Tool, c.Input
	switch c.Outcome {
	case "ok":
		post.ToolResponse = map[string]any{"stdout": c.Output, "stderr": "", "interrupted": false}
		if c.Tool != "Bash" {
			post.ToolResponse = map[string]any{"output": c.Output}
		}
		e.mustPost(post.build(t))
	case "failed":
		b, err := json.Marshal(map[string]any{
			"hook_event_name": "PostToolUseFailure", "session_id": testSession, "transcript_path": "/tmp/transcripts/sess-1.jsonl",
			"cwd": e.cwd, "permission_mode": "default", "tool_name": c.Tool, "tool_input": c.Input, "tool_use_id": id,
			"error": "Exit code 1", "is_interrupt": false, "duration_ms": 30,
		})
		if err != nil {
			t.Fatal(err)
		}
		e.mustPost(string(b))
	}
}

func (e *env) rpGreen(t *testing.T, prompt string) {
	t.Helper()
	e.rpCall(t, prompt, "toolu_"+prompt+"_edit", editOf("main.go"))
	e.rpCall(t, prompt, "toolu_"+prompt+"_test", goTestRun("ok"))
}

// The Stop line's clause at the level of detail that fits eighty columns with
// a finding resolved: the one note is counted, not titled.
var rpClause = regexp.MustCompile(`AutoSteer: 1 note given; 1 finding, resolved at #(\d+)\.`)

func TestRP1_ClauseCountsDeliveredFindingsResolvedChecks(t *testing.T) {
	e := newEnv(t)
	line := e.rpSession(t)
	m := rpClause.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("the Stop line is %q, want the clause naming one note, one finding and the green that resolved it", line)
	}
	if !strings.Contains(line, "verified - go passed after the last edit") {
		t.Errorf("the state sentence is missing from %q", line)
	}
	if n := len(strings.Split(strings.TrimRight(line, "\n"), "\n")); n != 3 {
		t.Errorf("the line is %d lines, want 3: sentence, clause, pointer", n)
	}
}

func TestRP1b_ClausePrintsOnlyOnChange(t *testing.T) {
	e := newEnv(t)
	first := e.rpSession(t)
	if !strings.Contains(first, "AutoSteer:") {
		t.Fatalf("the first Stop printed %q, want the clause", first)
	}
	// The same records, a new turn: the counts did not change.
	e.rpGreen(t, "prompt-3")
	second := e.mustSpeak(stopEvent("Stop", "Updated main.go again.", e.cwd))
	if strings.Contains(second, "AutoSteer:") || !strings.Contains(second, "verified") {
		t.Errorf("the second Stop printed %q, want the value line without the clause", second)
	}
	// A new finding changes them.
	e.rpFinding(t, "prompt-3", 2)
	e.rpGreen(t, "prompt-4")
	third := e.mustSpeak(stopEvent("Stop", "Once more.", e.cwd))
	if !strings.Contains(third, "AutoSteer: 2 notes given; 2 findings, 2 resolved") {
		t.Errorf("the third Stop printed %q, want the clause with the new counts", third)
	}
}

func TestRP1c_SummaryOffSilencesTheClauseNotTheDoneFlag(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	if res := e.run("", nil, "summary", "off"); res.exitCode != 0 || res.stdout != "summary: off\n" {
		t.Fatalf("summary off: exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
	}
	b, err := os.ReadFile(filepath.Join(e.home, "config.json"))
	if err != nil || strings.TrimSpace(string(b)) != `{"summary":false}` {
		t.Fatalf("config.json is %q (%v), want the persisted switch", b, err)
	}
	e.rpCall(t, "prompt-1", "toolu_red", goTestRun("failed"))
	e.rpFinding(t, "prompt-1", 1)
	e.rpGreen(t, "prompt-2")
	if line, ok := e.recapLine(stopEvent("Stop", "Updated main.go.", e.cwd)); ok {
		t.Errorf("with summary off the Stop printed %q, want nothing", line)
	}
	// The done flag is not the summary: a claim over a gate that did not run
	// after the last edit still speaks.
	e.rpCall(t, "prompt-3", "toolu_edit", editOf("util.go"))
	if line := e.mustSpeak(stopEvent("Stop", "All 42 tests pass.", e.cwd)); !strings.Contains(line, "done was claimed") {
		t.Errorf("with summary off a done claim printed %q, want the done flag", line)
	}
	if res := e.run("", nil, "summary", "on"); res.stdout != "summary: on\n" {
		t.Errorf("summary on printed %q", res.stdout)
	}
	if res := e.run("", nil, "summary", "--help"); res.exitCode == 0 {
		t.Errorf("summary --help exited 0, want a refusal like every verb")
	}
}

func TestRP1d_ReportBlockMatchesTheClause(t *testing.T) {
	e := newEnv(t)
	line := e.rpSession(t)
	m := rpClause.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("no clause in %q", line)
	}
	res := e.run("", nil, "report", "--session", testSession)
	if res.exitCode != 0 {
		t.Fatalf("report: exit %d, %s", res.exitCode, res.stderr)
	}
	story := "    autosteer: 1 regression location given at the failure; 1 finding, resolved at #" + m[1] + "\n"
	block := "    findings: 1 regression location at #1 (go), resolved by #" + m[1] + " green\n"
	for _, want := range []string{story, block, "    delivered: 1 note (1 regression location) at #1 with prompt prompt-1\n"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("the report lacks %q:\n%s", want, res.stdout)
		}
	}
}

func TestRP1e_EveryCountHasItsSeqs(t *testing.T) {
	e := newEnv(t)
	e.rpSession(t)
	res := e.run("", nil, "report", "--session", testSession, "--json")
	if res.exitCode != 0 {
		t.Fatalf("report --json: exit %d, %s", res.exitCode, res.stderr)
	}
	var rep struct {
		Sessions []struct {
			Autosteer struct {
				Stop      struct{ Seq int64 }
				Clause    string
				Delivered []struct {
					ID  string
					Seq int64
				}
				Findings []struct {
					ID         string
					Seq        int64
					State      string
					ResolvedBy int64 `json:"resolved_by"`
				}
				Gates []struct {
					Name         string
					LastGreenSeq *int64 `json:"last_green_seq"`
				}
			}
		}
	}
	if err := json.Unmarshal([]byte(res.stdout), &rep); err != nil || len(rep.Sessions) != 1 {
		t.Fatalf("report --json: %v, %d sessions", err, len(rep.Sessions))
	}
	a := rep.Sessions[0].Autosteer
	if a.Stop.Seq == 0 || len(a.Delivered) != 1 || len(a.Findings) != 1 || len(a.Gates) != 1 {
		t.Fatalf("autosteer %+v, want a stop seq, one note, one finding and one gate", a)
	}
	f := a.Findings[0]
	if f.ID == "" || f.Seq != 1 || f.State != "resolved" || f.ResolvedBy <= f.Seq || f.ResolvedBy > a.Stop.Seq {
		t.Errorf("finding %+v, want its id, seq 1 and a green seq after it within the Stop", f)
	}
	if a.Delivered[0].ID == "" || a.Delivered[0].Seq != 1 {
		t.Errorf("note %+v, want its id and seq", a.Delivered[0])
	}
	if a.Gates[0].LastGreenSeq == nil || *a.Gates[0].LastGreenSeq != f.ResolvedBy {
		t.Errorf("gate %+v, want the green run that resolved the finding", a.Gates[0])
	}
}

func TestRP1f_NoFindingsPrintsNoFindingsLine(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.rpGreen(t, "prompt-1")
	if line := e.mustSpeak(stopEvent("Stop", "Updated main.go.", e.cwd)); line != verifiedLine {
		t.Errorf("an issue-free Stop printed %q, want the plain value line", line)
	}
	res := e.run("", nil, "report", "--session", testSession)
	for _, want := range []string{"    findings: none\n", "    delivered: none\n", "    checks: none expected\n", "    autosteer: no notes, findings or checks to count\n"} {
		if !strings.Contains(res.stdout, want) {
			t.Errorf("the report lacks %q:\n%s", want, res.stdout)
		}
	}
}

func TestRP1h_SessionLast(t *testing.T) {
	e := newEnv(t)
	e.rpSession(t)
	last := e.run("", nil, "report", "--session", "last")
	named := e.run("", nil, "report", "--session", testSession)
	if last.exitCode != 0 || last.stdout != named.stdout {
		t.Errorf("--session last: exit %d, %s\n%s", last.exitCode, last.stderr, last.stdout)
	}
	// Another project's checkpoint is not this one's last session.
	elsewhere := t.TempDir()
	cmd := e.command("", nil, "report", "--session", "last")
	cmd.Dir = elsewhere
	if res := e.wait(cmd); res.exitCode == 0 || !strings.Contains(res.stderr, "no session of this project holds a checkpoint") {
		t.Errorf("from another project: exit %d, stderr %q, want the refusal", res.exitCode, res.stderr)
	}
}

// TestH121_TheFirstScreenIsUnderTwentyLines: the report of a session opens
// with its story, and the story is over, "details below" included, inside
// twenty lines of the top, store-wide header lines counted; the tables follow
// it (Rishi's finding: the report is dense; the acceptance is the story in
// under twenty lines, tables below).
func TestH121_TheFirstScreenIsUnderTwentyLines(t *testing.T) {
	e := newEnv(t)
	e.rpSession(t)
	res := e.run("", nil, "report", "--session", testSession)
	lines := strings.Split(res.stdout, "\n")
	end, table := -1, -1
	for i, l := range lines {
		if strings.HasPrefix(l, "    details below") {
			end = i
		}
		if strings.HasPrefix(l, "  the agent's account") {
			table = i
		}
	}
	if end < 0 || table != end+1 {
		t.Fatalf("the story's last line is %d and the first table %d, want the table right after:\n%s", end, table, res.stdout)
	}
	if end+1 >= 20 {
		t.Errorf("the first screen is %d lines, want under twenty:\n%s", end+1, res.stdout)
	}
	for _, want := range []string{"  story\n", "    last Stop: verified", "    autosteer: ", "    gates: go green at #"} {
		if !strings.Contains(strings.Join(lines[:end+1], "\n"), want) {
			t.Errorf("the first screen lacks %q:\n%s", want, strings.Join(lines[:end+1], "\n"))
		}
	}
}
