package acceptance

// The README's example is a render, not an illustration.
//
// "What a disagreement looks like" says its excerpt is `rashomon report`
// exactly as it renders that session. A sentence like that is true the day it
// is written and false the first time the renderer changes a word, and nothing
// notices: the README is not code. So the session it describes is recorded
// here through the real hooks -- a failed `go test ./...`, a subagent's three
// calls, a closing message saying the tests pass -- rendered by the real
// binary, and compared with the README's block byte for byte. When the render
// changes, this fails, and the README changes with it or the render does not.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readmeExcerptLead is the sentence that introduces the pinned block.
const readmeExcerptLead = "exactly as it renders that session:"

// readmeFinalMessage is the agent's closing message in the README's session.
const readmeFinalMessage = "Done. I refactored ParseConfig and all tests pass."

func TestReadme_TheDisagreementExcerptIsTheRender(t *testing.T) {
	want := readmeExcerpt(t)

	e := newEnv(t)
	e.watched(testSession)

	// The main agent's one call: the test suite, which fails. Its transcript
	// is real, so the account is the agent's own closing words.
	mainTranscript := e.writeFullTranscript(t, "toolu_tests", readmeFinalMessage)
	tests := defaultPayload()
	tests.ToolUseID = "toolu_tests"
	tests.TranscriptPath = mainTranscript
	tests.ToolInput = map[string]any{"command": "go test ./..."}
	e.mustHook(tests.build(t))
	e.mustPost(readmeFailure(t, tests))

	// The subagent's three calls, under its own transcript path. None of them
	// is in the main transcript, which is the line's whole point.
	subTranscript := filepath.Join(filepath.Dir(mainTranscript), testSession, "subagents", "agent-a41f.jsonl")
	for _, c := range []struct {
		id, tool string
		input    map[string]any
	}{
		{"toolu_sub_grep", "Grep", map[string]any{"pattern": "ParseConfig"}},
		{"toolu_sub_read", "Read", map[string]any{"file_path": "/tmp/project/config.go"}},
		{"toolu_sub_log", "Bash", map[string]any{"command": "git log --oneline -5"}},
	} {
		p := defaultPayload()
		p.ToolUseID, p.ToolName, p.ToolInput = c.id, c.tool, c.input
		p.AgentID, p.AgentType = "agent-a41f", "Explore"
		p.TranscriptPath = subTranscript
		e.mustHook(p.build(t))

		post := defaultPost()
		post.ToolUseID, post.ToolName, post.ToolInput = c.id, c.tool, c.input
		post.TranscriptPath = subTranscript
		e.mustPost(post.build(t))
	}
	e.probe("end", testSession)

	res := e.run("", nil, "report", "--session", testSession)
	if res.exitCode != 0 {
		t.Fatalf("report: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if got := renderedExcerpt(t, res.stdout); got != want {
		t.Errorf("the README's excerpt is not what `rashomon report` renders for the session it "+
			"describes. Update README.md \"What a disagreement looks like\" to the render, or "+
			"the render to the README.\nREADME:\n%s\nrender:\n%s", want, got)
	}
}

// readmeFailure is the PostToolUseFailure body for a call, in the shape H-21
// measured: "Exit code N", and no tool_response at all.
func readmeFailure(t *testing.T, p payloadOpts) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"hook_event_name": "PostToolUseFailure",
		"session_id":      p.SessionID,
		"transcript_path": p.TranscriptPath,
		"cwd":             p.CWD,
		"permission_mode": p.PermissionMode,
		"tool_name":       p.ToolName,
		"tool_input":      p.ToolInput,
		"tool_use_id":     p.ToolUseID,
		"error":           "Exit code 1",
		"is_interrupt":    false,
		"duration_ms":     4100,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// readmeExcerpt is the README's pinned block: the indented lines after the
// sentence that introduces it, with the four spaces of Markdown code-block
// indentation removed, so what remains is the report's own indentation.
func readmeExcerpt(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(moduleRoot, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(body), "\n")
	start := -1
	for i, l := range lines {
		if strings.Contains(l, readmeExcerptLead) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		t.Fatalf("README.md no longer says %q, so the excerpt this test pins cannot be found",
			readmeExcerptLead)
	}
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	var block []string
	for _, l := range lines[start:] {
		if !strings.HasPrefix(l, "    ") {
			break
		}
		block = append(block, strings.TrimPrefix(l, "    "))
	}
	if len(block) == 0 {
		t.Fatal("README.md has no indented block after the excerpt's lead sentence")
	}
	return strings.Join(block, "\n") + "\n"
}

// renderedExcerpt is the same span of a text report: from the agent's account
// up to the first line of the destinations section, which the README's block
// stops short of.
func renderedExcerpt(t *testing.T, report string) string {
	t.Helper()
	lines := strings.Split(report, "\n")
	start, end := -1, -1
	for i, l := range lines {
		if start < 0 && strings.HasPrefix(l, "  the agent's account") {
			start = i
		}
		if start >= 0 && strings.HasPrefix(l, "  executed differently from declared:") {
			end = i
			break
		}
	}
	if start < 0 || end < 0 {
		t.Fatalf("the report has no account-to-destinations span:\n%s", report)
	}
	return strings.Join(lines[start:end], "\n") + "\n"
}
