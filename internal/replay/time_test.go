package replay

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestREPLAY4b_EventsCarryTheTimeOfTheirLines: the recorder's clock is set to
// the event's historical time, which is the timestamp of the transcript line
// the event is taken from: the tool_use line for a Pre, the tool_result line
// for a Post, the prompt's line, and the last line of the turn's final
// message for a Stop. A line with no usable timestamp makes the event
// Unsupported (NoTimestamp), and no other line's time is used.
func TestREPLAY4b_EventsCarryTheTimeOfTheirLines(t *testing.T) {
	d := newDoc()
	d.promptLine("p1")                        // 1
	d.uses("m1", bashUse("a", "make"))        // 2
	d.uses("m1b", bashUse("b", "make test"))  // 3
	d.results("", okResult("b"))              // 4: answered before a
	d.results("", okResult("a"))              // 5
	d.add(map[string]any{"type": "assistant", // 6: a message of two lines, the first not the last
		"message": map[string]any{"id": "m2", "role": "assistant", "stop_reason": "end_turn",
			"content": []any{map[string]any{"type": "thinking", "thinking": "hm"}}}})
	d.ends("m2", "done") // 7
	tl := d.build(t, history0)
	want := map[string]int{"UserPromptSubmit": 1, "PreToolUse:a": 2, "PreToolUse:b": 3, "PostToolUse:b": 4, "PostToolUse:a": 5, "Stop": 7}
	got := map[string]int{}
	for _, e := range tl.Events {
		key := e.Kind.String()
		if e.ToolUseID != "" {
			key += ":" + e.ToolUseID
		}
		if !e.Supported() {
			t.Fatalf("%s unsupported: %v", key, e.Unsupported)
		}
		for line := 1; line < 20; line++ {
			if e.At.Equal(synthTime(line)) {
				got[key] = line
			}
		}
	}
	for k, line := range want {
		if got[k] != line {
			t.Errorf("%s is at the time of line %d, want line %d", k, got[k], line)
		}
	}

	// A line with no timestamp, or one that is not a time.
	for name, ts := range map[string]any{"none": nil, "garbage": "yesterday", "date only": "2026-10-29"} {
		d := newDoc()
		d.promptLine("p1")
		m := map[string]any{"type": "assistant",
			"message": map[string]any{"id": "m1", "role": "assistant", "stop_reason": "tool_use",
				"content": []any{map[string]any{"type": "tool_use", "id": "a", "name": "Bash", "input": map[string]any{"command": "make"}}}}}
		if ts != nil {
			m["timestamp"] = ts
		}
		// add fills a missing timestamp, so write the line as it is.
		if ts == nil {
			m["sessionId"], m["version"], m["cwd"], m["isSidechain"] = synthSession, "2.1.312", "/repo", false
			d.raw(mustJSON(m))
		} else {
			d.add(m)
		}
		d.results("", okResult("a"))
		d.ends("m2", "done")
		tl := d.build(t, history0)
		pre := find(t, tl, PreToolUse, "a")
		if pre.Supported() || reasons(pre.Unsupported) != "no_timestamp" || !pre.At.IsZero() || pre.Payload != nil {
			t.Errorf("%s: a Pre whose line has no usable time: unsupported %v, at %v", name, pre.Unsupported, pre.At)
		}
		if post := find(t, tl, PostToolUse, "a"); !post.Supported() {
			t.Errorf("%s: the Post (another line, with a time) is unsupported: %v", name, post.Unsupported)
		}
		if pre.Gap != NoTimestamp || pre.Taint != pre.Pos {
			t.Errorf("%s: an event with no time must be the gap that taints from its own position: gap %q, taint %+v", name, pre.Gap, pre.Taint)
		}
	}
}

// TestREPLAY3b_KeepFinalIsWhatTheRecorderKeeps: a message no longer than the
// recorder's head, tail and the margins is kept whole; a longer one is kept as
// its head and tail, valid UTF-8, never longer than they and the margins, and
// beginning and ending with the message's own bytes.
func TestREPLAY3b_KeepFinalIsWhatTheRecorderKeeps(t *testing.T) {
	limit := MessageHeadBytes + MessageTailBytes + 2*finalMargin
	for _, unit := range []string{"a", "é", "漢", "😀"} {
		for _, n := range []int{0, 10, limit - 1, limit, limit + 1, limit + 5, 3 * limit} {
			var b strings.Builder
			for i := 0; b.Len() < n; i++ {
				b.WriteString(unit)
				b.WriteByte(byte('a' + i%26))
			}
			s := b.String()
			kept, elided := keepFinal(s)
			if len(s) <= limit {
				if elided || kept != s {
					t.Errorf("%q x%d: a short message was cut", unit, n)
				}
				continue
			}
			if !elided || !utf8.ValidString(kept) {
				t.Errorf("%q x%d: elided %v, valid %v", unit, n, elided, utf8.ValidString(kept))
			}
			if len(kept) > limit {
				t.Errorf("%q x%d: kept %d bytes, over %d", unit, n, len(kept), limit)
			}
			if !strings.HasPrefix(s, kept[:MessageHeadBytes]) || !strings.HasSuffix(s, kept[len(kept)-MessageTailBytes:]) {
				t.Errorf("%q x%d: the kept text does not begin and end with the message's own head and tail", unit, n)
			}
		}
	}
}
