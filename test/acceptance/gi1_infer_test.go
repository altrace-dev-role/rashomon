package acceptance

// GI1: with no spec test gate, the Stop judges the project's own test
// command, read from its CI first and its manifests last (card GATE-INFER1,
// R46 as W1-W4, W2', W3'). The line names the command by its runner's row,
// labelled "(inferred)" until the project's index has seen a test run, in
// every session, within 80 columns.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gi1Project is a git project with files and no spec, in a directory of its
// own.
func gi1Project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// gi1Session runs one session of the project in dir under session id: its
// SessionStart probe, an edit, then each command as a green Bash call, then
// the Stop, whose final message claims nothing; it returns the Stop line.
func (e *env) gi1Session(dir, session string, commands ...string) string {
	e.t.Helper()
	if res := e.probe("start", session); res.exitCode != 0 {
		e.t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	call := func(id, tool string, input map[string]any) {
		envs := []string{stAgent, "CI="}
		extra := map[string]any{"cwd": dir, "session_id": session}
		if res := e.hook(stPayload(e.t, "PreToolUse", id, tool, input, extra), envs...); res.exitCode != 0 || res.stdout != "" {
			e.t.Fatalf("hook %s: exit %d, stdout %q, stderr %q", id, res.exitCode, res.stdout, res.stderr)
		}
		extra["tool_response"] = map[string]any{"stdout": "ok\n", "stderr": "", "interrupted": false}
		if res := e.post(stPayload(e.t, "PostToolUse", id, tool, input, extra), envs...); res.exitCode != 0 {
			e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
		}
	}
	call("toolu_e", "Edit", dghEdit(dir))
	for i, c := range commands {
		call("toolu_g"+string(rune('0'+i)), "Bash", map[string]any{"command": c})
	}
	b, err := json.Marshal(map[string]any{"hook_event_name": "Stop", "session_id": session, "cwd": dir,
		"transcript_path": "/tmp/transcripts/" + session + ".jsonl", "stop_hook_active": false,
		"last_assistant_message": "I changed internal/a.go."})
	if err != nil {
		e.t.Fatal(err)
	}
	line, _ := e.recapLine(string(b), stAgent, "CI=")
	e.t.Logf("Stop line (%s): %q", session, line)
	gb1Columns(e.t, line)
	return line
}

const gi1Workflow = "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - uses: actions/checkout@v4\n" +
	"      - run: pytest -q\n"

// TestGI1a_AFirstSessionNamesTheProjectsOwnTestCommand: a first session
// with no spec, an edit and no test run names the inferred command by its
// runner, the CI's before a manifest's; a run of its runner with other
// arguments binds it. Control: a project with nothing to infer keeps the
// no-gate line. Break: leave the Stop's inference unwired, or read the
// manifest before the CI.
func TestGI1a_AFirstSessionNamesTheProjectsOwnTestCommand(t *testing.T) {
	for _, c := range []struct {
		name     string
		files    map[string]string
		commands []string
		want     string
	}{
		{"go.mod, no run", map[string]string{"go.mod": "module m\n"}, nil, "go test (inferred) not run after the last edit."},
		{"CI before go.mod", map[string]string{"go.mod": "module m\n", ".github/workflows/ci.yml": gi1Workflow}, nil,
			"pytest (inferred) not run after the last edit."},
		{"go.mod, a run with other arguments", map[string]string{"go.mod": "module m\n"}, []string{"go test -count=1 ./..."},
			"verified - go test (inferred) ran with other arguments and passed."},
		{"control: nothing to infer", map[string]string{"main.go": "package main\n"}, nil,
			"no test or build is known for this project yet."},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newSteerEnv(t)
			got := e.gi1Session(gi1Project(t, c.files), "sess-gi1-a", c.commands...)
			if !strings.Contains(got, c.want) {
				t.Errorf("Stop line %q, want it to contain %q", got, c.want)
			}
		})
	}
}

// TestGI1a2_TheNextSessionStillNamesItWithoutTheLabel: after a session whose
// green run taught the index a test family, the next session's edit with no
// run still names the command, now without "(inferred)" (W3': promotion
// changes the label, never whether the gate stands). Break: stop inferring
// once the index holds a test family, and the second Stop says no test or
// build applies.
func TestGI1a2_TheNextSessionStillNamesItWithoutTheLabel(t *testing.T) {
	e := newSteerEnv(t)
	dir := gi1Project(t, map[string]string{"go.mod": "module m\n"})
	e.gi1Session(dir, "sess-gi1-one", "go test ./...")
	got := e.gi1Session(dir, "sess-gi1-two")
	if !strings.Contains(got, "go test not run after the last edit.") || strings.Contains(got, "(inferred)") {
		t.Errorf("second Stop line %q, want the command named without the label", got)
	}
}
