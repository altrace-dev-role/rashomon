package acceptance

import (
	"encoding/json"
	"strings"
	"testing"
)

// H-108 -- a tool payload that names no session writes no execution record,
// and the report says how many arrived.
//
// Per Cursor's documentation -- nobody has run Cursor against this recorder
// -- Cursor loads Claude Code's hooks from ~/.claude/settings.json by
// default and maps eight of Claude Code's events, PostToolUseFailure not
// among them; its payloads name a conversation_id where Claude Code sends
// session_id; and it documents its own postToolUse as "called after
// successful tool execution", beside a postToolUseFailure the mapping omits.
// So a failed Cursor command reaches the post path as PostToolUse or not at
// all. The post path read any sessionless PostToolUse as a Claude Code
// success and wrote outcome ok into the unattributed bucket: false for any
// failure that does arrive that way, and otherwise an outcome resting on an
// ok/failed pairing measured only on Claude Code -- the default
// Execution.Outcome's own comment forbids.
//
// Now a payload with no session id writes NO execution record: never ok,
// and nothing else in its place. Its declaration stays in the unattributed
// bucket, as evidence that something other than a Claude Code session is
// writing here, and the report counts those declarations at read time --
// from the store's unattributed run, with no new record type or field.
//
// Break: drop the guard and a sessionless PostToolUse is recorded as ok.

// cursorToolPayload is a PreToolUse or PostToolUse body shaped the way the
// release spec reads Cursor's hook documentation: conversation_id and
// generation_id where Claude Code sends session_id. Every other field is
// illustrative; what the test depends on is that no session_id key exists.
func cursorToolPayload(t *testing.T, event string) string {
	t.Helper()
	body := map[string]any{
		"conversation_id": "c0ffee00-cursor-conversation",
		"generation_id":   "g-1",
		"hook_event_name": event,
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "go test ./..."},
		"tool_use_id":     "toolu_cursor_1",
		"workspace_roots": []string{"/tmp/project"},
	}
	if event == "PostToolUse" {
		// A failed command, in the one form a Cursor failure could take here
		// if it reaches this path at all: PostToolUse, the only post event in
		// Cursor's documented mapping. The output is never read here -- the
		// post payload declares no field for it -- so nothing in it could
		// have told this program the call failed.
		body["tool_output"] = "FAIL\tgithub.com/example/project\t0.012s"
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// executionsInEveryRun is every execution record the store holds, whichever
// run directory it landed in. "None in the unattributed bucket" is not the
// property; "none anywhere" is.
func (e *env) executionsInEveryRun() []record {
	e.t.Helper()
	var all []record
	for _, dir := range e.runDirs() {
		all = append(all, e.executions(dir)...)
	}
	return all
}

// callsWithoutSessionID reads the report's store-wide count, failing the test
// if the field is missing or not a number: a missing field would otherwise
// read as zero, which is the one confusion this count exists to prevent.
func callsWithoutSessionID(t *testing.T, e *env, args ...string) int {
	t.Helper()
	doc := reportJSON(t, e, nil, args...)
	raw, ok := doc["calls_without_session_id"]
	if !ok {
		t.Fatalf("report --json %v carries no calls_without_session_id", args)
	}
	n, ok := raw.(float64)
	if !ok {
		t.Fatalf("report --json %v: calls_without_session_id = %v, want a number", args, raw)
	}
	return int(n)
}

const unattributedRun = "unattributed"

func TestH108_ACursorShapedCallWritesNoExecutionAndIsCounted(t *testing.T) {
	e := newEnv(t)
	// A Claude Code session on the same machine, so the count is shown to be
	// store-wide and the session beside it untouched.
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))

	for _, event := range []string{"PreToolUse", "PostToolUse"} {
		run := e.hook
		if event == "PostToolUse" {
			run = e.post
		}
		res := run(cursorToolPayload(t, event))
		if res.exitCode != 0 {
			t.Fatalf("%s: exit %d, want 0 -- a recorder never blocks the agent", event, res.exitCode)
		}
		if res.stdout != "" {
			t.Errorf("%s wrote to stdout, which the harness parses as control output: %q", event, res.stdout)
		}
	}

	// No execution record, never ok: not in the unattributed bucket and not
	// anywhere else. The Claude Code session's own execution is the one.
	for _, x := range e.executionsInEveryRun() {
		if x.str("tool_use_id") == "toolu_cursor_1" || x.str("session_id") != testSession {
			t.Errorf("an execution was recorded for a payload that named no session: %s", x.raw)
		}
	}
	if got := len(e.executions(testSession)); got != 1 {
		t.Errorf("sess-1 holds %d execution(s), want its own 1", got)
	}

	// The declaration stays, in the unattributed bucket, as the evidence.
	decls := e.declarations(unattributedRun)
	if len(decls) != 1 || decls[0].str("tool_use_id") != "toolu_cursor_1" {
		t.Fatalf("unattributed declarations = %v, want the Cursor call's one", decls)
	}
	// And the post invocation is not silent: its coverage record says it ran.
	if got := len(e.coverage(unattributedRun, "post")); got != 1 {
		t.Errorf("unattributed post-phase coverage records = %d, want 1: the post invocation "+
			"that wrote no execution must still leave its trace", got)
	}

	// The report counts it, store-wide: in the whole report and in a report
	// scoped to the Claude Code session beside it.
	if got := callsWithoutSessionID(t, e); got != 1 {
		t.Errorf("report: calls_without_session_id = %d, want 1", got)
	}
	if got := callsWithoutSessionID(t, e, "--session", testSession); got != 1 {
		t.Errorf("report --session %s: calls_without_session_id = %d, want 1 (store-wide)", testSession, got)
	}
	text := e.run("", nil, "report")
	if text.exitCode != 0 {
		t.Fatalf("report: exit %d, stderr %q", text.exitCode, text.stderr)
	}
	if !strings.Contains(text.stdout, "1 call arrived without a session id") {
		t.Errorf("the text report does not say a call arrived without a session id:\n%s", text.stdout)
	}

	// The Claude Code session's own report is unaffected.
	if rep := e.report(testSession); rep.Declarations.Recorded != 1 || rep.Executions.Recorded != 1 {
		t.Errorf("sess-1 report: %d declaration(s), %d execution(s), want 1 and 1",
			rep.Declarations.Recorded, rep.Executions.Recorded)
	}

	// And the turn's recap names the reason rather than another session.
	line, ok := e.recapLine(cursorStopPayload())
	if !ok || !strings.Contains(line, "no_session_id") {
		t.Errorf("the Cursor turn's recap printed %q (spoke: %v), want no_session_id", line, ok)
	}
}

// TestH108_ACodexShapedCallIsRecordedOK pins a KNOWN GAP, deliberately.
//
// A Codex-shaped payload, as the release spec describes Codex's hooks --
// nobody has run Codex against this recorder either -- carries a session_id
// and Claude Code's own field names, and nothing in the command line marks
// which harness ran it: the only installer is watch, which writes Claude
// Code's entries, and a Codex hook is wired by hand with no flag of its own.
// So this payload is indistinguishable from a Claude Code PostToolUse, and it
// is recorded exactly like one -- outcome ok. If Codex reports a failed call
// on PostToolUse, that failure lands here as ok, and no field this program
// reads could say otherwise: tool_response is output, and the post payload
// deliberately declares no field for it.
//
// Pinned rather than fixed because the fix is not a guess this program can
// make from the payload. What closes it is an adapter whose installer marks
// the harness in the command line it writes -- the way --install marks the
// install -- so the post path can decline to default to ok for a harness
// whose failure event it has not measured. Until one exists, the README is
// the only place this gap is covered.
//
// When that adapter lands, this test is expected to change: the assertion
// below moves from outcome ok to no execution record, the way H-108's Cursor
// case already reads.
func TestH108_ACodexShapedCallIsRecordedOK(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	const codexSession = "019a2b3c-4d5e-7f60-8a9b-codex0session"
	payload := func(event string) string {
		body := map[string]any{
			"session_id":      codexSession,
			"transcript_path": "/tmp/codex/rollout.jsonl",
			"cwd":             "/tmp/project",
			"hook_event_name": event,
			"tool_name":       "Bash",
			"tool_input":      map[string]any{"command": "go test ./..."},
			"tool_use_id":     "call_codex_1",
			// Keys Claude Code does not send, illustrative of a second
			// harness's additions. Unknown keys, discarded unread.
			"turn_id": "turn-1",
			"model":   "gpt-5-codex",
		}
		if event == "PostToolUse" {
			body["tool_response"] = map[string]any{"exit_code": 1, "output": "FAIL"}
		}
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	// Invoked with no flag at all: the hand-wired shape, which is also how an
	// entry written before --install existed arrives.
	for _, c := range []struct{ sub, event string }{{"hook", "PreToolUse"}, {"post", "PostToolUse"}} {
		if res := e.run(payload(c.event), nil, c.sub); res.exitCode != 0 || res.stdout != "" {
			t.Fatalf("%s: exit %d, stdout %q", c.sub, res.exitCode, res.stdout)
		}
	}

	// THE GAP, pinned: one execution, outcome ok, whatever the call did.
	execs := e.executions(codexSession)
	if len(execs) != 1 {
		t.Fatalf("executions for the Codex session = %d, want 1", len(execs))
	}
	if got := execs[0].str("outcome"); got != "ok" {
		t.Errorf("outcome = %q. If a harness marker now tells this program the payload came from "+
			"Codex, the known gap this test pins is closed: update the test and the README "+
			"together, rather than letting the pin pass for a different reason", got)
	}

	// It is attributed, so it is not counted as sessionless -- and the
	// healthy twin of H-108's line is what the text report says.
	if got := len(e.declarations(unattributedRun)); got != 0 {
		t.Errorf("unattributed declarations = %d, want 0: this payload named its session", got)
	}
	if got := callsWithoutSessionID(t, e); got != 0 {
		t.Errorf("calls_without_session_id = %d, want 0", got)
	}
	text := e.run("", nil, "report")
	if !strings.Contains(text.stdout, "calls without a session id: none") {
		t.Errorf("the text report does not carry the healthy twin:\n%s", text.stdout)
	}
	if strings.Contains(text.stdout, "arrived without a session id") {
		t.Errorf("the text report carries the degraded line with nothing sessionless recorded:\n%s", text.stdout)
	}
}
