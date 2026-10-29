package replay

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/history"
)

// FuzzReplayTimeline holds the timeline to the properties that do not depend
// on what the transcript says, over arbitrary input read as a transcript and
// over a transcript generated from ops (a prompt, uses, results of each kind,
// a turn end, a sidechain use, a line that cannot be read):
//
//   - no panic, and the same timeline on a second read;
//   - events are in strictly increasing position, whatever order calls were
//     used and answered in;
//   - each call has one PreToolUse and at most one Post, after it; a denial or
//     a call with no result has none; Denied and Missing are never both;
//   - a supported event has a payload that is a JSON object naming its own
//     event, an unsupported one has none and is a Gap, and a Gap has a Taint;
//   - Taint is zero or at most the event's own position, never moves later
//     once set, and an event after a Gap carries a Taint no later than it.
func FuzzReplayTimeline(f *testing.F) {
	f.Add([]byte(`{"type":"user","message":{"content":"hi"}}`), []byte{0, 1, 2, 3, 4})
	f.Add([]byte("{\n\n[]\n"), []byte{0, 1, 1, 2, 2, 5, 3, 6, 0, 4})
	f.Add([]byte{}, []byte{1, 0, 2, 0, 3, 7, 0, 1, 2, 1, 3})
	f.Fuzz(func(t *testing.T, raw, ops []byte) {
		checkTimelineProps(t, string(raw))
		checkTimelineProps(t, generate(ops))
	})
}

func generate(ops []byte) string {
	d := newDoc()
	next := 0
	for i := 0; i+1 < len(ops); i += 2 {
		id := fmt.Sprintf("t%d", ops[i+1]%5)
		switch ops[i] % 9 {
		case 0:
			d.promptLine(fmt.Sprintf("p%d", next))
			next++
		case 1:
			d.uses(fmt.Sprintf("m%d", next), bashUse(id, "make"))
			next++
		case 2:
			d.results("", okResult(id))
		case 3:
			d.results("", failResult(id))
		case 4:
			d.results("", denyResult(id))
		case 5:
			d.ends(fmt.Sprintf("m%d", next), "done")
			next++
		case 6:
			d.usesWith(map[string]any{"isSidechain": true, "agentId": "a"}, fmt.Sprintf("m%d", next), bashUse(id, "make"))
			next++
		case 7:
			d.raw("{cut")
		case 8:
			d.results("-", okResult(id))
		}
	}
	return d.String()
}

func checkTimelineProps(t *testing.T, text string) {
	t.Helper()
	opt := Options{TranscriptPath: synthPath}
	tl, err := Build(strings.NewReader(text), opt)
	if err != nil {
		t.Fatalf("a read from memory failed: %v", err)
	}
	again, err := Build(strings.NewReader(text), opt)
	if err != nil || fmt.Sprint(again.Events) != fmt.Sprint(tl.Events) {
		t.Fatalf("a second read differs (%v)", err)
	}
	pre, post := map[string]history.Pos{}, map[string]int{}
	var first history.Pos
	for i, e := range tl.Events {
		if i > 0 && !tl.Events[i-1].Pos.Before(e.Pos) {
			t.Fatalf("events %d and %d out of order: %+v then %+v", i-1, i, tl.Events[i-1].Pos, e.Pos)
		}
		switch e.Kind {
		case PreToolUse:
			if _, dup := pre[e.ToolUseID]; dup {
				t.Fatalf("two PreToolUse for %q", e.ToolUseID)
			}
			pre[e.ToolUseID] = e.Pos
			if e.Denied && e.Missing {
				t.Fatalf("%q is both denied and missing", e.ToolUseID)
			}
		case PostToolUse, PostToolUseFailure:
			p, ok := pre[e.ToolUseID]
			if !ok || !p.Before(e.Pos) {
				t.Fatalf("Post for %q at %+v with Pre at %+v (%v)", e.ToolUseID, e.Pos, p, ok)
			}
			if post[e.ToolUseID]++; post[e.ToolUseID] > 1 {
				t.Fatalf("two Post for %q", e.ToolUseID)
			}
		}
		if e.Supported() {
			var m map[string]json.RawMessage
			var name string
			if json.Unmarshal(e.Payload, &m) != nil || json.Unmarshal(m["hook_event_name"], &name) != nil || name != e.Kind.String() || e.Gap != "" && e.Gap != Evicted {
				t.Fatalf("%s %q: payload %q, gap %q", e.Kind, e.ToolUseID, e.Payload, e.Gap)
			}
		} else if e.Payload != nil || e.Gap == "" {
			t.Fatalf("%s %q is unsupported with payload %q gap %q", e.Kind, e.ToolUseID, e.Payload, e.Gap)
		}
		if e.Gap != "" && e.Taint.IsZero() {
			t.Fatalf("%s %q is a gap with no taint", e.Kind, e.ToolUseID)
		}
		if !e.Taint.IsZero() && e.Pos.Before(e.Taint) {
			t.Fatalf("%s %q at %+v is tainted from later, %+v", e.Kind, e.ToolUseID, e.Pos, e.Taint)
		}
		if !first.IsZero() && e.Taint != first && first.Before(e.Taint) || !first.IsZero() && e.Taint.IsZero() {
			t.Fatalf("taint moved later or went away: %+v then %+v", first, e.Taint)
		}
		if !e.Taint.IsZero() && (first.IsZero() || e.Taint.Before(first)) {
			first = e.Taint
		}
	}
	for id, p := range pre {
		for _, e := range tl.Events {
			if e.Kind == PreToolUse && e.ToolUseID == id && (e.Denied || e.Missing) && post[id] > 0 {
				t.Fatalf("%q at %+v is denied or missing and has a Post", id, p)
			}
		}
	}
}
