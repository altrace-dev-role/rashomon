package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
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

func said(t *testing.T, id, text, at string) string {
	return turnLine(t, map[string]any{"type": "assistant", "timestamp": at,
		"message": map[string]any{"id": id, "role": "assistant", "content": []map[string]any{{"type": "text", "text": text}}}})
}

func userTurnLine(t *testing.T, prompt string, content any, meta bool) string {
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

func writeTurns(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestFinalAssistantTexts_ATurnIsItsPrompt holds the attribution rules on the
// shapes a real 2.1.285 transcript has: assistant lines belong to the prompt
// of the user line before them; a tool_result with no promptId (seen once in
// a real transcript) and an injected meta line stay inside the turn; a
// subagent's sidechain line is not the main agent's word; and a typed
// prompt with no promptId ends the turn, so its reply is nobody's.
func TestFinalAssistantTexts_ATurnIsItsPrompt(t *testing.T) {
	user := func(prompt string, content any, meta bool) string { return userTurnLine(t, prompt, content, meta) }
	result := []map[string]any{{"type": "tool_result", "tool_use_id": "toolu_1", "content": "ok"}}
	side := turnLine(t, map[string]any{"type": "assistant", "isSidechain": true, "timestamp": "2026-09-29T10:00:05.000Z",
		"message": map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": "subagent words"}}}})

	p := writeTurns(t,
		user("p1", "first prompt", false),
		said(t, "m1", "working on it", "2026-09-29T10:00:01.000Z"),
		user("", result, false),
		user("", "skill text injected mid-turn", true),
		said(t, "m2", "p1's summary", "2026-09-29T10:00:04.000Z"),
		side,
		user("p2", "second prompt", false),
		said(t, "m3", "p2's summary", "2026-09-29T10:00:07.000Z"),
		user("", "a prompt from an older version", false),
		said(t, "m4", "the unkeyed reply", "2026-09-29T10:00:09.000Z"))
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
	// The final word's own line dates it, and the turn's spend is every
	// response tied to it: m1 and m2 are p1's, m3 is p2's, the unkeyed
	// reply's m4 is no turn's.
	if want := time.Date(2026, 9, 29, 10, 0, 4, 0, time.UTC).UnixMilli(); got["p1"].AtMS != want {
		t.Errorf("p1's AtMS = %d, want %d, the timestamp of the line that carried its summary", got["p1"].AtMS, want)
	}
	if !reflect.DeepEqual(got["p1"].Responses, map[string]bool{"m1": true, "m2": true}) ||
		!reflect.DeepEqual(got["p2"].Responses, map[string]bool{"m3": true}) {
		t.Errorf("responses: p1 %v, p2 %v; want p1 m1 and m2, p2 m3", got["p1"].Responses, got["p2"].Responses)
	}
}

// TestFinalAssistantTexts_AnUnreadableLineIsNoTurnsWord: a malformed line
// the reader cannot place must never let a turn be judged on words that may
// not be its last, nor tie the next prompt's spend to it.
//
// A text line with a timestamp that does not parse was skipped, and the turn
// was judged on earlier interim text ("On it."), so it could falsely fire. A
// user line whose header does not decode (a numeric timestamp) kept the tie,
// so the next prompt's reply and spend were credited to this turn -- "at
// least $0.44" printed for a turn whose real spend was $0.04.
func TestFinalAssistantTexts_AnUnreadableLineIsNoTurnsWord(t *testing.T) {
	undecodable := strings.Replace(userTurnLine(t, "p2", "the next prompt", false), `"timestamp":"2026-09-29T10:00:00.000Z"`, `"timestamp":5`, 1)
	if undecodable == userTurnLine(t, "p2", "the next prompt", false) {
		t.Fatal("premise: the timestamp was not replaced")
	}
	p := writeTurns(t,
		userTurnLine(t, "p1", "first prompt", false),
		said(t, "m1", "On it.", "2026-09-29T10:00:01.000Z"),
		said(t, "m2", "The command failed.", "not a time"),
		userTurnLine(t, "p2", "second prompt", false),
		said(t, "m3", "On it.", "2026-09-29T10:00:03.000Z"),
		said(t, "m4", "Hmm.", "not a time"),
		said(t, "m5", "p2's summary", "2026-09-29T10:00:05.000Z"),
		userTurnLine(t, "p3", "third prompt", false),
		said(t, "m6", "Working.", "2026-09-29T10:00:06.000Z"),
		undecodable,
		said(t, "m7", "The next prompt's reply.", "2026-09-29T10:00:08.000Z"),
		// A text line whose timestamp is a number: its header does not
		// decode, so it may be the turn's last word.
		userTurnLine(t, "p4", "fourth prompt", false),
		said(t, "m8", "On it.", "2026-09-29T10:00:09.000Z"),
		strings.Replace(said(t, "m9", "The command failed.", "2026-09-29T10:00:10.000Z"), `"timestamp":"2026-09-29T10:00:10.000Z"`, `"timestamp":10`, 1),
		// An assistant line whose content is an object: its header decodes,
		// its text does not.
		userTurnLine(t, "p5", "fifth prompt", false),
		said(t, "m10", "On it.", "2026-09-29T10:00:11.000Z"),
		turnLine(t, map[string]any{"type": "assistant", "timestamp": "2026-09-29T10:00:12.000Z",
			"message": map[string]any{"id": "m11", "role": "assistant", "content": map[string]any{"type": "text", "text": "The command failed."}}}))
	got := FinalAssistantTexts(p, map[string]bool{"p1": true, "p2": true, "p3": true, "p4": true, "p5": true})
	if got["p1"].Said {
		t.Errorf("p1 = %q, want no words: its last text line could not be dated, so the earlier one is not its last", got["p1"].Text)
	}
	if !got["p2"].Said || got["p2"].Text != "p2's summary" {
		t.Errorf("p2 = %q, want \"p2's summary\": a later dated line is the turn's word again", got["p2"].Text)
	}
	if got["p3"].Said || got["p3"].Responses["m7"] {
		t.Errorf("p3 = %+v: the reply after a user line that did not decode was credited to it", got["p3"])
	}
	for _, p := range []string{"p4", "p5"} {
		if got[p].Said {
			t.Errorf("%s = %q, want no words: its last text line could not be read, so the earlier one is not its last", p, got[p].Text)
		}
	}
}

// TestFinalAssistantTexts_AnUndecodableLineEndsTheTieUnlessItIsASidechain:
// which undecodable lines ended a turn's tie was read from the bytes "user"
// and "text". That fired on an assistant line whose tool_use input held the
// value "user", and on a subagent's sidechain user line -- whose responses
// after it then fell out of the turn -- while a user line truncated before
// its type, holding neither word, kept the tie and credited the next
// prompt's spend and words to this turn. The rule is now the one the bound
// states: any undecodable main-transcript line ends the tie and drops the
// turn's words, unless it is a sidechain line.
func TestFinalAssistantTexts_AnUndecodableLineEndsTheTieUnlessItIsASidechain(t *testing.T) {
	side := func(id, at string) string {
		return turnLine(t, map[string]any{"type": "assistant", "isSidechain": true, "timestamp": at,
			"message": map[string]any{"id": id, "role": "assistant", "content": []map[string]any{{"type": "text", "text": "subagent words"}}}})
	}
	for _, tc := range []struct {
		name, line string
		ends       bool
	}{
		{"a malformed assistant line whose tool_use input holds \"user\"",
			`{"type":"assistant","timestamp":5,"message":{"id":"mx","role":"assistant","content":[{"type":"tool_use","input":{"role":"user"}}]}}`, true},
		{"a user line truncated before its type", `{"parentUuid":"u-1","isSidechain":false,"promptId":"p2","mess`, true},
		{"an undecodable sidechain user line", `{"type":"user","isSidechain":true,"timestamp":5,"message":{"role":"user","content":"task"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := writeTurns(t,
				userTurnLine(t, "p1", "first prompt", false),
				said(t, "m1", "Ran it as requested.", "2026-09-29T10:00:01.000Z"),
				tc.line,
				side("s1", "2026-09-29T10:00:03.000Z"),
				side("s2", "2026-09-29T10:00:04.000Z"))
			got := FinalAssistantTexts(p, map[string]bool{"p1": true})["p1"]
			if tc.ends {
				if got.Said || got.Responses["s1"] || got.Responses["s2"] {
					t.Errorf("p1 = %+v: the line after an undecodable one was tied to the turn, or its earlier words kept", got)
				}
				return
			}
			if !got.Said || got.Text != "Ran it as requested." || !got.Responses["s1"] || !got.Responses["s2"] {
				t.Errorf("p1 = %+v: an undecodable sidechain line ended the tie, so the subagent's responses fell out of the turn", got)
			}
		})
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

// TestAssistantLine_DecodesOnlyTextBlocks holds the shape FinalAssistantTexts
// decodes an assistant line into: the role, and each block's type and text.
// The earlier reader held message.content whole in a json.RawMessage -- every
// block of every tied line, a tool_use input with a "text" key included --
// while the README said only the final assistant message was read. No field
// may be a RawMessage, an interface or a map, each of which would hold bytes
// it was never asked for, and only the text of "text" blocks is kept.
func TestAssistantLine_DecodesOnlyTextBlocks(t *testing.T) {
	allowed := map[string]bool{"message": true, "role": true, "content": true, "type": true, "text": true}
	raw := reflect.TypeOf(json.RawMessage{})
	var walk func(reflect.Type, string)
	walk = func(ty reflect.Type, path string) {
		for ty.Kind() == reflect.Pointer || ty.Kind() == reflect.Slice {
			if ty == raw {
				t.Errorf("%s is a json.RawMessage: it holds the bytes it spans as a value", path)
				return
			}
			ty = ty.Elem()
		}
		switch ty.Kind() {
		case reflect.Interface, reflect.Map:
			t.Errorf("%s is a %s: it holds whatever it is handed", path, ty.Kind())
			return
		case reflect.Struct:
		default:
			return
		}
		for i := 0; i < ty.NumField(); i++ {
			f := ty.Field(i)
			tag := strings.Split(f.Tag.Get("json"), ",")[0]
			if !allowed[tag] {
				t.Errorf("%s.%s decodes %q: an assistant line is read for its text blocks alone", path, f.Name, tag)
			}
			walk(f.Type, path+"."+f.Name)
		}
	}
	fn := reflect.TypeOf(decodeAssistantLine)
	walk(fn.Out(0), "decodeAssistantLine")

	// And FinalAssistantTexts decodes every text-bearing assistant line it
	// reads through that helper, and no other way: a second decode beside
	// it would be a shape this walk never sees.
	decoded := 0
	defer func(orig func([]byte) (assistantLine, error)) { decodeAssistantLine = orig }(decodeAssistantLine)
	orig := decodeAssistantLine
	decodeAssistantLine = func(raw []byte) (assistantLine, error) {
		decoded++
		return orig(raw)
	}
	p := writeTurns(t,
		userTurnLine(t, "p1", "a prompt", false),
		said(t, "m1", "On it.", "2026-09-29T10:00:01.000Z"),
		said(t, "m2", "Done.", "2026-09-29T10:00:02.000Z"))
	if got := FinalAssistantTexts(p, map[string]bool{"p1": true})["p1"]; got.Text != "Done." || decoded != 2 {
		t.Errorf("p1 = %q after %d decodes through decodeAssistantLine, want \"Done.\" after 2", got.Text, decoded)
	}

	// And only a text block's text is kept, from either content shape.
	for _, tc := range []struct {
		line, want string
	}{
		{`{"message":{"role":"assistant","content":[{"type":"thinking","thinking":"t"},` +
			`{"type":"tool_use","name":"Write","input":{"text":"a file body"}},{"type":"text","text":"Done."}]}}`, "Done."},
		{`{"message":{"role":"assistant","content":"Plain words."}}`, "Plain words."},
		{`{"message":{"role":"assistant","content":[{"type":"tool_use","input":{"text":"a file body"}}]}}`, ""},
		{`{"message":{"role":"assistant","content":[{"type":"other","text":"not a text block"},{"type":"text","text":"Done."}]}}`, "Done."},
	} {
		var l assistantLine
		if err := json.Unmarshal([]byte(tc.line), &l); err != nil {
			t.Fatal(err)
		}
		if got, _ := l.Message.Content.text(); got != tc.want {
			t.Errorf("text(%s) = %q, want %q", tc.line, got, tc.want)
		}
	}
}
