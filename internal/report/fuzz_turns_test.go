package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// FuzzFinalAssistantTexts holds the per-turn reader to the whole-file one.
// A fuzzed script builds a transcript of well-formed lines -- prompts, text
// and tool_use replies, tool results, blank lines -- and for each turn,
// FinalAssistantTexts over the whole file must say what FinalAssistantText
// says over that turn's own lines (its prompt up to the next): the same text,
// or none. Its Responses are the turn's assistant message ids, and AtMS the
// timestamp of the line its text came from.
func FuzzFinalAssistantTexts(f *testing.F) {
	f.Add([]byte{0, 1, 2, 3, 1, 0, 4, 5, 1}, "Done.|On it.|  |All 12 tests pass.")
	f.Add([]byte{1, 0, 2, 3, 6, 0, 0, 1}, "ok|<>&\"\\|é中")
	f.Add([]byte{0, 1, 8, 0, 9, 3, 2}, "first|second")
	f.Fuzz(func(t *testing.T, script []byte, words string) { checkTurns(t, script, words) })
}

// TestRubricTest1_TurnSeeds runs FuzzFinalAssistantTexts' property over its
// seeds as a plain test, which is what a mutant names.
func TestRubricTest1_TurnSeeds(t *testing.T) {
	for _, s := range []struct {
		script []byte
		words  string
	}{
		{[]byte{0, 1, 2, 3, 1, 0, 4, 5, 1}, "Done.|On it.|  |All 12 tests pass."},
		{[]byte{1, 0, 2, 3, 6, 0, 0, 1}, "ok|<>&\"\\|é中"},
		{[]byte{0, 1, 8, 0, 9, 3, 2}, "first|second"},
		{[]byte{0, 1, 7, 1, 0, 1, 4}, "a|b|c"},
	} {
		checkTurns(t, s.script, s.words)
	}
}

type turnWant struct {
	prompt string
	lines  []string
	ids    map[string]bool
	textAt int64
}

func checkTurns(t *testing.T, script []byte, words string) {
	t.Helper()
	if len(script) > 64 {
		script = script[:64]
	}
	pool := strings.Split(words, "|")
	t0 := time.Date(2026, 10, 13, 10, 0, 0, 0, time.UTC)
	var all []string
	var turns []*turnWant
	var cur *turnWant
	add := func(line string) {
		all = append(all, line)
		if cur != nil {
			cur.lines = append(cur.lines, line)
		}
	}
	quote := func(s string) string {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	for i, op := range script {
		at := t0.Add(time.Duration(i) * time.Second)
		ts := quote(at.Format(time.RFC3339Nano))
		word := pool[int(op/10)%len(pool)]
		id := "msg_" + strconv.Itoa(i)
		assistant := func(content string) {
			line := `{"type":"assistant","timestamp":` + ts + `,"message":{"id":"` + id +
				`","role":"assistant","content":` + content + `}}`
			add(line)
			if cur == nil {
				return
			}
			cur.ids[id] = true
			if _, ok := FinalAssistantText(writeTranscript(t, []string{line})); ok {
				cur.textAt = at.UnixMilli()
			}
		}
		switch op % 10 {
		case 0, 9:
			cur = &turnWant{prompt: "p" + strconv.Itoa(len(turns)), ids: map[string]bool{}}
			turns = append(turns, cur)
			content := quote(word)
			if op%10 == 9 {
				content = `[{"type":"text","text":` + quote(word) + `}]`
			}
			add(`{"type":"user","promptId":"` + cur.prompt + `","timestamp":` + ts +
				`,"message":{"role":"user","content":` + content + `}}`)
		case 1:
			assistant(`[{"type":"text","text":` + quote(word) + `}]`)
		case 2:
			assistant(`[{"type":"tool_use","id":"toolu_` + strconv.Itoa(i) + `","name":"Bash","input":{"text":` + quote(word) + `}}]`)
		case 3:
			prompt := ""
			if cur != nil {
				prompt = cur.prompt
			}
			add(`{"type":"user","promptId":"` + prompt + `","timestamp":` + ts +
				`,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_x","content":` + quote(word) + `}]}}`)
		case 4:
			assistant(`[{"type":"thinking","thinking":"hm"},{"type":"text","text":` + quote(word) +
				`},{"type":"tool_use","id":"toolu_` + strconv.Itoa(i) + `","name":"Bash","input":{}}]`)
		case 5:
			add("")
		case 6:
			assistant(quote(word))
		case 7:
			add(" \t ")
		case 8:
			assistant(`[{"type":"text","text":` + quote(word) + `},{"type":"text","text":` + quote(word) + `}]`)
		}
	}
	want := map[string]bool{}
	for _, tw := range turns {
		want[tw.prompt] = true
	}
	got := FinalAssistantTexts(writeTranscript(t, all), want)
	for _, tw := range turns {
		text, ok := FinalAssistantText(writeTranscript(t, tw.lines))
		g := got[tw.prompt]
		if g.Said != ok || g.Text != text {
			t.Fatalf("turn %s: FinalAssistantTexts said %v %q, FinalAssistantText over its lines %v %q\n%s",
				tw.prompt, g.Said, g.Text, ok, text, strings.Join(all, "\n"))
		}
		if ok && g.AtMS != tw.textAt {
			t.Fatalf("turn %s: text at %d, want %d", tw.prompt, g.AtMS, tw.textAt)
		}
		if len(g.Responses) != len(tw.ids) {
			t.Fatalf("turn %s: responses %v, want %v", tw.prompt, g.Responses, tw.ids)
		}
		for id := range tw.ids {
			if !g.Responses[id] {
				t.Fatalf("turn %s: responses %v, want %v", tw.prompt, g.Responses, tw.ids)
			}
		}
	}
	for p := range got {
		if !want[p] {
			t.Fatalf("FinalAssistantTexts returned %q, which was not asked for", p)
		}
	}
}

func writeTranscript(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
