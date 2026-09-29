package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

// The timeline end to end: real hooks for the main agent and a subagent, then
// the built binary's `report --timeline`, as text and as JSON.
//
// The session: the main agent's `pytest -q` fails, a subagent runs the same
// command and it passes, the main agent's `make` fails and is never re-run,
// and one more call is declared with no execution record.

const tlSubTranscript = "/tmp/transcripts/sess-1.jsonl/subagents/agent-cafe0001.jsonl"

func recordTimelineSession(t *testing.T, e *env) {
	t.Helper()
	e.watched(testSession)

	call := func(id, command, agent string) {
		p := defaultPayload()
		p.ToolUseID = id
		p.ToolInput = map[string]any{"command": command}
		if agent != "" {
			p.AgentID = agent
			p.AgentType = "general-purpose"
			p.TranscriptPath = tlSubTranscript
		}
		e.mustHook(p.build(t))
	}
	fail := func(id, command, code string) {
		var body map[string]any
		if err := json.Unmarshal([]byte(failurePayload(t, id, "Exit code "+code, false, 20)), &body); err != nil {
			t.Fatal(err)
		}
		body["tool_input"] = map[string]any{"command": command}
		b, _ := json.Marshal(body)
		e.mustPost(string(b))
	}
	ok := func(id, command, transcript string) {
		post := defaultPost()
		post.ToolUseID = id
		post.ToolInput = map[string]any{"command": command}
		if transcript != "" {
			post.TranscriptPath = transcript
		}
		e.mustPost(post.build(t))
	}

	call("toolu_t1", "pytest -q", "")
	fail("toolu_t1", "pytest -q", "1")
	call("toolu_t2", "pytest -q", "agent-cafe0001")
	ok("toolu_t2", "pytest -q", tlSubTranscript)
	call("toolu_t3", "make build", "")
	fail("toolu_t3", "make build", "2")
	call("toolu_t4", "ls", "")
	e.probe("end", testSession)
}

// A1: both agents on one list, in order, each failure followed up.
func TestTimeline_RendersEveryAgentInOrder(t *testing.T) {
	e := newEnv(t)
	recordTimelineSession(t, e)

	out := e.run("", nil, "report", "--session", testSession, "--timeline").stdout
	for _, want := range []string{
		"timeline: 4 calls, main agent + 1 subagent",
		"failed       2  (1 same command succeeded later, 0 same program succeeded later, 1 no later success)",
		"unknown      1",
		"general-purpose·cafe",
		"→ same command ok at",
		"failed (exit 2)",
		"→ no later success",
		"no execution record",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--timeline is missing %q:\n%s", want, out)
		}
	}
	i1, i2, i3 := strings.Index(out, "Bash pytest"), strings.Index(out, "general-purpose·cafe"), strings.Index(out, "Bash make")
	if i1 < 0 || i2 < 0 || i3 < 0 || !(i1 < i2 && i2 < i3) {
		t.Errorf("rows are not in recording order (main pytest, subagent pytest, main make):\n%s", out)
	}
}

// A2: the JSON carries the same rows, and no digest.
func TestTimeline_JSONMatchesTheText(t *testing.T) {
	e := newEnv(t)
	recordTimelineSession(t, e)

	res := e.run("", nil, "report", "--json", "--session", testSession)
	if res.exitCode != 0 {
		t.Fatalf("report --json: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	var rep struct {
		Sessions []struct {
			Timeline struct {
				Calls []struct {
					ToolUseID string `json:"tool_use_id"`
					Group     string `json:"group"`
					Agent     *struct {
						ID string `json:"id"`
					} `json:"agent"`
					Later *struct {
						Kind string `json:"kind"`
					} `json:"later"`
				} `json:"calls"`
			} `json:"timeline"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &rep); err != nil {
		t.Fatalf("report --json does not parse: %v", err)
	}
	if len(rep.Sessions) != 1 {
		t.Fatalf("sessions = %d, want 1", len(rep.Sessions))
	}
	calls := rep.Sessions[0].Timeline.Calls
	var got []string
	for _, c := range calls {
		got = append(got, c.ToolUseID+":"+c.Group)
	}
	want := "toolu_t1:failed,toolu_t2:ok,toolu_t3:failed,toolu_t4:unknown"
	if strings.Join(got, ",") != want {
		t.Fatalf("timeline = %v, want %s", got, want)
	}
	if calls[0].Later == nil || calls[0].Later.Kind != "same_command" {
		t.Errorf("the failed pytest is not followed up by the subagent's run: %+v", calls[0].Later)
	}
	if calls[1].Agent == nil || calls[1].Agent.ID != "agent-cafe0001" {
		t.Errorf("the subagent's call does not name its agent: %+v", calls[1].Agent)
	}
	if calls[2].Later != nil {
		t.Errorf("the failed make was never re-run, yet carries %+v", calls[2].Later)
	}
	if strings.Contains(res.stdout, `"digest"`) {
		t.Error("report --json carries a shape digest; the timeline compares digests and must not print them")
	}
}

// A3: --timeline works under --redact.
func TestTimeline_SurvivesRedaction(t *testing.T) {
	e := newEnv(t)
	recordTimelineSession(t, e)
	res := e.run("", nil, "report", "--session", testSession, "--timeline", "--redact")
	if res.exitCode != 0 || !strings.Contains(res.stdout, "timeline: 4 calls") {
		t.Fatalf("--timeline --redact: exit %d\n%s\n%s", res.exitCode, res.stdout, res.stderr)
	}
}

// A4: without the flag the text report does not change.
func TestTimeline_DefaultReportIsUnchanged(t *testing.T) {
	e := newEnv(t)
	recordTimelineSession(t, e)
	out := e.run("", nil, "report", "--session", testSession).stdout
	if strings.Contains(out, "timeline:") {
		t.Errorf("the default report renders the timeline:\n%s", out)
	}
}
