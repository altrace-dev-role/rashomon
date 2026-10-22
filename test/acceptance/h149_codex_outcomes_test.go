package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// H-149 -- at report time, a Codex execution takes its outcome from the
// rollout's item_completed entry with exactly its id, and from nothing else.
//
// Codex's hook payloads carry no exit code, so every Codex execution is
// recorded with none (H-145). The rollout does: phase 4 measured each
// command's item_completed entry carrying the record's tool_use_id as item.id,
// with an integer exit_code and a status. The rule, decided by Rishi for
// Karthik's review: a command with exit 0 is ok and any other integer is
// failed; a file change whose status is completed is ok, and failed is
// failed; anything else --
// no matching id, a missing field, a value not named -- stays unobserved.
//
// The first test replays phase 4's eight calls through the real hook entry
// points against the real rollout lines (test/fixtures/codex/phase4, scrubbed).
// The payloads are rebuilt from the phase 4 records and rollouts, because
// phase 4 captured none; the ids, turns, tools and commands are phase 4's. The
// second holds every way an item must fail to judge a call.

// codexCall is one call to replay: its ids and what the hook was told.
type codexCall struct {
	session, turn, id, tool string
	input                   map[string]any
	agent                   string
	rollout                 string
}

const (
	p4Session1 = "01a16455-3641-7002-aab1-1c986d458c00"
	p4Session2 = "01a1645b-3105-7602-b4ad-5472f0b11986"
	p4Sub      = "01a1645c-c8a7-74d1-a2ec-afab1d819e47"

	p4FirstRg = "exec-44c82ba1-bc33-4ddf-bd8c-abd9532c65f0"
	p4Cat     = "exec-85a4b155-8925-490f-8d65-eab5b0c354cb"
	p4Patch   = "exec-c3c93eab-a32c-4889-8753-629533297ef6"
	p4Calc    = "exec-c618d9e6-771b-41f8-a206-1f4ca03aa061"
	p4Missing = "exec-242a1f82-a32e-49d2-953b-3ea1ac9973a7"
	p4Spawn   = "call_7a00c6f64c264a828ff5a7b34e8b388f"
	p4Wait    = "call_66196d3cda8846b393e290b57caefc57"
	p4SubRg   = "exec-18c5eb84-012b-48ef-b819-3a22aad50b03"
)

var phase4Calls = []codexCall{
	{p4Session1, "01a16457-b68e-7c32-a8db-6b1253c7036d", p4FirstRg, "Bash",
		map[string]any{"command": "pwd && rg --files -g 'calc.py' -g 'AGENTS.md'"}, "", p4Session1},
	{p4Session1, "01a16457-b68e-7c32-a8db-6b1253c7036d", p4Cat, "Bash",
		map[string]any{"command": "cat calc.py && find .. -name AGENTS.md -print"}, "", p4Session1},
	// What apply_patch's tool_input holds was not captured; the recorder
	// labels no file for it either way.
	{p4Session1, "01a16457-b68e-7c32-a8db-6b1253c7036d", p4Patch, "apply_patch",
		map[string]any{"command": "[not captured]"}, "", p4Session1},
	{p4Session2, "01a1645b-577f-74c3-875e-a0bc6b7551a3", p4Calc, "Bash",
		map[string]any{"command": "python3 calc.py"}, "", p4Session2},
	{p4Session2, "01a1645c-1f03-7453-9e64-c84df3239413", p4Missing, "Bash",
		map[string]any{"command": "python3 missing.py"}, "", p4Session2},
	{p4Session2, "01a1645c-c08a-7192-960b-c1eefff78566", p4Spawn, "collaborationspawn_agent",
		map[string]any{"task_name": "list_files", "fork_turns": "all", "message": "[encrypted]"}, "", p4Session2},
	{p4Session2, "01a1645c-c08a-7192-960b-c1eefff78566", p4Wait, "collaborationwait_agent",
		map[string]any{"timeout_ms": 30000}, "", p4Session2},
	// The subagent's own turn; the recorder files it under its parent's.
	{p4Session2, "01a1645c-c8b6-7b52-be3e-8b66bcb2bb57", p4SubRg, "Bash",
		map[string]any{"command": "rg --files -g '*'"}, p4Sub, p4Sub},
}

// replayCodex runs each call's PreToolUse and PostToolUse through the hook
// entry points, after one SessionStart per session. rollouts maps a rollout
// name to the path the payloads give as transcript_path.
func replayCodex(t *testing.T, e *env, calls []codexCall, rollouts map[string]string) {
	t.Helper()
	codex := append([]string{"--harness", "codex"}, e.installArgs()...)
	run := func(event string, sub []string, payload map[string]any) {
		t.Helper()
		payload["hook_event_name"] = event
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		res := e.run(string(b), nil, append(sub, codex...)...)
		if res.exitCode != 0 || res.stdout != "" || res.stderr != "" {
			t.Fatalf("%s: exit %d, stdout %q, stderr %q", event, res.exitCode, res.stdout, res.stderr)
		}
	}
	started := map[string]bool{}
	for _, c := range calls {
		base := func() map[string]any {
			p := map[string]any{
				"session_id": c.session, "turn_id": c.turn, "cwd": "/home/user/rashomon-phase4/work",
				"model": "gpt-6-luna", "permission_mode": "default", "transcript_path": rollouts[c.rollout],
			}
			if c.agent != "" {
				p["agent_id"], p["agent_type"] = c.agent, "default"
			}
			return p
		}
		if !started[c.session] {
			started[c.session] = true
			p := base()
			p["source"] = "startup"
			delete(p, "turn_id")
			run("SessionStart", []string{"probe", "start"}, p)
		}
		pre := base()
		pre["tool_name"], pre["tool_use_id"], pre["tool_input"] = c.tool, c.id, c.input
		run("PreToolUse", []string{"hook"}, pre)
		post := base()
		post["tool_name"], post["tool_use_id"], post["tool_input"] = c.tool, c.id, c.input
		post["tool_response"] = "[output not kept]"
		run("PostToolUse", []string{"post"}, post)
	}
}

// unobservedLink is how a chain link names an execution with no outcome.
const unobservedLink = "outcome unobserved"

// codexReport is as much of a report as H-149 asserts on.
type codexReport struct {
	Coverage struct {
		Reasons []string `json:"reasons"`
	} `json:"coverage"`
	SilentFailures struct {
		Failed     int `json:"failed"`
		Unobserved int `json:"outcome_unobserved"`
	} `json:"silent_failures"`
	Transcripts []struct {
		MissingFromStore      []string `json:"missing_from_store"`
		MissingFromTranscript []string `json:"missing_from_transcript"`
		ExecutedButUnrecorded []string `json:"executed_but_unrecorded"`
	} `json:"transcripts"`
	Chains struct {
		Prompts []struct {
			Links []struct {
				ToolUseID string `json:"tool_use_id"`
				Outcome   string `json:"outcome"`
			} `json:"links"`
		} `json:"prompts"`
	} `json:"chains"`
}

func (r codexReport) outcomes() map[string]string {
	out := map[string]string{}
	for _, p := range r.Chains.Prompts {
		for _, l := range p.Links {
			out[l.ToolUseID] = l.Outcome
		}
	}
	return out
}

func codexReportOf(t *testing.T, e *env, session string) codexReport {
	t.Helper()
	var rep struct {
		Sessions []codexReport `json:"sessions"`
	}
	res := e.run("", nil, "report", "--json", "--session", session)
	if err := json.Unmarshal([]byte(res.stdout), &rep); err != nil || len(rep.Sessions) != 1 {
		t.Fatalf("report: exit %d, %v\n%s", res.exitCode, err, res.stdout)
	}
	return rep.Sessions[0]
}

func TestH149_Phase4RunsTakeTheirOutcomesFromTheRollout(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	rollouts := map[string]string{}
	for _, s := range []string{p4Session1, p4Session2, p4Sub} {
		name := "rollout-" + s + ".jsonl"
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, mustRead(t, filepath.Join(moduleRoot, "test", "fixtures", "codex", "phase4", name)), 0o600); err != nil {
			t.Fatal(err)
		}
		rollouts[s] = path
	}
	replayCodex(t, e, phase4Calls, rollouts)

	want := map[string]string{
		p4FirstRg: "failed", // exit 2: rg could not read directories under ~
		p4Cat:     "ok",
		p4Patch:   "ok", // a file change that completed
		p4Calc:    "ok",
		p4Missing: "failed",       // exit 2
		p4Spawn:   unobservedLink, // not a command or a file change
		p4Wait:    unobservedLink,
		p4SubRg:   "ok",
	}
	s1, s2 := codexReportOf(t, e, p4Session1), codexReportOf(t, e, p4Session2)
	got := s1.outcomes()
	for id, o := range s2.outcomes() {
		got[id] = o
	}
	for id, w := range want {
		if got[id] != w {
			t.Errorf("%s: outcome %q, want %q", id, got[id], w)
		}
	}

	// The missing.py session: 1 failure, and the two agent tools unobserved.
	if s2.SilentFailures.Failed != 1 || s2.SilentFailures.Unobserved != 2 {
		t.Errorf("session 2: %d failed, %d unobserved; want 1 and 2", s2.SilentFailures.Failed, s2.SilentFailures.Unobserved)
	}
	if text := e.run("", nil, "report", "--session", p4Session2).stdout; !strings.Contains(text, "\n  failed calls: 1\n") {
		t.Errorf("session 2 text report does not say 1 failed call:\n%s", text)
	}
	// The exit code is the item's own number, and the timeline names it.
	if text := e.run("", nil, "report", "--timeline", "--session", p4Session2).stdout; !strings.Contains(text, "failed (exit 2)") {
		t.Errorf("session 2 timeline does not name exit 2:\n%s", text)
	}
	// The first session's rg failed, which the hooks could not tell, and the
	// rest of it, the patch included, is judged.
	if s1.SilentFailures.Failed != 1 || s1.SilentFailures.Unobserved != 0 {
		t.Errorf("session 1: %d failed, %d unobserved; want 1 and 0", s1.SilentFailures.Failed, s1.SilentFailures.Unobserved)
	}

	// The rollout and the store agree, both sessions, all three rollouts.
	for name, s := range map[string]codexReport{"session 1": s1, "session 2": s2} {
		for _, r := range s.Coverage.Reasons {
			if r == "transcript_mismatch" || r == "execution_mismatch" {
				t.Errorf("%s carries %s", name, r)
			}
		}
		for _, tr := range s.Transcripts {
			if len(tr.MissingFromStore)+len(tr.MissingFromTranscript)+len(tr.ExecutedButUnrecorded) != 0 {
				t.Errorf("%s: transcript accounting disagrees: %+v", name, tr)
			}
		}
	}
	if len(s1.Transcripts) != 1 || len(s2.Transcripts) != 2 {
		t.Errorf("%d and %d rollouts accounted; want 1 and 2", len(s1.Transcripts), len(s2.Transcripts))
	}

	// The records keep what the hooks could observe: nothing.
	for _, s := range []string{p4Session1, p4Session2} {
		for _, x := range e.executions(s) {
			if x.str("outcome") != "" || x.fields["exit_code"] != nil {
				t.Errorf("%s: the stored record was given an outcome", x.str("tool_use_id"))
			}
		}
	}
}

func TestH149_AnItemThatDoesNotSayStaysUnobserved(t *testing.T) {
	const session = "01a16455-0000-7000-8000-000000000001"
	ids := []string{
		"exec-string-code", "exec-float-code", "exec-no-code", "exec-null-code",
		"exec-unknown-type", "exec-patch-done", "exec-patch-odd", "exec-elsewhere",
		"exec-conflict", "exec-no-item", "exec-patch-failed",
	}
	item := func(typ, id, extra string) string {
		return `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"` + typ + `","id":"` + id + `"` + extra + `}}}` + "\n"
	}
	body := item("CommandExecution", "exec-string-code", `,"exit_code":"1","status":"failed"`) +
		item("CommandExecution", "exec-float-code", `,"exit_code":1.0,"status":"failed"`) +
		item("CommandExecution", "exec-no-code", `,"status":"failed"`) +
		item("CommandExecution", "exec-null-code", `,"exit_code":null,"status":"failed"`) +
		item("Mystery", "exec-unknown-type", `,"exit_code":1,"status":"failed"`) +
		item("FileChange", "exec-patch-done", `,"status":"completed"`) +
		item("FileChange", "exec-patch-odd", `,"status":"Failed"`) +
		item("CommandExecution", "exec-conflict", `,"exit_code":0`) +
		item("CommandExecution", "exec-conflict", `,"exit_code":1`) +
		// A near miss: one character off a recorded id.
		item("CommandExecution", "exec-no-item-", `,"exit_code":1`) +
		item("FileChange", "exec-patch-failed", `,"status":"failed"`) +
		`{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"exec-elsewhere"` +
		"\n" + // not one line of JSON: skipped
		`not json at all` + "\n"
	dir := t.TempDir()
	main := filepath.Join(dir, "rollout-main.jsonl")
	other := filepath.Join(dir, "rollout-other.jsonl")
	// exec-elsewhere completes, failed, in a rollout its declaration does not
	// name: a match by id in the wrong file is not a match.
	if err := os.WriteFile(main, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte(item("CommandExecution", "exec-elsewhere", `,"exit_code":1`)), 0o600); err != nil {
		t.Fatal(err)
	}

	e := newEnv(t)
	var calls []codexCall
	for _, id := range ids {
		tool := "Bash"
		if strings.HasPrefix(id, "exec-patch") {
			tool = "apply_patch"
		}
		calls = append(calls, codexCall{session, "01a16457-0000-7000-8000-000000000001", id, tool,
			map[string]any{"command": "true"}, "", "main"})
	}
	calls = append(calls, codexCall{session, "01a16457-0000-7000-8000-000000000001", "exec-other-file", "Bash",
		map[string]any{"command": "true"}, "", "other"})
	replayCodex(t, e, calls, map[string]string{"main": main, "other": other})

	rep := codexReportOf(t, e, session)
	got := rep.outcomes()
	for _, id := range ids {
		want := unobservedLink
		switch id {
		case "exec-patch-failed":
			want = "failed"
		case "exec-patch-done":
			want = "ok"
		}
		if got[id] != want {
			t.Errorf("%s: outcome %q, want %q", id, got[id], want)
		}
	}
	if rep.SilentFailures.Failed != 1 {
		t.Errorf("%d failed; want 1, the failed file change", rep.SilentFailures.Failed)
	}

	// The near miss is a command the store does not hold, and exec-no-item
	// one the rollout does not: both sides of the check, by exact id.
	var fromStore, fromTranscript []string
	for _, tr := range rep.Transcripts {
		fromStore = append(fromStore, tr.MissingFromStore...)
		fromTranscript = append(fromTranscript, tr.MissingFromTranscript...)
	}
	if strings.Join(fromStore, ",") != "exec-no-item-" {
		t.Errorf("missing from store: %v; want the near miss alone", fromStore)
	}
	for _, id := range []string{"exec-no-item", "exec-elsewhere"} {
		if !strings.Contains(","+strings.Join(fromTranscript, ",")+",", ","+id+",") {
			t.Errorf("missing from transcript %v does not name %s", fromTranscript, id)
		}
	}
	mismatch := false
	for _, r := range rep.Coverage.Reasons {
		mismatch = mismatch || r == "transcript_mismatch"
	}
	if !mismatch {
		t.Errorf("a rollout that disagrees with the store raised no transcript_mismatch: %v", rep.Coverage.Reasons)
	}
}
