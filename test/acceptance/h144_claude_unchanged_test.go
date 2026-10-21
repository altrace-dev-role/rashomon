package acceptance

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// H-144 -- a hook invoked without --harness behaves exactly as it did before
// the flag existed.
//
// Adding a second harness puts a branch in front of every hook path, and the
// risk is that the Claude Code side of the branch drifts: a record gains or
// loses a key, an outcome reads differently, a stand-down speaks where it was
// silent. Every assertion elsewhere checks the fields it names; this one
// checks every byte, against output the binary produced BEFORE the harness
// work began (testdata/h144_claude_unchanged.golden, committed on its own
// ahead of it). The store is pinned -- a fixed install key and id, fixed
// payloads, a cwd and transcript path that are not temp directories -- so
// the only thing normalised is recorded_at_unix_ms, the clock.
//
// Regenerate with -update-h144 only when a change to Claude Code behaviour is
// intended, and say so in that commit.
var updateH144 = flag.Bool("update-h144", false, "rewrite the H-144 golden from the current binary")

const (
	h144Key     = "5f1d3a7c9e0b2468ace13579bdf02468ace13579bdf02468ace13579bdf02468"
	h144Install = "0123456789abcdef0123456789abcdef"
	h144Golden  = "testdata/h144_claude_unchanged.golden"
)

var h144Clock = regexp.MustCompile(`"recorded_at_unix_ms":[0-9]+`)

func TestH144_WithoutTheFlagClaudeCodeOutputIsByteForByteUnchanged(t *testing.T) {
	e := newEnv(t)
	writeFile(t, filepath.Join(e.home, "install.key"), h144Key+"\n")
	writeFile(t, filepath.Join(e.home, "install.json"),
		`{"install_id":"`+h144Install+`","created_at_unix_ms":1700000000000,"schema_version":4}`+"\n")
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	var out strings.Builder
	step := func(name string, res result) {
		fmt.Fprintf(&out, "## %s\nexit %d\nstdout %q\nstderr %q\n", name, res.exitCode, res.stdout, res.stderr)
	}
	ours := []string{"--install", h144Install}
	call := func(name, sub, payload string, extra ...string) {
		step(name, e.run(payload, nil, append(append([]string{sub}, extra...), ours...)...))
	}
	session := func(event string) string {
		return mustJSON(t, map[string]any{
			"hook_event_name": event, "session_id": testSession,
			"transcript_path": "/tmp/transcripts/sess-1.jsonl", "cwd": "/tmp/project",
		})
	}
	pre := func(id, tool string, input map[string]any, mod func(*payloadOpts)) string {
		p := defaultPayload()
		p.ToolUseID, p.ToolName, p.ToolInput = id, tool, input
		if mod != nil {
			mod(&p)
		}
		return p.build(t)
	}
	post := func(id, tool string, input map[string]any, response any) string {
		p := defaultPost()
		p.ToolUseID, p.ToolName, p.ToolInput, p.ToolResponse = id, tool, input, response
		return p.build(t)
	}
	failure := func(id string, input map[string]any, interrupt bool) string {
		return mustJSON(t, map[string]any{
			"hook_event_name": "PostToolUseFailure", "session_id": testSession,
			"transcript_path": "/tmp/transcripts/sess-1.jsonl", "cwd": "/tmp/project",
			"permission_mode": "default", "tool_name": "Bash", "tool_input": input,
			"tool_use_id": id, "error": "Exit code 1\nFAIL\tpkg", "is_interrupt": interrupt,
			"duration_ms": 1200,
		})
	}

	call("probe start", "probe", session("SessionStart"), "start")

	gitStatus := map[string]any{"command": "git status --short", "description": "Show status"}
	call("pre ok", "hook", pre("toolu_1", "Bash", gitStatus, nil))
	call("post ok", "post", post("toolu_1", "Bash", gitStatus, map[string]any{"stdout": " M a\n", "stderr": ""}))

	goTest := map[string]any{"command": "go test ./..."}
	call("pre failing", "hook", pre("toolu_2", "Bash", goTest, nil))
	call("post failure", "post", failure("toolu_2", goTest, false))

	write := map[string]any{"file_path": "/tmp/project/.env", "content": "A=1\n"}
	call("pre write", "hook", pre("toolu_3", "Write", write, nil))
	call("post write", "post", post("toolu_3", "Write", write, map[string]any{"type": "create"}))

	sleep := map[string]any{"command": "sleep 60", "run_in_background": true}
	call("pre background", "hook", pre("toolu_4", "Bash", sleep, nil))
	call("post background", "post", post("toolu_4", "Bash", sleep, map[string]any{"backgroundTaskId": "b1"}))

	ls := map[string]any{"command": "ls -1"}
	call("pre subagent", "hook", pre("toolu_5", "Bash", ls, func(p *payloadOpts) {
		p.AgentID, p.AgentType = "agent-7", "Explore"
		p.TranscriptPath = "/tmp/transcripts/sess-1/subagents/agent-7.jsonl"
	}))
	call("post subagent", "post", post("toolu_5", "Bash", ls, map[string]any{"stdout": "a\n"}))

	makeTest := map[string]any{"command": "make test 2>&1 | tail -40"}
	call("pre interrupted", "hook", pre("toolu_6", "Bash", makeTest, nil))
	call("post interrupted", "post", failure("toolu_6", makeTest, true))

	call("pre fetch", "hook", pre("toolu_7", "WebFetch", map[string]any{"url": "https://example.com/x", "prompt": "p"}, nil))
	call("pre mcp", "hook", pre("toolu_8", "mcp__srv__tool", map[string]any{"q": "x"}, nil))
	call("pre no session", "hook", pre("toolu_9", "Bash", ls, func(p *payloadOpts) { p.SessionID = "" }))

	step("pre other install", e.run(pre("toolu_10", "Bash", ls, nil), nil, "hook", "--install", "ffffffffffffffffffffffffffffffff"))

	call("recap stop", "recap", stopPayload(testSession, "Done. All tests pass.", false))
	call("recap pending", "recap", mustJSON(t, map[string]any{
		"hook_event_name": "UserPromptSubmit", "session_id": testSession, "prompt": "next",
	}), "pending")

	if res := e.pause(); res.exitCode != 0 {
		t.Fatalf("pause: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	call("pre paused", "hook", pre("toolu_11", "Bash", ls, nil))
	if res := e.resume(); res.exitCode != 0 {
		t.Fatalf("resume: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	call("probe end", "probe", session("SessionEnd"), "end")

	runs := filepath.Join(e.home, "runs")
	dirs, err := os.ReadDir(runs)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dirs {
		for _, name := range []string{"records.ndjson", "coverage.ndjson", "spill.ndjson", "seq"} {
			b, err := os.ReadFile(filepath.Join(runs, d.Name(), name))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&out, "## file runs/%s/%s\n%s", d.Name(), name, h144Clock.ReplaceAll(b, []byte(`"recorded_at_unix_ms":0`)))
		}
	}
	got := out.String()

	if *updateH144 {
		writeFile(t, h144Golden, got)
		t.Logf("rewrote %s", h144Golden)
		return
	}
	want, err := os.ReadFile(h144Golden)
	if err != nil {
		t.Fatalf("reading the golden: %v", err)
	}
	if got != string(want) {
		t.Errorf("output differs from %s; first difference:\n%s", h144Golden, firstDifference(string(want), got))
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// firstDifference names the first line at which two outputs part, with the
// line from each, which is what a reader needs to see before reading a diff.
func firstDifference(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := 0; i < len(w) || i < len(g); i++ {
		var wl, gl string
		if i < len(w) {
			wl = w[i]
		}
		if i < len(g) {
			gl = g[i]
		}
		if wl != gl {
			return fmt.Sprintf("line %d\n want %s\n  got %s", i+1, wl, gl)
		}
	}
	return "(none)"
}
