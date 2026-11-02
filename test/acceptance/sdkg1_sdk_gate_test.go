package acceptance

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// SDK-GATE1 (B309): the full screen says Agent SDK programs are recorded
// "only if you answer y to the second question below", and nothing read the
// answer: watch wrote it to harnesses.json and every hook path recorded an
// SDK program's calls after an n all the same. The person was asked and said
// n, so the hook paths now stand down, writing nothing at all, for a call an
// Agent SDK program made unless harnesses.json holds the y. No answer on file
// (an install from before the question, or the plugin route, which never asks
// it) is not a y. Only a positive detection closes the gate: the TypeScript
// and Python SDKs' entrypoints. Claude Code sets sdk-cli for every
// non-interactive run, claude -p included, which is Claude Code, not a
// program built on the SDK, so it records as before.

// sdkgSession runs one whole session as entrypoint, with extra in its
// environment: start, a prompt, an edit, the turn's end and the session's
// end, each exiting 0.
func (e *env) sdkgSession(entrypoint string, extra ...string) {
	e.t.Helper()
	ep := append([]string{"CLAUDE_CODE_ENTRYPOINT=" + entrypoint}, extra...)
	for _, step := range []struct {
		payload string
		args    []string
	}{
		{e.sessionPayload("start", testSession), []string{"probe", "start"}},
		{fmt.Sprintf(`{"session_id":%q,"hook_event_name":"UserPromptSubmit","prompt":"fix a.go","cwd":"/tmp/project"}`, testSession), []string{"recap", "pending"}},
	} {
		if res := e.run(step.payload, ep, append(step.args, e.installArgs()...)...); res.exitCode != 0 {
			e.t.Fatalf("%v as %s: exit %d, stderr %q", step.args, entrypoint, res.exitCode, res.stderr)
		}
	}
	e.stCall("toolu_e", "Edit", stEdit, 0, ep...)
	stop := fmt.Sprintf(`{"session_id":%q,"hook_event_name":"Stop","cwd":"/tmp/project","transcript_path":"/tmp/none.jsonl"}`, testSession)
	if res := e.recap(stop, ep...); res.exitCode != 0 {
		e.t.Fatalf("recap as %s: exit %d, stderr %q", entrypoint, res.exitCode, res.stderr)
	}
	if res := e.probe("end", testSession, ep...); res.exitCode != 0 {
		e.t.Fatalf("probe end as %s: exit %d, stderr %q", entrypoint, res.exitCode, res.stderr)
	}
}

// sdkgAnswer leaves the store as watch --yes does and then as answer says:
// "n" keeps the "paused" an n writes, "" removes harnesses.json (no answer on
// file), "y" writes the y.
func (e *env) sdkgAnswer(answer string) {
	e.t.Helper()
	const onFile = `{"surfaces":{"sdk":{"record":"on","steer":"off"}},"version":1}` + "\n"
	if res := e.watch(); res.exitCode != 0 {
		e.t.Fatalf("watch --yes: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	harnesses := filepath.Join(e.home, "harnesses.json")
	switch answer {
	case "n":
		if got := e.sdkRecording(); got != "paused" {
			e.t.Fatalf("premise: watch --yes left the Agent SDK answer %q, want paused", got)
		}
	case "":
		if err := os.Remove(harnesses); err != nil {
			e.t.Fatal(err)
		}
	case "y":
		if err := os.WriteFile(harnesses, []byte(onFile), 0o600); err != nil {
			e.t.Fatal(err)
		}
	}
}

// declared is whether the session's records hold the declaration of id.
func (e *env) declared(id string) bool {
	e.t.Helper()
	for _, d := range e.declarations(testSession) {
		if d.str("tool_use_id") == id {
			return true
		}
	}
	return false
}

// TestSDKG1_AnAgentSDKProgramIsRecordedOnlyAfterAY: on the watch route, a
// whole session from the TypeScript or the Python SDK changes nothing on the
// machine after an n (watch --yes leaves the same "paused" an n does) or with
// no answer on file, and is recorded after a y. Controls: Claude Code's
// terminal and claude -p record after an n. Break: drop the gate and the n
// rows record; read no answer as y and the no-answer row records; close the
// gate on any entrypoint, or on sdk-cli, and a control records nothing.
func TestSDKG1_AnAgentSDKProgramIsRecordedOnlyAfterAY(t *testing.T) {
	for _, tc := range []struct {
		name, answer, entrypoint string
		recorded                 bool
	}{
		{"n, the TypeScript SDK", "n", "sdk-ts", false},
		{"n, the Python SDK", "n", "sdk-py", false},
		{"no answer on file, the TypeScript SDK", "", "sdk-ts", false},
		{"y, the TypeScript SDK", "y", "sdk-ts", true},
		{"n, Claude Code's terminal", "n", "cli", true},
		{"n, claude -p", "n", "sdk-cli", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.sdkgAnswer(tc.answer)
			before := e.machine()
			e.sdkgSession(tc.entrypoint)
			if !tc.recorded {
				e.requireUnchanged(before, fmt.Sprintf("a session as %s after answer %q", tc.entrypoint, tc.answer))
				return
			}
			if !e.declared("toolu_e") {
				t.Errorf("a session as %s after answer %q recorded no declaration of its edit", tc.entrypoint, tc.answer)
			}
		})
	}
}

// TestSDKG1_ThePluginRouteRecordsNoAgentSDKProgram: the plugin route asks no
// Agent SDK question and writes no answer, so its breadth is off: a session
// from the TypeScript SDK with the plugin enabled changes nothing on the
// machine, while the same session from Claude Code's terminal records its
// edit. Break: drop the gate, or read no answer as y, and the SDK session
// records.
func TestSDKG1_ThePluginRouteRecordsNoAgentSDKProgram(t *testing.T) {
	for _, tc := range []struct {
		entrypoint string
		recorded   bool
	}{{"sdk-ts", false}, {"cli", true}} {
		t.Run(tc.entrypoint, func(t *testing.T) {
			e, fx := pcEnv(t)
			run := func(stdin string, args ...string) {
				t.Helper()
				cmd := exec.Command(fx.bin, args...)
				cmd.Stdin = strings.NewReader(stdin)
				cmd.Env = e.environ("CI=", stNow304, "CLAUDE_CODE_ENTRYPOINT="+tc.entrypoint)
				cmd.Dir = e.cwd
				if res := e.wait(cmd); res.exitCode != 0 {
					t.Fatalf("%v via the plugin binary as %s: exit %d, stderr %q", args, tc.entrypoint, res.exitCode, res.stderr)
				}
			}
			before := e.machine()
			run(e.sessionPayload("start", testSession), "probe", "start")
			run(stPayload(t, "PreToolUse", "toolu_e", "Edit", stEdit, nil), "hook")
			run(stPayload(t, "PostToolUse", "toolu_e", "Edit", stEdit,
				map[string]any{"tool_response": map[string]any{"stdout": "", "stderr": "", "interrupted": false}}), "post")
			if !tc.recorded {
				e.requireUnchanged(before, "a plugin session as "+tc.entrypoint)
				return
			}
			if !e.declared("toolu_e") {
				t.Errorf("a plugin session as %s recorded no declaration of its edit", tc.entrypoint)
			}
		})
	}
}
