package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// turnLine is one line of a main transcript for FinalAssistantTexts.
func turnLine(t *testing.T, fields map[string]any) string {
	t.Helper()
	if _, ok := fields["timestamp"]; !ok {
		fields["timestamp"] = "2026-09-29T10:00:00.000Z"
	}
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func said(t *testing.T, text, at string) string {
	return turnLine(t, map[string]any{"type": "assistant", "timestamp": at,
		"message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": text}}}})
}

// TestFinalAssistantTexts_ATurnIsItsPrompt holds the attribution rules on the
// shapes a real 2.1.285 transcript has: assistant lines belong to the prompt
// of the user line before them; a tool_result with no promptId (seen once in
// a real transcript) and an injected meta line stay inside the turn; a
// subagent's sidechain line is not the main agent's word; and a typed
// prompt with no promptId ends the turn, so its reply is nobody's.
func TestFinalAssistantTexts_ATurnIsItsPrompt(t *testing.T) {
	user := func(prompt string, content any, meta bool) string {
		f := map[string]any{"type": "user", "isSidechain": false,
			"message": map[string]any{"role": "user", "content": content}}
		if prompt != "" {
			f["promptId"] = prompt
		}
		if meta {
			f["isMeta"] = true
		}
		return turnLine(t, f)
	}
	result := []map[string]any{{"type": "tool_result", "tool_use_id": "toolu_1", "content": "ok"}}
	side := turnLine(t, map[string]any{"type": "assistant", "isSidechain": true, "timestamp": "2026-09-29T10:00:05.000Z",
		"message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": "subagent words"}}}})

	lines := []string{
		user("p1", "first prompt", false),
		said(t, "working on it", "2026-09-29T10:00:01.000Z"),
		user("", result, false),
		user("", "skill text injected mid-turn", true),
		said(t, "p1's summary", "2026-09-29T10:00:04.000Z"),
		side,
		user("p2", "second prompt", false),
		said(t, "p2's summary", "2026-09-29T10:00:07.000Z"),
		user("", "a prompt from an older version", false),
		said(t, "the unkeyed reply", "2026-09-29T10:00:09.000Z"),
	}
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := FinalAssistantTexts(p, map[string]bool{"p1": true, "p2": true})
	if got["p1"].Text != "p1's summary" {
		t.Errorf("p1 = %q, want \"p1's summary\": a tool_result or meta line without promptId ended the turn, "+
			"or a sidechain line was read as the main agent's", got["p1"].Text)
	}
	if got["p2"].Text != "p2's summary" {
		t.Errorf("p2 = %q, want \"p2's summary\": an unkeyed prompt's reply was credited to the turn before it", got["p2"].Text)
	}
	if len(got) != 2 {
		t.Errorf("got %d prompts, want only the two wanted", len(got))
	}
}
