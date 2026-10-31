package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/fault"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// The Stop view through the binary (card 2.4 R12, R15): a watched session,
// calls replayed as Claude Code sends them, and the recap entry run on a
// Stop, StopFailure or UserPromptSubmit payload. The fixtures carry verified
// post coverage (card 2.3 A2), which only a watched machine has.

// stopEvent is a Stop-family payload for sess-1: the event, the final
// message, and cwd when it is not empty. stop_hook_active is the JSON
// boolean Claude Code sends.
func stopEvent(event, msg, cwd string) string {
	m := map[string]any{
		"hook_event_name":        event,
		"session_id":             testSession,
		"transcript_path":        "/tmp/transcripts/" + testSession + ".jsonl",
		"stop_hook_active":       false,
		"last_assistant_message": msg,
	}
	if cwd != "" {
		m["cwd"] = cwd
	}
	b, err := json.Marshal(m)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// pendingEvent is the UserPromptSubmit catch-up's payload, with cwd.
func pendingEvent(cwd string) string {
	return fmt.Sprintf(`{"session_id":%q,"hook_event_name":"UserPromptSubmit","prompt":"next","cwd":%q}`, testSession, cwd)
}

// turnCall replays one call of sess-1 in prompt, as replayCall does.
func (e *env) turnCall(t *testing.T, prompt, id string, c fixtureCall) {
	t.Helper()
	pre := defaultPayload()
	pre.PromptID, pre.ToolUseID, pre.ToolName, pre.ToolInput = prompt, id, c.Tool, c.Input
	e.mustHook(pre.build(t))
	switch c.Outcome {
	case "ok":
		post := defaultPost()
		post.ToolUseID, post.ToolName, post.ToolInput = id, c.Tool, c.Input
		post.ToolResponse = map[string]any{"stdout": c.Output, "stderr": "", "interrupted": false}
		if c.Tool != "Bash" {
			post.ToolResponse = map[string]any{"output": c.Output}
		}
		e.mustPost(post.build(t))
	case "failed":
		b, err := json.Marshal(map[string]any{
			"hook_event_name": "PostToolUseFailure", "session_id": testSession, "transcript_path": "/tmp/transcripts/sess-1.jsonl",
			"cwd": "/tmp/project", "permission_mode": "default", "tool_name": c.Tool, "tool_input": c.Input, "tool_use_id": id,
			"error": fmt.Sprintf("Exit code %d", max(c.ExitCode, 1)), "is_interrupt": false, "duration_ms": 30,
		})
		if err != nil {
			t.Fatal(err)
		}
		e.mustPost(string(b))
	}
}

func editOf(path string) fixtureCall {
	return fixtureCall{Tool: "Edit", Input: map[string]any{"file_path": "/tmp/project/" + path, "old_string": "a", "new_string": "b"}, Outcome: "ok"}
}

func goTestRun(outcome string) fixtureCall {
	c := fixtureCall{Tool: "Bash", Input: map[string]any{"command": "go test ./..."}, Outcome: outcome}
	if outcome == "ok" {
		c.Output = "ok  \texample.com/p\t0.01s\n"
	} else {
		c.ExitCode = 1
	}
	return c
}

// greenTurn is prompt's edit of main.go and then a passing go test: the
// verified row.
func (e *env) greenTurn(t *testing.T, prompt string) {
	t.Helper()
	e.turnCall(t, prompt, "toolu_"+prompt+"_edit", editOf("main.go"))
	e.turnCall(t, prompt, "toolu_"+prompt+"_test", goTestRun("ok"))
}

// verifiedLine is greenTurn's value line for sess-1, settings install.
const verifiedLine = "※ rashomon: verified - go test passed after the last edit.\n" +
	"            → rashomon report --session " + testSession

// checkpoints is sess-1's verify records.
func (e *env) checkpoints() []record {
	e.t.Helper()
	return recordsOfType(e.records(testSession), "verify")
}

// recordSeq is a record's seq, false when it is null or absent.
func recordSeq(r record) (int64, bool) {
	f, ok := r.fields["seq"].(float64)
	return int64(f), ok
}

// indexFile is the project index path for the stop's cwd under this store.
func (e *env) indexFile(cwd string) string {
	return verify.IndexPath(e.home, baseline.Key(cwd))
}

func (e *env) mustSpeak(payload string, extraEnv ...string) string {
	e.t.Helper()
	line, ok := e.recapLine(payload, extraEnv...)
	if !ok {
		e.t.Fatalf("recap printed nothing for %s", payload)
	}
	return line
}

// TestPG8b_CheckpointIsAppendedUnder8KiB: a Stop appends one verify record:
// a seq of its own, under 8 KiB, point stop, point_seq the run's last seq,
// the turn's prompt, and the final message's claim verdicts, which the next
// prompt reads (card 2.4 R3, R7, R9, D90; PF-PG8).
func TestPG8b_CheckpointIsAppendedUnder8KiB(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.greenTurn(t, "prompt-1")
	e.mustSpeak(stopEvent("Stop", "All 42 tests pass.", e.cwd))

	var last int64
	for _, r := range e.records(testSession) {
		if s, ok := recordSeq(r); ok && r.typ() != "verify" {
			last = max(last, s)
		}
	}
	cps := e.checkpoints()
	if len(cps) != 1 {
		t.Fatalf("%d verify records, want 1", len(cps))
	}
	cp := cps[0]
	if n := len(cp.raw) + 1; n > verify.CheckpointMaxBytes {
		t.Errorf("checkpoint line is %d bytes, want at most %d", n, verify.CheckpointMaxBytes)
	}
	seq, ok := recordSeq(cp)
	if !ok || seq <= last {
		t.Errorf("checkpoint seq %v (set %v), want one past the run's last, %d", seq, ok, last)
	}
	if ps, _ := cp.fields["point_seq"].(float64); cp.str("point") != verify.PointStop || int64(ps) != last {
		t.Errorf("point %q at %v, want %q at %d", cp.str("point"), cp.fields["point_seq"], verify.PointStop, last)
	}
	if cp.str("prompt_id") != "prompt-1" || cp.str("session_id") != testSession {
		t.Errorf("prompt %q session %q, want prompt-1 and %s", cp.str("prompt_id"), cp.str("session_id"), testSession)
	}
	if cp.fields["schema_version"] != float64(5) || cp.fields["critical"] != false {
		t.Errorf("schema_version %v critical %v, want 5 and false", cp.fields["schema_version"], cp.fields["critical"])
	}
	claims, _ := cp.fields["claims"].([]any)
	if len(claims) != 1 {
		t.Fatalf("checkpoint claims %v, want the message's one claim", cp.fields["claims"])
	}
	if c, _ := claims[0].(map[string]any); c["class"] != "tests_pass" || c["strict"] != true || c["verdict"] != "green" {
		t.Errorf("checkpoint claim %v, want a strict tests_pass with verdict green", claims[0])
	}
}

// TestR2b_RecapNeverPrintsStopContinue: whatever prints, the JSON on stdout
// has one key, systemMessage, and never one that steers the turn (card 2.4
// R12, R2b): on a value line, and on a red gate under a pass claim.
func TestR2b_RecapNeverPrintsStopContinue(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	keys := func(stdout string) []string {
		var m map[string]any
		if err := json.Unmarshal([]byte(stdout), &m); err != nil {
			t.Fatalf("stdout %q is not one JSON object: %v", stdout, err)
		}
		var ks []string
		for k := range m {
			ks = append(ks, k)
		}
		slices.Sort(ks)
		return ks
	}

	e.greenTurn(t, "prompt-1")
	res := e.recap(stopEvent("Stop", "Updated main.go.", e.cwd))
	if res.stdout == "" {
		t.Fatal("the verified turn printed nothing, want its value line")
	}
	if ks := keys(res.stdout); !slices.Equal(ks, []string{"systemMessage"}) {
		t.Errorf("keys %q, want only systemMessage", ks)
	}

	e.turnCall(t, "prompt-2", "toolu_p2_edit", editOf("util.go"))
	e.turnCall(t, "prompt-2", "toolu_p2_test", goTestRun("failed"))
	res = e.recap(stopEvent("Stop", "All 42 tests pass.", e.cwd))
	if res.stdout == "" {
		t.Fatal("the red turn under a pass claim printed nothing")
	}
	if ks := keys(res.stdout); !slices.Equal(ks, []string{"systemMessage"}) {
		t.Errorf("keys %q on the red turn, want only systemMessage", ks)
	}
}

// TestFUN14_RecapSentenceWinsOverTheValueLine: a turn the value line would
// speak for prints it; a turn with a recap sentence prints that sentence and
// no value line (tech 2.2.11 condition 2; card 2.4 R12).
func TestFUN14_RecapSentenceWinsOverTheValueLine(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.greenTurn(t, "prompt-1")
	if line := e.mustSpeak(stopEvent("Stop", "Updated main.go.", e.cwd)); line != verifiedLine {
		t.Fatalf("the verified turn printed %q, want %q", line, verifiedLine)
	}

	e.turnCall(t, "prompt-2", "toolu_p2_edit", editOf("util.go"))
	e.turnCall(t, "prompt-2", "toolu_p2_test", goTestRun("ok"))
	e.turnCall(t, "prompt-2", "toolu_p2_false", fixtureCall{Tool: "Bash", Input: map[string]any{"command": "false"}, Outcome: "failed"})
	line := e.mustSpeak(stopEvent("Stop", "Updated util.go.", e.cwd))
	if !strings.Contains(line, "1 recorded failure") || strings.Contains(line, "verified") {
		t.Errorf("the turn with a failed call printed %q, want its recorded failure and no value line", line)
	}
}

// TestFUN14b_DoneFlagWinsOverTheValueLine: a strict pass claim over a gate
// that did not run after the last edit prints the done flag, with the value
// sentence it stands on, in the value line's place (card 2.4 R7, R12).
func TestFUN14b_DoneFlagWinsOverTheValueLine(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.turnCall(t, "prompt-1", "toolu_test", goTestRun("ok"))
	e.turnCall(t, "prompt-1", "toolu_edit_1", editOf("main.go"))
	e.turnCall(t, "prompt-1", "toolu_edit_2", editOf("util.go"))
	want := "※ rashomon: done was claimed; the record shows 2 files edited, no test or build run after the last edit.\n" +
		"            → rashomon report --session " + testSession
	if line := e.mustSpeak(stopEvent("Stop", "All 42 tests pass.", e.cwd)); line != want {
		t.Errorf("printed %q, want the done flag %q", line, want)
	}
}

// TestFUN16_SummaryIsOnWithNoConfig: with no config file the summary switch
// is on and a verified turn prints its value line (D71).
func TestFUN16_SummaryIsOnWithNoConfig(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	if _, err := os.Stat(filepath.Join(e.home, "config.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("premise: config.json is present or unreadable: %v", err)
	}
	e.greenTurn(t, "prompt-1")
	if line, ok := e.recapLine(stopEvent("Stop", "Updated main.go.", e.cwd)); line != verifiedLine {
		t.Errorf("printed %q (%v), want %q", line, ok, verifiedLine)
	}
}

// TestSTOP_IndexIsWrittenUnderTheProjectKey: a Stop with cwd observes the
// turn's gate runs into the index under baseline.Key(cwd) (card 2.4 R10).
func TestSTOP_IndexIsWrittenUnderTheProjectKey(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.greenTurn(t, "prompt-1")
	e.mustSpeak(stopEvent("Stop", "Updated main.go.", e.cwd))
	idx, err := verify.ReadIndex(e.indexFile(e.cwd))
	if err != nil || idx == nil {
		t.Fatalf("index at %s: %v, %v; want one", e.indexFile(e.cwd), idx, err)
	}
	if idx.ProjectKey != baseline.Key(e.cwd) || len(idx.Entries) != 1 {
		t.Fatalf("index %+v, want one entry under %q", idx, baseline.Key(e.cwd))
	}
	if en := idx.Entries[0]; en.Sessions != 1 || en.LastSession != testSession || en.Kind != "test" {
		t.Errorf("entry %+v, want one session, %s, kind test", en, testSession)
	}
}

// TestSTOP_NoCwdNoIndex: a Stop without cwd still appends its checkpoint, and
// reads and writes no index (card 2.4 R10).
func TestSTOP_NoCwdNoIndex(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.greenTurn(t, "prompt-1")
	e.recapLine(stopEvent("Stop", "Updated main.go.", ""))
	if n := len(e.checkpoints()); n != 1 {
		t.Fatalf("%d verify records, want 1: the Stop view ran", n)
	}
	if _, err := os.Stat(filepath.Join(e.home, "verify")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s exists (%v), want no index written without cwd", filepath.Join(e.home, "verify"), err)
	}
}

// TestSTOP_PendingWrapsTheValueLine: a turn no Stop reached prints its value
// line at the next prompt, every line marked as the previous turn's,
// AutoSteer clause included (card 2.4 R1, R8, R12).
func TestSTOP_PendingWrapsTheValueLine(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.greenTurn(t, "prompt-1")
	note, err := json.Marshal(map[string]any{"type": "steer", "state": "delivered", "prompt_id": "prompt-1",
		"event": "PostToolUseFailure", "pattern": "regression-location", "level": "nudge", "model_text": "go test failed at call 3"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.home, "runs", testSession, "steer.ndjson"), append(note, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	ind := strings.Repeat(" ", len([]rune("※ rashomon, previous turn: ")))
	want := "※ rashomon, previous turn: verified - go test passed after the last edit.\n" +
		ind + "AutoSteer: 1 regression location given at the failure.\n" +
		ind + "→ rashomon report --session " + testSession
	if line, ok := e.recapPending(testSession, "next"); line != want {
		t.Errorf("the catch-up printed %q (%v), want %q", line, ok, want)
	}
}

// TestSTOP_SecondStopForThePromptPrintsNothing: the second Stop for a prompt
// prints nothing (recap.Claim's dedupe, D77) and appends a second checkpoint
// at the same point (D82).
func TestSTOP_SecondStopForThePromptPrintsNothing(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.greenTurn(t, "prompt-1")
	if line := e.mustSpeak(stopEvent("Stop", "Updated main.go.", e.cwd)); line != verifiedLine {
		t.Fatalf("the first Stop printed %q, want %q", line, verifiedLine)
	}
	if line, ok := e.recapLine(stopEvent("Stop", "Updated main.go.", e.cwd)); ok {
		t.Errorf("the second Stop printed %q, want nothing", line)
	}
	cps := e.checkpoints()
	if len(cps) != 2 || cps[0].fields["point_seq"] != cps[1].fields["point_seq"] {
		t.Errorf("%d verify records, want 2 at one point_seq", len(cps))
	}
}

// TestSTOP_FlushFailurePrintsNothing: a failed Flush before stdout prints
// nothing (REC-34, fail closed) and leaves the prompt unclaimed, so the next
// Stop for it prints (D93); the same turn without the fault prints.
func TestSTOP_FlushFailurePrintsNothing(t *testing.T) {
	failing := "RASHOMON_FAULT=" + fault.PointRecapFlush + ":fail=1"
	for _, c := range []struct {
		env  []string
		want string
	}{
		{nil, verifiedLine},
		{[]string{failing}, ""},
	} {
		e := newEnv(t)
		e.watched(testSession)
		e.greenTurn(t, "prompt-1")
		if line, _ := e.recapLine(stopEvent("Stop", "Updated main.go.", e.cwd), c.env...); line != c.want {
			t.Errorf("env %q: printed %q, want %q", c.env, line, c.want)
		}
		if c.env == nil {
			continue
		}
		if line, _ := e.recapLine(stopEvent("Stop", "Updated main.go.", e.cwd)); line != verifiedLine {
			t.Errorf("the Stop after a failed Flush printed %q, want %q: the failed one claimed the prompt", line, verifiedLine)
		}
	}
}

// TestSTOP_EmptyRunWritesNoCheckpoint: a Stop over a run with no seq has no
// point, so it appends nothing; the first recorded call gives the next Stop
// one (card 2.4 R3).
func TestSTOP_EmptyRunWritesNoCheckpoint(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	if line, ok := e.recapLine(stopEvent("Stop", "Hello.", e.cwd)); ok {
		t.Errorf("the empty turn printed %q, want nothing", line)
	}
	if n := len(e.checkpoints()); n != 0 {
		t.Fatalf("%d verify records over an empty run, want 0", n)
	}
	e.greenTurn(t, "prompt-1")
	e.recapLine(stopEvent("Stop", "Updated main.go.", e.cwd))
	if n := len(e.checkpoints()); n != 1 {
		t.Errorf("%d verify records after a recorded turn, want 1", n)
	}
}

// TestSTOP_PendingAppendsNoCheckpoint: the catch-up prints the value line and
// appends no checkpoint and observes no index (card 2.4 R12).
func TestSTOP_PendingAppendsNoCheckpoint(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.greenTurn(t, "prompt-1")
	res := e.run(pendingEvent(e.cwd), nil, append([]string{"recap", "pending"}, e.installArgs()...)...)
	if res.exitCode != 0 || res.stdout == "" {
		t.Fatalf("recap pending: exit %d, stdout %q; want the previous turn's value line", res.exitCode, res.stdout)
	}
	if n := len(e.checkpoints()); n != 0 {
		t.Errorf("%d verify records after the catch-up, want 0", n)
	}
	if _, err := os.Stat(e.indexFile(e.cwd)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("index at %s (%v), want none observed by the catch-up", e.indexFile(e.cwd), err)
	}
}

// TestSTOP_StopFailureWritesNoLine: StopFailure writes the checkpoint and the
// index and prints no value line (tech 2.2.11 condition 4; card 2.4 R12).
func TestSTOP_StopFailureWritesNoLine(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.greenTurn(t, "prompt-1")
	if line, ok := e.recapLine(stopEvent("StopFailure", "Updated main.go.", e.cwd)); ok {
		t.Errorf("StopFailure printed %q, want nothing", line)
	}
	if n := len(e.checkpoints()); n != 1 {
		t.Errorf("%d verify records after StopFailure, want 1", n)
	}
	if idx, err := verify.ReadIndex(e.indexFile(e.cwd)); err != nil || idx == nil {
		t.Errorf("index after StopFailure: %v, %v; want one", idx, err)
	}
}
