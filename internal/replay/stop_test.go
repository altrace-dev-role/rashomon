package replay

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/history"
)

// hookWritten reports a transcript line that a hook caused to be written, and
// so one that history from before Rashomon was installed does not have: the
// system line Claude Code writes after Stop hooks ran, and attachments that
// carry a hook's output.
func hookWritten(l string) bool {
	var line struct {
		Type       string `json:"type"`
		Attachment struct {
			Type string `json:"type"`
		} `json:"attachment"`
	}
	if json.Unmarshal([]byte(l), &line) != nil {
		return false
	}
	return line.Type == "system" || line.Type == "attachment" && strings.HasPrefix(line.Attachment.Type, "hook_")
}

// TestREPLAY3_StopBoundariesDoNotNeedHookLines runs each fixture twice, as
// captured and with every line a hook wrote removed (the stop_hook_summary
// system line, and any attachment carrying a hook's output). The Stop must
// stand at the same place both times, the line of the last assistant message
// of the turn, carry the same payload, and be the Stop the captured payload
// says: a Stop rule that leaned on the hook's own line would move or vanish.
func TestREPLAY3_StopBoundariesDoNotNeedHookLines(t *testing.T) {
	for _, version := range []string{"2.1.280", "2.1.312"} {
		raw, err := os.ReadFile(fixtureDir + version + "/transcript.jsonl")
		if err != nil {
			t.Fatal(err)
		}
		all := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
		var kept []string
		removed := 0
		shift := make([]int, len(all)+2) // old line -> lines removed before it
		for i, l := range all {
			shift[i+1] = removed
			if hookWritten(l) {
				removed++
				continue
			}
			kept = append(kept, l)
		}
		if removed == 0 {
			t.Fatalf("%s: premise: the fixture has no hook-written line to remove", version)
		}
		full, err := Build(strings.NewReader(string(raw)), Options{TranscriptPath: placeholderPath})
		if err != nil {
			t.Fatal(err)
		}
		bare, err := Build(strings.NewReader(strings.Join(kept, "\n")+"\n"), Options{TranscriptPath: placeholderPath})
		if err != nil {
			t.Fatal(err)
		}
		if len(full.Events) != len(bare.Events) {
			t.Fatalf("%s: %d events with the hook's lines, %d without", version, len(full.Events), len(bare.Events))
		}
		stops := 0
		for i, a := range full.Events {
			b := bare.Events[i]
			// Positions in the bare file are the full file's less the lines removed before.
			want := history.Pos{Line: a.Pos.Line - shift[a.Pos.Line], Block: a.Pos.Block}
			if a.Kind != b.Kind || a.ToolUseID != b.ToolUseID || b.Pos != want || string(a.Payload) != string(b.Payload) ||
				a.BoundaryUnknown || b.BoundaryUnknown || !b.Supported() {
				t.Errorf("%s event %d: with %s %+v, without %s %+v (want %+v); unknown %v %v; unsupported %v", version, i, a.Kind, a.Pos, b.Kind, b.Pos, want, a.BoundaryUnknown, b.BoundaryUnknown, b.Unsupported)
			}
			if a.Kind == Stop {
				stops++
			}
		}
		if stops != 1 {
			t.Errorf("%s: %d Stops, the capture has one", version, stops)
		}
		// And it is the captured Stop: same session, cwd and message.
		var cap captured
		for _, c := range loadCaptured(t, version) {
			if c.event == "Stop" {
				cap = c
			}
		}
		for _, e := range bare.Events {
			if e.Kind != Stop {
				continue
			}
			var p map[string]string
			if err := json.Unmarshal(e.Payload, &p); err != nil {
				t.Fatal(err)
			}
			if p["session_id"] != cap.str("session_id") || p["cwd"] != cap.str("cwd") || p["last_assistant_message"] != cap.str("last_assistant_message") {
				t.Errorf("%s: the Stop without hook lines is %v, the captured is %s", version, p, cap.raw)
			}
		}
		if len(bare.Turns) != 1 || bare.Turns[0].End != EndStop || bare.Turns[0].StopReason != "end_turn" {
			t.Errorf("%s: turns %+v", version, bare.Turns)
		}
	}
}

// stopsOf lists a timeline's Stop events.
func stopsOf(tl Timeline) []Event {
	var out []Event
	for _, e := range tl.Events {
		if e.Kind == Stop {
			out = append(out, e)
		}
	}
	return out
}

// TestREPLAY3_UnknownStops: where the transcript does not establish a Stop,
// the event is a Stop of unknown boundary with a reason, never a Stop, and
// never absent where one may have fired. The file simply ending, or a
// reply that is not an end_turn, does not make one.
func TestREPLAY3_UnknownStops(t *testing.T) {
	cases := []struct {
		name   string
		build  func(d *doc)
		stops  int
		reason Reason // of the last Stop event, "" when it is a Stop
		at     func(d *doc, lines map[string]int) history.Pos
	}{
		{"ends mid-turn after a tool_result", func(d *doc) {
			d.promptLine("p1")
			d.uses("m1", bashUse("a", "make"))
			d.results("", okResult("a"))
		}, 1, StopNotEndTurn, func(d *doc, l map[string]int) history.Pos { return history.Pos{Line: 3, Block: 1} }},
		{"a stop_hook_summary after a turn cut mid-call is not a Stop", func(d *doc) {
			d.promptLine("p1")
			d.uses("m1", bashUse("a", "make"))
			d.results("", okResult("a"))
			d.hookNote() // proof a Stop hook ran, which history from before Rashomon never has: not read
		}, 1, StopNotEndTurn, func(d *doc, l map[string]int) history.Pos { return history.Pos{Line: 3, Block: 1} }},
		{"ends on a tool_use with no result", func(d *doc) {
			d.promptLine("p1")
			d.uses("m1", bashUse("a", "make"))
		}, 1, StopNotEndTurn, func(d *doc, l map[string]int) history.Pos { return history.Pos{Line: 2, Block: 1} }},
		{"a prompt and nothing else", func(d *doc) { d.promptLine("p1") }, 1, StopNotEndTurn,
			func(d *doc, l map[string]int) history.Pos { return history.Pos{Line: 1, Block: 1} }},
		{"interrupted after the last reply", func(d *doc) {
			d.promptLine("p1")
			d.uses("m1", bashUse("a", "make"))
			d.add(map[string]any{"type": "user", "message": map[string]any{"role": "user",
				"content": []any{map[string]any{"type": "text", "text": "[Request interrupted by user for tool use]"}}}})
		}, 1, StopInterrupted, func(d *doc, l map[string]int) history.Pos { return history.Pos{Line: 3, Block: 1} }},
		{"a reply that is not end_turn", func(d *doc) {
			d.promptLine("p1")
			d.add(map[string]any{"type": "assistant", "message": map[string]any{"id": "m", "role": "assistant", "stop_reason": "max_tokens",
				"content": []any{map[string]any{"type": "text", "text": "cut off"}}}})
		}, 1, StopNotEndTurn, func(d *doc, l map[string]int) history.Pos { return history.Pos{Line: 2, Block: 1} }},
		{"an unreadable line follows the last reply", func(d *doc) {
			d.promptLine("p1")
			d.ends("m1", "done")
			d.raw("{cut")
		}, 1, StopUnreadableLine, func(d *doc, l map[string]int) history.Pos { return history.Pos{Line: 2, Block: 1} }},
		{"the turn went on after an end_turn", func(d *doc) {
			d.promptLine("p1")
			d.ends("m1", "done?")
			d.uses("m2", bashUse("a", "make")) // another Stop hook blocked the stop
			d.results("", okResult("a"))
			d.ends("m3", "now done")
		}, 2, "", func(d *doc, l map[string]int) history.Pos { return history.Pos{Line: 2, Block: 1} }},
		{"a prompt answered by no model turn, then another prompt, is no Stop", func(d *doc) {
			d.promptLine("p1")
			d.promptLine("p2")
			d.ends("m1", "done")
		}, 1, "", func(d *doc, l map[string]int) history.Pos { return history.Pos{Line: 3, Block: 1} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newDoc()
			tc.build(d)
			tl := d.build(t, history.DefaultLimits)
			stops := stopsOf(tl)
			if len(stops) != tc.stops {
				t.Fatalf("%d Stop events %+v, want %d", len(stops), stops, tc.stops)
			}
			s := stops[len(stops)-1]
			switch {
			case tc.reason == "" && s.BoundaryUnknown:
				t.Errorf("a Stop the transcript establishes is unknown: %v", s.Unsupported)
			case tc.reason != "" && (!s.BoundaryUnknown || len(s.Unsupported) != 1 || s.Unsupported[0] != tc.reason || s.Gap != tc.reason || s.Payload != nil):
				t.Errorf("Stop = unknown %v, %v, gap %q, payload %q; want unknown, %s", s.BoundaryUnknown, s.Unsupported, s.Gap, s.Payload, tc.reason)
			}
			if want := tc.at(d, nil); tc.name != "the turn went on after an end_turn" && s.Pos != want {
				t.Errorf("Stop at %+v, want %+v", s.Pos, want)
			}
			if tc.name == "the turn went on after an end_turn" {
				first := stops[0]
				if !first.BoundaryUnknown || first.Unsupported[0] != StopContinued || first.Pos != tc.at(d, nil) || stops[1].BoundaryUnknown {
					t.Errorf("stops = %+v", stops)
				}
			}
			for _, e := range tl.Events {
				if e.Kind == Stop && !e.BoundaryUnknown && e.Payload == nil {
					t.Error("a Stop with no payload that is not unknown")
				}
			}
		})
	}
}
