package acceptance

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// MASKED-AT-ACT1's card acceptance, through the binary: a test or build run
// whose line hid its exit status, and whose output does not say how it ended,
// is said at that call's own post, once per gate per prompt, as a fact the
// record states. The line is eng-4's call 3060 (session 02bc13cc, transcript
// line 6568) word for word, its cd pointed at this test's project: a python
// heredoc that edits a Go file, then `gofmt -l`, `go vet ... 2>&1 | head -3`
// and an echo. Seven hours of later Stops repeated that vet's hidden status;
// at the act it was said nowhere.

// maMaskedVet is the note call 3060 raises at its post.
const maMaskedVet = "rashomon record: gate 'go vet' ran in this call and the pipe to head hid its exit status; " +
	"no outcome of this run is recorded."

// maCall3060 is call 3060's command with its cd pointed at dir.
func maCall3060(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/ma1_call3060.txt")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Replace(string(b), "{{dir}}", dir, 1)
}

// maBash runs one Bash call through the binary, hook then PostToolUse, with
// stdout as its output, and returns the post's stdout.
func (e *env) maBash(id, command, stdout string) string {
	e.t.Helper()
	input := map[string]any{"command": command}
	envs := []string{stAgent, "CI="}
	if res := e.hook(stPayload(e.t, "PreToolUse", id, "Bash", input, nil), envs...); res.exitCode != 0 || res.stdout != "" {
		e.t.Fatalf("hook %s: exit %d, stdout %q, stderr %q", id, res.exitCode, res.stdout, res.stderr)
	}
	more := map[string]any{"tool_response": map[string]any{"stdout": stdout, "stderr": "", "interrupted": false}}
	res := e.post(stPayload(e.t, "PostToolUse", id, "Bash", input, more), envs...)
	if res.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	return res.stdout
}

// maText is the text a post's stdout carries to the agent and the person,
// "" for none.
func maText(t *testing.T, stdout string) string {
	t.Helper()
	if stdout == "" {
		return ""
	}
	var out struct {
		SystemMessage string `json:"systemMessage"`
		Hook          struct {
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("post stdout %q is not JSON: %v", stdout, err)
	}
	return out.Hook.Context + out.SystemMessage
}

// TestMA1_TheMaskedExitIsSaidAtTheAct: an edit, then call 3060's line. Its
// post carries the note to the agent and logs one masked-status nudge keyed
// by the go vet gate; the same line again in the same prompt says nothing
// more. Controls, each in its own session: the same vet with its status in
// view, and pytest under tail whose summary the record reads (D229), print
// nothing. Break: drop the post's trigger or the producer, and call 3060's
// post prints nothing.
func TestMA1_TheMaskedExitIsSaidAtTheAct(t *testing.T) {
	t.Run("call 3060's line at its post", func(t *testing.T) {
		e := newSteerEnv(t)
		if res := e.stCall("toolu_e", "Edit", stEdit, 0); res.stdout != "" {
			t.Fatalf("premise: the edit's post printed %q", res.stdout)
		}
		line := maCall3060(t, "/tmp/project")
		stdout := e.maBash("toolu_v", line, "vet is not allowed directly")
		if text := maText(t, stdout); !strings.Contains(text, maMaskedVet) {
			t.Fatalf("call 3060's post printed %q, want the note %q", stdout, maMaskedVet)
		}
		lines := e.steerLines()
		if len(lines) != 1 {
			t.Fatalf("%d steer lines, want 1", len(lines))
		}
		l := lines[0]
		if l.str("pattern") != string(pattern.MaskedStatus) || l.str("event") != "PostToolUse" || l.str("level") != "nudge" ||
			!strings.HasPrefix(l.str("key"), "learned:") {
			t.Errorf("steer line %s, want a masked-status nudge at PostToolUse keyed by the go vet gate", l.raw)
		}
		if again := e.maBash("toolu_v2", line, "vet is not allowed directly"); maText(t, again) != "" {
			t.Errorf("the same line again in the prompt printed %q, want nothing: once per gate per prompt", again)
		}
	})
	t.Run("the same vet with its status in view", func(t *testing.T) {
		e := newSteerEnv(t)
		e.stCall("toolu_e", "Edit", stEdit, 0)
		if out := e.maBash("toolu_v", "cd /tmp/project && go vet ./internal/gateinfer 2>&1", ""); out != "" {
			t.Errorf("an unpiped vet's post printed %q, want nothing", out)
		}
	})
	t.Run("pytest under tail read from its summary", func(t *testing.T) {
		e := newSteerEnv(t)
		e.stCall("toolu_e", "Edit", stEdit, 0)
		if out := e.maBash("toolu_p", "python3 -m pytest -q 2>&1 | tail -15", "....\n4 passed in 0.12s\n"); out != "" {
			t.Errorf("a tailed pytest that passed printed %q, want nothing: its outcome is on the record", out)
		}
	})
}
