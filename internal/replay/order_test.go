package replay

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/history"
)

func checkTimeline(t *testing.T, d *doc, tl Timeline) {
	t.Helper()
	g, w := got(tl), d.expected()
	if show(g) != show(w) {
		t.Fatalf("timeline\n  %s\nwant\n  %s", show(g), show(w))
	}
	for i := 1; i < len(tl.Events); i++ {
		if !tl.Events[i-1].Pos.Before(tl.Events[i].Pos) {
			t.Fatalf("events %d and %d are out of file order: %+v then %+v", i-1, i, tl.Events[i-1].Pos, tl.Events[i].Pos)
		}
	}
	for _, e := range tl.Events {
		if !e.Supported() {
			t.Errorf("%s %s: unsupported %v", e.Kind, e.ToolUseID, e.Unsupported)
		}
	}
}

// TestREPLAY2_OrderAroundAWrite is ORDER-1 at the timeline: calls overlap,
// and a result can come before the result of a call used earlier. A test
// run (a) is used first and answered last, after a Write (b) was used and
// answered, so that the write's execution is before the run's: at the
// Write's Post a point sees the run's declaration and not yet its execution.
// Every event sits at its own position, whatever order the calls were
// emitted in.
func TestREPLAY2_OrderAroundAWrite(t *testing.T) {
	d := newDoc()
	d.promptLine("p1")
	d.uses("m1", bashUse("a", "go test ./..."), writeUse("b", "/repo/f.go")) // one line, blocks 0 and 1
	d.results("", okResult("b"))                                             // b is answered first
	d.uses("m2", bashUse("c", "go vet ./..."))                               // c is used after b's result
	d.results("", okResult("a"))                                             // a is answered after the write, c's use
	d.results("", failResult("c"))
	d.ends("m3", "done")
	tl := d.build(t, history.DefaultLimits)
	checkTimeline(t, d, tl)

	// The positions in words: Pre a, Pre b, Post b, Pre c, Post a, Post c.
	want := []string{"UserPromptSubmit", "PreToolUse a", "PreToolUse b", "PostToolUse b", "PreToolUse c", "PostToolUse a", "PostToolUseFailure c", "Stop"}
	var have []string
	for _, e := range tl.Events {
		have = append(have, strings.TrimSpace(e.Kind.String()+" "+e.ToolUseID))
	}
	if strings.Join(have, ", ") != strings.Join(want, ", ") {
		t.Errorf("order %v, want %v", have, want)
	}
	// a was emitted before b and c by history, and a's result is after b's:
	// the order is the file's, not the calls'.
	pos := map[string]history.Pos{}
	for _, e := range tl.Events {
		pos[e.Kind.String()+e.ToolUseID] = e.Pos
	}
	if !pos["PostToolUseb"].Before(pos["PostToolUsea"]) || !pos["PreToolUsea"].Before(pos["PreToolUseb"]) {
		t.Errorf("a is used before b and b is answered before a: %+v", pos)
	}
}

// TestREPLAY2_OrderAroundATurnEnd: results on either side of a turn's end. A
// call answered before the end is a Post before the Stop; one answered after
// it, on a line after the last assistant line, is a Post after the Stop, and
// the Stop's predecessors are exactly the events on earlier lines. A result
// that comes after the next prompt too is after that prompt's event, and
// carries the promptId its own line names.
func TestREPLAY2_OrderAroundATurnEnd(t *testing.T) {
	d := newDoc()
	p1 := d.promptLine("p1")
	d.uses("m1", bashUse("a", "make test"), bashUse("b", "make lint"), bashUse("z", "sleep 99"))
	d.results("", okResult("a"))
	stop1 := d.ends("m2", "all green") // a Stop; b and z are unanswered
	d.results("", okResult("b"))       // late: after the Stop
	p2 := d.promptLine("p2")
	d.results("", failResult("z")) // later still: after the next prompt, with p2 as its promptId
	d.ends("m3", "second")
	tl := d.build(t, history.DefaultLimits)
	checkTimeline(t, d, tl)

	var stops []Event
	for _, e := range tl.Events {
		if e.Kind == Stop {
			stops = append(stops, e)
		}
	}
	if len(stops) != 2 || stops[0].Pos != (history.Pos{Line: stop1, Block: 1}) {
		t.Fatalf("stops = %+v", stops)
	}
	// A Stop's preceding events are exactly the ones on earlier lines of the
	// file, whatever the calls they belong to.
	for _, s := range stops {
		for _, e := range tl.Events {
			before := e.Pos.Line < s.Pos.Line
			placed := e.Pos.Before(s.Pos)
			if before != placed {
				t.Errorf("%s %s at %+v against the Stop at %+v: before the line %v, before the Stop %v", e.Kind, e.ToolUseID, e.Pos, s.Pos, before, placed)
			}
		}
	}
	ev := func(kind Kind, id string) (Event, int) {
		for i, e := range tl.Events {
			if e.Kind == kind && e.ToolUseID == id {
				return e, i
			}
		}
		t.Fatalf("no %s %s", kind, id)
		return Event{}, 0
	}
	_, iStop := ev(Stop, "")
	_, iPostB := ev(PostToolUse, "b")
	if iPostB < iStop {
		t.Errorf("b's result is on a line after the Stop and its Post is before it")
	}
	if _, iPostA := ev(PostToolUse, "a"); iPostA > iStop {
		t.Errorf("a's result is on a line before the Stop and its Post is after it")
	}
	z, iZ := ev(PostToolUseFailure, "z")
	if !(iZ > iStop) || z.Pos.Line <= p2 || !strings.Contains(string(z.Payload), `"prompt_id":"p2"`) {
		t.Errorf("z's result is after the next prompt (line %d), carries p2, and is after the Stop: %+v %s", p2, z.Pos, z.Payload)
	}
	// And its Pre, used in turn one, names turn one's prompt.
	pre, _ := ev(PreToolUse, "z")
	if !strings.Contains(string(pre.Payload), `"prompt_id":"p1"`) || pre.Pos.Line <= p1 {
		t.Errorf("z's declaration: %s", pre.Payload)
	}
}

// TestREPLAY2_OrderIsTheFilesNotTheCalls: a larger, shuffled case. Twenty
// calls are used across four lines and answered in a fixed shuffle spread
// over the lines after them, a turn end in the middle; each event is where
// the builder put it.
func TestREPLAY2_OrderIsTheFilesNotTheCalls(t *testing.T) {
	d := newDoc()
	d.promptLine("p1")
	var ids []string
	for l := 0; l < 4; l++ {
		var us []toolUse
		for i := 0; i < 5; i++ {
			id := string(rune('a'+l)) + string(rune('0'+i))
			ids = append(ids, id)
			us = append(us, bashUse(id, "cmd "+id))
		}
		d.uses("m"+string(rune('0'+l)), us...)
	}
	// Answer in a stride-7 permutation of the twenty, ending the turn after
	// the eleventh result.
	for k := 0; k < len(ids); k++ {
		d.results("", okResult(ids[(k*7)%len(ids)]))
		if k == 10 {
			d.ends("mx", "midway")
		}
	}
	tl := d.build(t, history.DefaultLimits)
	checkTimeline(t, d, tl)
	if len(tl.Events) != 1+20+20+1 {
		t.Errorf("%d events", len(tl.Events))
	}
}

// TestREPLAY2_DenialIsNotMissing: a confirmed denial has a PreToolUse and no
// Post and is Denied; a call whose result was never read has a PreToolUse and
// no Post and is Missing. The two are distinguishable on the event, and
// neither is a gap: both are exactly what a live run recorded.
func TestREPLAY2_DenialIsNotMissing(t *testing.T) {
	d := newDoc()
	d.promptLine("p1")
	d.uses("m1", bashUse("d", "rm -rf build"), bashUse("m", "sleep 1"), bashUse("k", "ls"))
	d.results("", denyResult("d"))
	d.results("", okResult("k"))
	d.ends("m2", "done")
	tl := d.build(t, history.DefaultLimits)
	checkTimeline(t, d, tl)
	flags := map[string][2]bool{}
	for _, e := range tl.Events {
		if e.Kind == PreToolUse {
			flags[e.ToolUseID] = [2]bool{e.Denied, e.Missing}
			if e.Gap != "" || !e.Taint.IsZero() {
				t.Errorf("%s: gap %q taint %+v", e.ToolUseID, e.Gap, e.Taint)
			}
		}
		if e.Denied && e.Missing {
			t.Errorf("%s is both", e.ToolUseID)
		}
	}
	want := map[string][2]bool{"d": {true, false}, "m": {false, true}, "k": {false, false}}
	for id, w := range want {
		if flags[id] != w {
			t.Errorf("%s: denied/missing = %v, want %v", id, flags[id], w)
		}
	}
	for _, e := range tl.Events {
		if (e.Kind == PostToolUse || e.Kind == PostToolUseFailure) && (e.ToolUseID == "d" || e.ToolUseID == "m") {
			t.Errorf("a Post for %s, which has no execution", e.ToolUseID)
		}
	}
}

// TestREPLAY2_WalkStreams: Walk yields in file order with only what cannot yet
// be placed held back, and stops when yield says so.
func TestREPLAY2_WalkStreams(t *testing.T) {
	d := newDoc()
	d.promptLine("p1")
	d.uses("m1", bashUse("a", "x"), bashUse("b", "y"))
	d.results("", okResult("b"))
	d.results("", okResult("a"))
	d.ends("m2", "done")
	var n int
	_, err := Walk(d.reader(), Options{TranscriptPath: synthPath}, func(e Event) error {
		n++
		if n == 3 {
			return ErrStop
		}
		return nil
	})
	if err != nil || n != 3 {
		t.Errorf("stopped after %d events with %v", n, err)
	}
}
