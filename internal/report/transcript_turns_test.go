package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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

// TestUserBlocks_DecodeOnlyBlockTypes holds the shape toolResultOnly decodes
// a user line into: a block's type and nothing else. A user line's content is
// a typed prompt or a tool's output, and deciding "tool result, not a prompt"
// needs neither, so no field may give either a place to land -- not a text
// or content tag, and not a json.RawMessage, which would copy the whole of
// message.content into a value.
func TestUserBlocks_DecodeOnlyBlockTypes(t *testing.T) {
	allowed := map[string]bool{"message": true, "content": true, "type": true}
	raw := reflect.TypeOf(json.RawMessage{})
	var walk func(reflect.Type, string)
	walk = func(ty reflect.Type, path string) {
		if ty == raw {
			t.Errorf("%s is a json.RawMessage: it holds the bytes it spans as a value", path)
			return
		}
		for ty.Kind() == reflect.Pointer || ty.Kind() == reflect.Slice {
			ty = ty.Elem()
		}
		if ty.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < ty.NumField(); i++ {
			f := ty.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			if !allowed[tag] {
				t.Errorf("%s.%s decodes %q: a user line is judged by its block types alone", path, f.Name, tag)
			}
			walk(f.Type, path+"."+f.Name)
		}
	}
	walk(reflect.TypeOf(userBlocks{}), "userBlocks")

	// And it still tells the two apart.
	for _, tc := range []struct {
		line string
		want bool
	}{
		{`{"type":"user","message":{"content":[{"type":"tool_result","content":"out"}]}}`, true},
		{`{"type":"user","message":{"content":[{"type":"tool_result"},{"type":"text","text":"and a prompt"}]}}`, false},
		{`{"type":"user","message":{"content":"a typed prompt"}}`, false},
		{`{"type":"user","message":{"content":[]}}`, false},
	} {
		if got := toolResultOnly([]byte(tc.line)); got != tc.want {
			t.Errorf("toolResultOnly(%s) = %v, want %v", tc.line, got, tc.want)
		}
	}
}
