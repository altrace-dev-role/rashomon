package hook

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// pipedRun lands one Bash call at PostToolUse (exit 0, as a pipe leaves it)
// whose tool_response carries stdout, and returns its Post.
func (f *stFix) pipedRun(id, line, stdout string) *Post {
	f.t.Helper()
	in := map[string]any{"command": line}
	h := New(f.st, f.clock)
	h.Close(&Signals{}, h.Capture(strings.NewReader(f.payload("PreToolUse", id, "Bash", in, nil))))
	f.now = f.now.Add(time.Minute)
	p := NewPost(f.st, f.clock)
	more := map[string]any{"tool_response": map[string]any{"stdout": stdout, "stderr": ""}}
	if err := p.Capture(strings.NewReader(f.payload("PostToolUse", id, "Bash", in, more))); err != nil {
		f.t.Fatalf("post %s: %v", id, err)
	}
	f.now = f.now.Add(time.Minute)
	return p
}

// pipedAfterGreen lands a green pytest, a write between, then the piped run
// with stdout, and returns what its step prints.
func pipedAfterGreen(t *testing.T, stdout string) []byte {
	t.Helper()
	return pipedAfterGreenEnv(t, stdout, stEnv(0))
}

func pipedAfterGreenEnv(t *testing.T, stdout string, env SteerEnv) []byte {
	t.Helper()
	f := newST(t)
	f.pipedRun("toolu_g", "pytest -q", "3 passed in 0.12s\n")
	f.call("toolu_e", "Edit", map[string]any{"file_path": filepath.Join(f.cwd, "a.py"), "old_string": "a", "new_string": "b"}, 0)
	p := f.pipedRun("toolu_r", "pytest -q | tail -5", stdout)
	return p.Steer(env)
}

// TestPR_APipedRedRunnerStepsAtPostToolUse: a runner whose exit a pipe hid
// fires PostToolUse; its output's red summary is read as a failure's is, so
// regression-location names the write between the green and the red.
func TestPR_APipedRedRunnerStepsAtPostToolUse(t *testing.T) {
	out := string(pipedAfterGreen(t, "FAILED a.py::t - assert 1\n1 failed, 2 passed in 0.12s\n"))
	for _, want := range []string{"gate 'pytest' passed earlier", "the pipe hid the exit status", "'a.py'"} {
		if !strings.Contains(out, want) {
			t.Errorf("the note %q lacks %q", out, want)
		}
	}
}

// TestPR_ANonRedPipedRunStepsNothing: a green summary, a no_summary output
// and an unread one raise nothing.
func TestPR_ANonRedPipedRunStepsNothing(t *testing.T) {
	for name, stdout := range map[string]string{"green": "3 passed in 0.12s\n", "no_summary": "collecting ...\n", "empty_stdout": ""} {
		if out := pipedAfterGreen(t, stdout); len(out) != 0 {
			t.Errorf("%s: printed %q", name, out)
		}
	}
}

// TestPR_APipedRedRunnerKeepsThePostToolUseFloor: with consent the note is
// model-visible, and only on a client version at or over the PostToolUse
// row's floor, 2.1.280 (VERSION-GATE1); below it the user's flag stands alone.
func TestPR_APipedRedRunnerKeepsThePostToolUseFloor(t *testing.T) {
	const red = "1 failed, 2 passed in 0.12s\n"
	at := stEnv(steer.DisclosureVersion)
	if out := string(pipedAfterGreenEnv(t, red, at)); !strings.Contains(out, "hookSpecificOutput") {
		t.Errorf("at the floor the note is not model-visible: %q", out)
	}
	below := stEnv(steer.DisclosureVersion)
	below.AIAgent = "claude-code_2-1-279_harness"
	if out := string(pipedAfterGreenEnv(t, red, below)); strings.Contains(out, "hookSpecificOutput") {
		t.Errorf("below the floor the note is model-visible: %q", out)
	}
}
