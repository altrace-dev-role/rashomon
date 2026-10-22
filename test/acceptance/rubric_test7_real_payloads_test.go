package acceptance

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Claude Code payloads and transcript every other test here builds by hand
// were written from what the authors believed Claude Code sends. These are what
// it sent: one live session of Claude Code 2.1.280 (claude -p, model
// claude-haiku-4-5, 2026-10-13), run from an empty sandbox directory with
// `--setting-sources project` so no user hook or plugin loaded, and with a
// --settings file whose every rashomon event (install.Events) ran a script
// that wrote its stdin, unmodified, to a file. The prompt asked for ten steps:
// Write, Read, Edit, `ls -la`, `cat does-not-exist.txt` (fails), `rm -f
// out.txt` (refused by a --disallowedTools rule), Grep, Glob, WebFetch of
// https://example.com/ (which the model reached through ToolSearch, WebFetch
// being a deferred tool), and `cd .. && ls wd > /dev/null && echo done`.
//
// payloads.ndjson holds the 25 payloads in the order they fired, one per line
// (each arrived as one line and a newline). transcript.jsonl is the session's
// transcript. Redaction rewrote the home directory and the sandbox path, and
// the user name in the `ls -la` output; it dropped ten attachment lines that
// carry the account's context rather than the session's (session_context,
// prompt_snapshot, skill_listing, agent_listing_delta, deferred_tools_delta,
// mcp_instructions_delta, remote_session_change). Four kinds of value were
// then replaced with stable fakes, because they name the account's requests
// to Anthropic and nothing here reads them for what they are: every requestId
// (req_fixture01 to req_fixture12, distinct where the originals were, which
// is all spend asks of them), every message id and tool-use id (msg_fixtureNN
// and toolu_fixtureNN, same rule -- every reader here matches one of these
// against another in the same pair of files, never against its own text),
// and every thinking block's opaque signature (fixture-signature-01 to -12);
// the thinking text itself was empty.
// TestRubricTEST7_FixtureHoldsNoLiveRequestIDsOrSignatures holds both files
// to those forms. Nothing else was changed: session and prompt ids,
// timestamps, usage and example.com are as Claude Code wrote them.
const (
	realFixtureDir     = "../fixtures/claude-code/2.1.280"
	realSession        = "3cb822fe-8dcf-48d0-b9ca-725c8eed47f9"
	realTranscriptPath = "/Users/user/.claude/projects/-private-tmp-rashomon-fixture-wd/" + realSession + ".jsonl"
)

type realPayload struct {
	raw       string
	event     string
	tool      string
	toolUseID string
}

// realPayloads reads payloads.ndjson, pointing each payload's transcript_path
// at transcript, the fixture's copy in this test's machine.
func realPayloads(t *testing.T, transcript string) []realPayload {
	t.Helper()
	f, err := os.Open(filepath.Join(realFixtureDir, "payloads.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	quoted, err := json.Marshal(transcript)
	if err != nil {
		t.Fatal(err)
	}
	var out []realPayload
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		raw := sc.Text()
		if strings.Count(raw, realTranscriptPath) != 1 {
			t.Fatalf("premise: a payload does not name the fixture transcript once: %s", raw)
		}
		raw = strings.Replace(raw, `"`+realTranscriptPath+`"`, string(quoted), 1)
		var head struct {
			Event     string `json:"hook_event_name"`
			Tool      string `json:"tool_name"`
			ToolUseID string `json:"tool_use_id"`
		}
		if err := json.Unmarshal([]byte(raw), &head); err != nil {
			t.Fatalf("payload is not JSON: %v", err)
		}
		out = append(out, realPayload{raw: raw, event: head.Event, tool: head.Tool, toolUseID: head.ToolUseID})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(out) != 25 {
		t.Fatalf("premise: %d payloads, want the 25 captured", len(out))
	}
	return out
}

// replayReal installs, then feeds every captured payload to the subcommand
// watch installs for its event, in the order Claude Code fired them.
func (e *env) replayReal(t *testing.T) []realPayload {
	t.Helper()
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	raw, err := os.ReadFile(filepath.Join(realFixtureDir, "transcript.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(e.configDir, "projects", "-private-tmp-rashomon-fixture-wd", realSession+".jsonl")
	if err := os.MkdirAll(filepath.Dir(transcript), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcript, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	payloads := realPayloads(t, transcript)
	sub := map[string][]string{
		"SessionStart": {"probe", "start"}, "SessionEnd": {"probe", "end"},
		"PreToolUse": {"hook"}, "PostToolUse": {"post"}, "PostToolUseFailure": {"post"},
		"Stop": {"recap"}, "UserPromptSubmit": {"recap", "pending"},
	}
	for _, p := range payloads {
		args, ok := sub[p.event]
		if !ok {
			t.Fatalf("no subcommand for captured event %q", p.event)
		}
		res := e.run(p.raw+"\n", nil, append(append([]string{}, args...), e.installArgs()...)...)
		if res.exitCode != 0 {
			t.Fatalf("%s (%s %s): exit %d, stderr %q", p.event, p.tool, p.toolUseID, res.exitCode, res.stderr)
		}
	}
	return payloads
}

// TestRubricTEST7_RealSessionReplays replays the captured session through the
// binary, as watch installs it, and holds the store and the report to what
// Claude Code sent: every PreToolUse a declaration, every PostToolUse and
// PostToolUseFailure an execution, the transcript's ids all accounted for,
// the refused rm a denial rather than a gap, and the agent's own account the
// final message the Stop payload carried.
func TestRubricTEST7_RealSessionReplays(t *testing.T) {
	e := newEnv(t)
	payloads := e.replayReal(t)

	declared, executed := map[string]string{}, map[string]bool{}
	var stop string
	for _, p := range payloads {
		switch p.event {
		case "PreToolUse":
			declared[p.toolUseID] = p.tool
		case "PostToolUse", "PostToolUseFailure":
			executed[p.toolUseID] = true
		case "Stop":
			var s struct {
				Message string `json:"last_assistant_message"`
			}
			if err := json.Unmarshal([]byte(p.raw), &s); err != nil {
				t.Fatal(err)
			}
			stop = s.Message
		}
	}
	if len(declared) != 11 || len(executed) != 10 || stop == "" {
		t.Fatalf("premise: %d declared, %d executed, final message %q", len(declared), len(executed), stop)
	}

	decls := e.declarations(realSession)
	if len(decls) != len(declared) {
		t.Errorf("%d declarations, want one per PreToolUse (%d)", len(decls), len(declared))
	}
	var denied string
	for _, d := range decls {
		id := d.str("tool_use_id")
		if declared[id] != d.str("tool_name") {
			t.Errorf("declaration %s names %q, the payload %q", id, d.str("tool_name"), declared[id])
		}
		if !executed[id] {
			denied = id
		}
		if d.str("tool_name") == "WebFetch" {
			if hosts, _ := d.fields["hosts"].([]any); len(hosts) != 1 || hosts[0] != "example.com" {
				t.Errorf("the WebFetch of https://example.com/ declares hosts %v", d.fields["hosts"])
			}
		}
	}
	failed := 0
	for _, x := range e.executions(realSession) {
		if !executed[x.str("tool_use_id")] {
			t.Errorf("execution %s answers no PostToolUse", x.str("tool_use_id"))
		}
		if x.str("outcome") != "ok" {
			failed++
			if code, _ := x.fields["exit_code"].(float64); x.str("outcome") != "failed" || code != 1 {
				t.Errorf("the failing cat reads %s, exit %v; want failed, exit 1", x.str("outcome"), x.fields["exit_code"])
			}
		}
	}
	if failed != 1 {
		t.Errorf("%d executions did not succeed, want the one cat of a missing file", failed)
	}

	rep := e.report(realSession)
	if rep.Coverage.State != "verified" || len(rep.Coverage.Reasons) != 0 {
		t.Errorf("coverage %s %v, want verified with no reasons", rep.Coverage.State, rep.Coverage.Reasons)
	}
	if len(rep.Transcripts) != 1 {
		t.Fatalf("%d transcripts in the report, want 1", len(rep.Transcripts))
	}
	tr := rep.Transcripts[0]
	if !tr.Readable || tr.IDsInTranscript == nil || *tr.IDsInTranscript != len(declared) {
		t.Errorf("transcript readable=%v ids=%v, want readable with %d ids", tr.Readable, tr.IDsInTranscript, len(declared))
	}
	for name, list := range map[string][]string{
		"missing_from_store": tr.MissingFromStore, "missing_from_transcript": tr.MissingFromTranscript,
		"executed_but_unrecorded": tr.ExecutedButUnrecorded, "refused_before_hooks": tr.RefusedBeforeHooks,
		"declared_without_result": tr.DeclaredWithoutResult,
	} {
		if len(list) != 0 {
			t.Errorf("%s is %v on a session every hook recorded", name, list)
		}
	}
	if len(tr.DeniedByUser) != 1 || tr.DeniedByUser[0] != denied {
		t.Errorf("denied_by_user %v, want the refused rm %s", tr.DeniedByUser, denied)
	}
	if got := e.reportAccount(realSession); got != stop {
		t.Errorf("the report's account is %q, want the final message the Stop payload carried, %q", got, stop)
	}
}

// reportAccount is the account block's text from report --json.
func (e *env) reportAccount(sessionID string) string {
	e.t.Helper()
	res := e.run("", nil, "report", "--json", "--session", sessionID)
	var rep struct {
		Sessions []struct {
			Account struct {
				Text string `json:"text"`
			} `json:"account"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &rep); err != nil || len(rep.Sessions) != 1 {
		e.t.Fatalf("report --json: %v\n%s", err, res.stdout)
	}
	return rep.Sessions[0].Account.Text
}

// TestRubricTEST7_RealSessionDigests drives digest from the captured session:
// the store the replay left, and the Stop payload Claude Code sent, piped in
// whole as a Stop hook would pipe it. The turn's declarations and executions
// are the payloads', the refused rm is the one declaration without an
// execution, the failing cat is the one failure, and the final message is
// read from the real payload's own shape.
//
// Whether silent_failures fires is not pinned. On this session it does not,
// and for a reason worth knowing: the final message never mentions the cat
// that failed, but it says the rm was "denied", which is in the failure
// vocabulary, and the vocabulary is matched across the whole message.
func TestRubricTEST7_RealSessionDigests(t *testing.T) {
	e := newEnv(t)
	payloads := e.replayReal(t)
	var stopRaw, denied string
	executed := map[string]bool{}
	for _, p := range payloads {
		switch p.event {
		case "Stop":
			stopRaw = p.raw
		case "PostToolUse", "PostToolUseFailure":
			executed[p.toolUseID] = true
		}
	}
	for _, p := range payloads {
		if p.event == "PreToolUse" && !executed[p.toolUseID] {
			denied = p.toolUseID
		}
	}
	if stopRaw == "" || denied == "" {
		t.Fatal("premise: the capture holds a Stop payload and one refused call")
	}
	res := e.run(stopRaw+"\n", nil, "digest", "--session", realSession, "--stdin")
	if res.exitCode != 0 {
		t.Fatalf("digest --stdin: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	var d digestOutput
	if err := json.Unmarshal([]byte(res.stdout), &d); err != nil {
		t.Fatalf("digest output is not JSON: %v\n%s", err, res.stdout)
	}
	if d.SessionID != realSession || d.Declarations.Recorded != 11 || d.Executions.Recorded != 10 {
		t.Errorf("digest of %s: %d declarations, %d executions; want 11 and 10", d.SessionID, d.Declarations.Recorded, d.Executions.Recorded)
	}
	if w := d.Declarations.WithoutExecution; len(w) != 1 || w[0].ToolUseID != denied {
		t.Errorf("without_execution %+v, want the refused rm %s", w, denied)
	}
	if !d.SilentFailures.FinalMessageAvailable || d.SilentFailures.Failed != 1 {
		t.Errorf("silent_failures %+v: want the Stop payload's final message read and the one failing cat", d.SilentFailures)
	}
}
