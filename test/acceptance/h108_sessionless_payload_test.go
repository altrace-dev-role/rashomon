package acceptance

import (
	"encoding/json"
	"fmt"
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
func cursorToolPayload(t *testing.T, event, toolUseID string) string {
	t.Helper()
	body := map[string]any{
		"conversation_id": cursorConversation,
		"generation_id":   "g-1",
		"hook_event_name": event,
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "go test ./..."},
		"tool_use_id":     toolUseID,
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
		res := run(cursorToolPayload(t, event, "toolu_cursor_1"))
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

// cursorConversation is the conversation every Cursor-shaped fixture here
// belongs to.
const cursorConversation = "c0ffee00-cursor-conversation"

// cursorSessionPayload is a session start or end shaped the way Cursor's
// hook documentation describes one: sessionStart's session_id documented as
// "same as conversation_id", beside the conversation_id every Cursor hook
// receives. The remaining fields are illustrative.
func cursorSessionPayload(t *testing.T, event string) string {
	t.Helper()
	body := map[string]any{
		"conversation_id":     cursorConversation,
		"session_id":          cursorConversation,
		"generation_id":       "g-0",
		"hook_event_name":     event,
		"is_background_agent": false,
		"workspace_roots":     []string{"/tmp/project"},
	}
	if event == "sessionEnd" {
		body["duration_ms"] = 1200
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestH108_ACursorConversationIsNotACleanEmptySession: a conversation whose
// start and end name it, and whose calls do not, must not render as a clean
// session with nothing in it.
//
// Per Cursor's documentation, its sessionStart and sessionEnd carry a
// session_id -- documented as the same value as conversation_id -- while its
// tool, stop and prompt events carry none. Recorded as a session, that start
// and end opened and closed a run no call ever landed in, because the calls
// went to the unattributed run: the report said "coverage: verified" and
// "declarations recorded: 0" for a conversation that made two calls. A clean
// zero over something this program did not watch is the one number it
// exists never to print.
//
// Break: record a start or end that names a conversation, and the
// conversation appears as a verified session with zero calls.
func TestH108_ACursorConversationIsNotACleanEmptySession(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	probe := func(phase, event string) {
		t.Helper()
		res := e.run(cursorSessionPayload(t, event), nil, append([]string{"probe", phase}, e.installArgs()...)...)
		if res.exitCode != 0 || res.stdout != "" {
			t.Fatalf("probe %s: exit %d, stdout %q -- a recorder never blocks the agent", phase, res.exitCode, res.stdout)
		}
	}

	probe("start", "sessionStart")
	for i := 1; i <= 2; i++ {
		id := fmt.Sprintf("toolu_cursor_%d", i)
		e.mustHook(cursorToolPayload(t, "PreToolUse", id))
		e.mustPost(cursorToolPayload(t, "PostToolUse", id))
	}
	probe("end", "sessionEnd")

	for _, dir := range e.runDirs() {
		if dir == cursorConversation {
			t.Errorf("a run exists for conversation %s, which no call was recorded under", cursorConversation)
		}
	}

	// The whole report holds the unattributed calls and nothing else: no
	// session for the conversation, clean or otherwise.
	doc := reportJSON(t, e, nil)
	sessions, _ := doc["sessions"].([]any)
	var names []string
	for _, s := range sessions {
		id, _ := s.(map[string]any)["session_id"].(string)
		names = append(names, id)
	}
	if strings.Join(names, ",") != unattributedRun {
		t.Errorf("sessions = %v, want only %q: the conversation must not render as a session "+
			"of its own", names, unattributedRun)
	}
	if got := callsWithoutSessionID(t, e); got != 2 {
		t.Errorf("calls_without_session_id = %d, want the conversation's 2", got)
	}
	if text := e.run("", nil, "report").stdout; strings.Contains(text, "session "+cursorConversation) {
		t.Errorf("the text report renders the conversation as a session:\n%s", text)
	}
}

// TestH108_AClaudeCodeSessionStillRecordsItsStartAndEnd pins the assumption
// the rule above rests on: Claude Code sends no conversation_id today.
//
// The payloads are Claude Code's SessionStart and SessionEnd with the fields
// its hooks documentation (code.claude.com/docs/en/hooks) lists for them --
// documented, not captured -- and they must still record the start and the
// end. This cannot notice Claude Code changing on its own; nothing here runs
// Claude Code. What it does is make the assumption a fixture: a rule that
// grows past conversation_id goes red here, and if Claude Code ever
// documents a conversation_id on these events, adding it to this fixture
// turns this red -- the moment to replace the rule with a harness marker in
// the installed command line, the flag the Codex gap above needs too.
func TestH108_AClaudeCodeSessionStillRecordsItsStartAndEnd(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	const id = "0a7b1c2d-3e4f-4a5b-8c6d-claudecode01"
	common := func(event string) map[string]any {
		return map[string]any{
			"session_id":      id,
			"transcript_path": "/tmp/transcripts/" + id + ".jsonl",
			"cwd":             e.cwd,
			"scratchpad_dir":  "/tmp/scratchpad",
			"permission_mode": "default",
			"hook_event_name": event,
		}
	}
	start := common("SessionStart")
	start["source"] = "startup"
	start["model"] = "claude-sonnet-4-5"
	end := common("SessionEnd")
	end["reason"] = "prompt_input_exit"

	for _, c := range []struct {
		phase string
		body  map[string]any
	}{{"start", start}, {"end", end}} {
		b, err := json.Marshal(c.body)
		if err != nil {
			t.Fatal(err)
		}
		if res := e.run(string(b), nil, append([]string{"probe", c.phase}, e.installArgs()...)...); res.exitCode != 0 {
			t.Fatalf("probe %s: exit %d", c.phase, res.exitCode)
		}
		if got := len(e.coverage(id, c.phase)); got != 1 {
			t.Errorf("%s-phase coverage records for a Claude Code session = %d, want 1", c.phase, got)
		}
	}
	if rep := e.report(id); !rep.Coverage.StartRecorded || !rep.Coverage.EndRecorded {
		t.Errorf("start recorded = %v, end recorded = %v, want both: Claude Code's own session "+
			"start and end must still count", rep.Coverage.StartRecorded, rep.Coverage.EndRecorded)
	}
}

// TestH108_ASessionlessFailureEventRecordsNoExecutionEither: the guard covers
// PostToolUseFailure too. No documented harness is known to send this event
// without a session id -- Cursor's documented mapping omits it -- but the
// guard is about the ok/failed pairing, measured only on Claude Code, and
// the failure half of it is no more measured without a session id than the
// ok half. Pinned because the natural edit ("keep failures; a failure can
// never be a false success") records an outcome this path has no
// measurement for, into a run no session owns.
//
// Break: exempt the failure event from the guard, and a failed execution is
// recorded in the unattributed run.
func TestH108_ASessionlessFailureEventRecordsNoExecutionEither(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	b, err := json.Marshal(map[string]any{
		"hook_event_name": "PostToolUseFailure",
		"cwd":             "/tmp/project",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "go test ./..."},
		"tool_use_id":     "toolu_sessionless_failure",
		"error":           "Exit code 1",
		"is_interrupt":    false,
		"duration_ms":     30,
	})
	if err != nil {
		t.Fatal(err)
	}

	res := e.post(string(b))
	if res.exitCode != 0 {
		t.Fatalf("post: exit %d, want 0 -- a recorder never blocks the agent", res.exitCode)
	}
	if res.stdout != "" {
		t.Errorf("post wrote to stdout, which the harness parses as control output: %q", res.stdout)
	}
	if got := e.executionsInEveryRun(); len(got) != 0 {
		t.Errorf("a failure event that named no session recorded %d execution(s): %s", len(got), got[0].raw)
	}
	if got := len(e.coverage(unattributedRun, "post")); got != 1 {
		t.Errorf("unattributed post-phase coverage records = %d, want 1: the invocation still "+
			"leaves its trace", got)
	}
}
