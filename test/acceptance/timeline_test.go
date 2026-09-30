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
// one more call -- naming a host -- is declared with no execution record, one
// failure is posted for a call that was never declared, and one call is
// denied at the permission prompt, which only the transcript says.

const tlSubTranscript = "/tmp/transcripts/sess-1.jsonl/subagents/agent-cafe0001.jsonl"

// tlHost is named by the undeclared-outcome call. The timeline never prints a
// host, and under --redact no section may.
const tlHost = "build.internal.example"

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
	call("toolu_t4", "curl -s https://"+tlHost+"/status", "")
	// A PostToolUseFailure with no PreToolUse before it: an execution record
	// and nothing else. It is a call the session made, and it failed.
	fail("toolu_t5", "pytest -q", "3")
	// Denied: a declaration, no execution record, and a transcript whose
	// tool_result says the user refused it. No post hook: it never ran.
	denied := defaultPayload()
	denied.ToolUseID = "toolu_t6"
	denied.ToolInput = map[string]any{"command": "rm -rf build"}
	denied.TranscriptPath = writeResultTranscript(t, e, denied.ToolUseID, deniedText, true)
	e.mustHook(denied.build(t))
	e.probe("end", testSession)
}

// A1: both agents on one list, in order, each failure followed up, and the
// undeclared failure on the list and in the count the report's own
// failed-calls line agrees with.
func TestTimeline_RendersEveryAgentInOrder(t *testing.T) {
	e := newEnv(t)
	recordTimelineSession(t, e)

	out := e.run("", nil, "report", "--session", testSession, "--timeline").stdout
	for _, want := range []string{
		"timeline: 6 calls (4 main agent, 1 from 1 subagent, 1 agent unknown)",
		"failed       3  (1 same command ok, recorded after; 0 same program ok, recorded after; 1 no later success recorded, 1 not checked)",
		"never ran    1",
		"unknown      1",
		"denied before running",
		"1 call with no declaration recorded",
		"→ same command ok at",
		"failed (exit 2)",
		"→ no later success recorded",
		"no execution record",
		"failed (exit 3), no declaration recorded",
		"failed calls: 3",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("--timeline is missing %q:\n%s", want, out)
		}
	}
	// Rows found by their columns, not by a substring: the agent label also
	// appears in the arrow on the failed row it followed up, so a bare Index
	// would find the arrow first and the check would pass on any order.
	row := func(agent, call string) int {
		for i, line := range strings.Split(out, "\n") {
			f := strings.Fields(line)
			if len(f) >= 5 && f[2] == agent && f[3]+" "+f[4] == call {
				return i
			}
		}
		return -1
	}
	i1, i2, i3 := row("main", "Bash pytest"), row("general-purpose·cafe", "Bash pytest"), row("main", "Bash make")
	if i1 < 0 || i2 < 0 || i3 < 0 || !(i1 < i2 && i2 < i3) {
		t.Errorf("rows are not in recording order (main pytest %d, subagent pytest %d, main make %d):\n%s", i1, i2, i3, out)
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
					AgentUnknown bool `json:"agent_unknown"`
					Later        *struct {
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
	want := "toolu_t1:failed,toolu_t2:ok,toolu_t3:failed,toolu_t4:unknown,toolu_t6:never_ran,toolu_t5:failed"
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
	if calls[5].Agent != nil || !calls[5].AgentUnknown {
		t.Errorf("the undeclared call names an agent the record does not: %+v", calls[5])
	}
	if strings.Contains(res.stdout, `"digest"`) {
		t.Error("report --json carries a shape digest; the timeline compares digests and must not print them")
	}
}

// A3: --timeline works under --redact, and the host a call named is printed
// nowhere in it.
func TestTimeline_SurvivesRedaction(t *testing.T) {
	e := newEnv(t)
	recordTimelineSession(t, e)
	res := e.run("", nil, "report", "--session", testSession, "--timeline", "--redact")
	if res.exitCode != 0 || !strings.Contains(res.stdout, "timeline: 6 calls") {
		t.Fatalf("--timeline --redact: exit %d\n%s\n%s", res.exitCode, res.stdout, res.stderr)
	}
	if strings.Contains(res.stdout, tlHost) {
		t.Errorf("--timeline --redact prints the host %q:\n%s", tlHost, res.stdout)
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
