package report

import (
	"encoding/json"
	"strings"
	"testing"
)

// assistantWith is a one-turn main transcript whose only assistant line
// carries content, raw JSON, as its message.content.
func assistantWith(t *testing.T, content string) string {
	t.Helper()
	line := `{"type":"assistant","timestamp":"2026-09-29T10:00:02.000Z","message":{"id":"msg_1","role":"assistant","content":` +
		content + `}}`
	return userTurnLine(t, "p1", "do it", false) + "\n" + line + "\n"
}

// checkTextPrefilter is FuzzTextPrefilter's property for one content value:
// when the full decode of the line finds text, FinalAssistantTexts reports
// exactly that text for the turn, and FinalAssistantText agrees with it.
func checkTextPrefilter(t *testing.T, content []byte) {
	if !json.Valid(content) || strings.ContainsAny(string(content), "\n\r") {
		return
	}
	line := `{"message":{"role":"assistant","content":` + string(content) + `}}`
	decoded, err := decodeAssistantLine([]byte(line))
	if err != nil {
		return
	}
	want, found := decoded.Message.Content.text()
	transcript := assistantWith(t, string(content))
	got := FinalAssistantTexts(strings.NewReader(transcript), map[string]bool{"p1": true})["p1"]
	if found && (!got.Said || got.Text != want) {
		t.Errorf("content %s: the decode finds text %q, FinalAssistantTexts reports %+v", content, want, got)
	}
	single, ok := FinalAssistantText(strings.NewReader(transcript))
	if ok != got.Said || single != got.Text {
		t.Errorf("content %s: FinalAssistantText = %q, %v; FinalAssistantTexts = %q, %v", content, single, ok, got.Text, got.Said)
	}
}

// textPrefilterSeeds are content values a real transcript writes -- an array
// of blocks, a plain string -- and the spellings encoding/json also accepts.
var textPrefilterSeeds = []string{
	`[{"type":"text","text":"Done; the build passes."}]`,
	`"Done; the build passes."`,
	`[{"type":"text","Text":"a capital key"}]`,
	`[{"type":"t\u0065xt","T\u0065xt":"an escaped type and key"}]`,
	`[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls"}}]`,
	`"   "`,
	`null`,
}

// TestRubricPERF3_APlainStringReplyIsTheTurnsFinalWord holds the turn reader
// to the decode on a reply whose content is a plain string, a shape this
// file's own decoder says real transcripts carry. A byte prefilter for the
// bytes "text" skipped such a line before decoding it: the turn had no final
// words, and the two final-message readers disagreed on one file.
func TestRubricPERF3_APlainStringReplyIsTheTurnsFinalWord(t *testing.T) {
	transcript := assistantWith(t, `"Done; the build passes."`)
	got := FinalAssistantTexts(strings.NewReader(transcript), map[string]bool{"p1": true})["p1"]
	if !got.Said || got.Text != "Done; the build passes." {
		t.Errorf("FinalAssistantTexts[p1] = %+v; want the plain-string reply", got)
	}
	for _, seed := range textPrefilterSeeds {
		checkTextPrefilter(t, []byte(seed))
	}
}

// FuzzTextPrefilter fuzzes a turn's one assistant line's content against the
// full decode of that line, and the two final-message readers against each
// other.
func FuzzTextPrefilter(f *testing.F) {
	for _, seed := range textPrefilterSeeds {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, content []byte) {
		checkTextPrefilter(t, content)
	})
}

// sidechainHead is a header shape whose timestamp is a string, so a line
// with a numeric timestamp does not decode and DecodeHeader falls back to
// reading the isSidechain key alone.
type sidechainHead struct {
	Timestamp string `json:"timestamp"`
}

// TestRubricTEST14_AnUndecodableLineIsASidechainOneByTheDecodesRule holds the
// key walk behind an undecodable line to the rule a struct decode applies to
// the same key: any case under Unicode folding, the last occurrence winning,
// null leaving the value as it was. The walk took the first key spelt
// exactly, so a line the decode reads as the main agent's could end a
// subagent's tie, and the reverse.
func TestRubricTEST14_AnUndecodableLineIsASidechainOneByTheDecodesRule(t *testing.T) {
	cases := []struct {
		line string
		want LineKind
	}{
		{`{"isSidechain":true,"timestamp":1}`, LineUndecodableSidechain},
		{`{"IsSidechain":true,"timestamp":1}`, LineUndecodableSidechain},
		{"{\"iſSidechain\":true,\"timestamp\":1}", LineUndecodableSidechain},
		{`{"isSidechain":false,"isSidechain":true,"timestamp":1}`, LineUndecodableSidechain},
		{`{"isSidechain":true,"ISSIDECHAIN":false,"timestamp":1}`, LineUndecodable},
		{`{"isSidechain":true,"isSidechain":null,"timestamp":1}`, LineUndecodableSidechain},
		{`{"isSidechain":true,"isSidechain":"no","timestamp":1}`, LineUndecodableSidechain},
		{`{"message":{"isSidechain":true},"timestamp":1}`, LineUndecodable},
		{`{"isSidechain":true,"timestamp":1`, LineUndecodableSidechain},
	}
	for _, c := range cases {
		var head sidechainHead
		if got := DecodeHeader([]byte(c.line), &head); got != c.want {
			t.Errorf("DecodeHeader(%s) = %v, want %v", c.line, got, c.want)
		}
		checkSidechainKey(t, []byte(c.line))
	}
}

// checkSidechainKey is FuzzSidechainKey's property: on any line a struct
// decode reads isSidechain from without error, the walk reads the same.
func checkSidechainKey(t *testing.T, raw []byte) {
	var s struct {
		IsSidechain bool `json:"isSidechain"`
	}
	if json.Unmarshal(raw, &s) != nil {
		return
	}
	if got := topLevelSidechain(raw); got != s.IsSidechain {
		t.Errorf("topLevelSidechain(%q) = %v; the struct decode reads %v", raw, got, s.IsSidechain)
	}
}

// FuzzSidechainKey fuzzes topLevelSidechain against a struct decode of the
// one key it reads.
func FuzzSidechainKey(f *testing.F) {
	f.Add([]byte(`{"isSidechain":true}`))
	f.Add([]byte(`{"isSidechain":false,"ISSIDECHAIN":true,"isSidechain":null}`))
	f.Add([]byte(`{"type":"assistant","message":{"isSidechain":true},"isSidechain":false}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		checkSidechainKey(t, raw)
	})
}
