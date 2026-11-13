package engine

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// wideConsumer is the unit brief of a store unit with a decision and a
// lesson, whose one consumer file declares five functions that all call
// into it, so the consumer line lists past its cap and bind leaves counts
// in its slot.
func wideConsumer(t *testing.T) Brief {
	t.Helper()
	var syms []synSym
	var calls [][2]string
	for _, n := range []string{"b1", "b2", "b3", "b4", "b5"} {
		syms = append(syms, fn(n))
		calls = append(calls, [2]string{"brief/brief.py#" + n, "store/loader.py#load_rows"})
	}
	g := synth(t, []synFile{
		{"store", "store/loader.py", []synSym{fn("load_rows")}},
		{"brief", "brief/brief.py", syms},
	}, calls, nil)
	record(t, g, "dec:D1", "decision", "store/loader.py")
	record(t, g, "les:L1", "lesson", "store/loader.py")
	b := UnitBrief(g, unitRev, storeUnit, Brief{}, Options{})
	if b.Empty() {
		t.Fatal("the fixture's unit brief is empty")
	}
	return b
}

// stored is l as a reader of the unit-brief file has it: the key, and the
// slot rebuilt from its bound fact with its counts restored (slot D10).
func stored(l Line) Line {
	s := l.Slot
	var fresh slot.GraphLine
	if f, ok := s.Code(); ok {
		fresh = slot.NewCodeLine(f)
	} else if f, ok := s.Table(); ok {
		fresh = slot.NewTableLine(f)
	} else if f, ok := s.Knowledge(); ok {
		fresh = slot.NewKnowledgeLine(f)
	} else if f, ok := s.Unit(); ok {
		fresh = slot.NewUnitLine(f)
	} else if f, ok := s.NotKnown(); ok {
		fresh = slot.NewNotKnownLine(f)
	}
	return Line{Key: l.Key, Slot: fresh.WithMore(s.More())}
}

// TestRebind_RendersTheUnitBriefsMessage (W-M2.3 hook half, D11): the unit
// brief's kept lines, stored and read back as their keys and rebuilt
// slots, render through Rebind to the message and the record form
// UnitBrief made, byte for byte, by the same bind and deliver. Witness: a
// line left out renders a different message, so the comparison reads the
// lines. Break: make Rebind skip deliver, and its Message is empty.
func TestRebind_RendersTheUnitBriefsMessage(t *testing.T) {
	digest := func(s string) string { return strings.Repeat("ef", 32) }
	b := wideConsumer(t)
	kept := b.Lines[:b.Kept]
	cut := false
	for _, l := range kept {
		cut = cut || l.Slot.More() != (slot.Counts{})
	}
	if !cut {
		t.Fatalf("the fixture's premise: no kept line carries a count bind left:\n%s", b.Text())
	}
	lines := make([]Line, len(kept))
	for i, l := range kept {
		lines[i] = stored(l)
	}
	r := Rebind(b.Rev, lines)
	if r.Kept != b.Kept {
		t.Fatalf("rebind kept %d lines, want %d", r.Kept, b.Kept)
	}
	if got, want := r.Message.Text(), b.Message.Text(); got != want {
		t.Errorf("message\n got %q\nwant %q", got, want)
	}
	gotRec, err := r.Record(digest)
	if err != nil {
		t.Fatal(err)
	}
	wantRec, err := b.Record(digest)
	if err != nil {
		t.Fatal(err)
	}
	if gotRec.Text() != wantRec.Text() {
		t.Errorf("record\n got %q\nwant %q", gotRec.Text(), wantRec.Text())
	}
	if short := Rebind(b.Rev, lines[1:]); short.Message.Text() == b.Message.Text() {
		t.Error("witness: a brief with a line left out renders the same message")
	}
}
