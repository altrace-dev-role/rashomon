package engine

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// afterHead is text without its first line: a brief's lines.
func afterHead(text string) string {
	_, rest, _ := strings.Cut(text, "\n")
	return rest
}

// TestUnitSaid_NamesTheUnitWhereTheGraphHeadNamesTheRequest (W-M2.3 hook
// half, ruling 04:58 B): a unit brief said at the post is its kept lines,
// the ones deliver rendered under the graph head, under the unit's head,
// which names the unit and holds no request and no counts; with a digest,
// its record form is the same lines' record form under the record head,
// which names no unit. Without one there is no record form. Break: render
// the record through the plain row, and the record names the unit.
func TestUnitSaid_NamesTheUnitWhereTheGraphHeadNamesTheRequest(t *testing.T) {
	digest := func(string) string { return strings.Repeat("ef", 32) }
	b := wideConsumer(t)
	m, rec, ok := b.UnitSaid("store", digest)
	if !ok {
		t.Fatal("the unit brief said nothing")
	}
	head := "rashomon record: an edit in unit 'store', noted once a session; the repository graph at rev " + b.Rev.String() + " records for it:\n"
	if !strings.HasPrefix(m.Text(), head) || afterHead(m.Text()) != afterHead(b.Text()) {
		t.Errorf("said\n%s\nwant the head %q over the lines of\n%s", m.Text(), head, b.Text())
	}
	if strings.Contains(m.Text(), "request") || strings.Contains(m.Text(), "node(s)") {
		t.Errorf("the unit's note names the request or its counts: %q", m.Text())
	}
	want, err := b.Record(digest)
	if err != nil {
		t.Fatal(err)
	}
	recHead := "rashomon record: an edit in a unit on record, noted once a session; the repository graph at rev " + b.Rev.String() + " records for it:\n"
	if !strings.HasPrefix(rec.Text(), recHead) || afterHead(rec.Text()) != afterHead(want.Text()) {
		t.Errorf("record\n%s\nwant the head %q over the lines of\n%s", rec.Text(), recHead, want.Text())
	}
	if _, rec, ok := b.UnitSaid("store", nil); !ok || rec.Template() != "" {
		t.Errorf("without a digest: ok %v, record template %q, want a note and no record form", ok, rec.Template())
	}
}

// knowLines is five lesson lines, each keyed on its own id, whose rule is
// words words long.
func knowLines(words int) []Line {
	var out []Line
	for i := 1; i <= 5; i++ {
		id := slot.KnowledgeID("les:L" + string(rune('0'+i)))
		out = append(out, Line{Key: graph.ID(id), Slot: slot.NewKnowledgeLine(slot.KnowledgeFact{Kind: slot.KnowledgeLesson, ID: id,
			Path:   slot.RelPath("../" + strings.Repeat("abcdefghij/", 9) + "a.py"),
			Clause: slot.Approved(strings.Repeat("word ", words) + "end"), Source: slot.Path(strings.Repeat("docs/", 20) + "a.md")})})
	}
	return out
}

// TestUnitSaid_StepsALineDownUnderALongUnitHead: a brief whose five lines
// fill the briefing cap under the graph head (one more word each and
// deliver keeps fewer) says fewer lines, at least one, under the head of a
// unit whose path is long, as deliver steps a line down, rather than none.
// Witness: under a short unit path, whose head is shorter than the graph
// head, all five are said.
func TestUnitSaid_StepsALineDownUnderALongUnitHead(t *testing.T) {
	rev := graph.RevCommit("c1")
	words := 0
	for w := 31; w > 0; w-- {
		if Rebind(rev, knowLines(w)).Kept == 5 {
			words = w
			break
		}
	}
	if words == 0 || words == 31 {
		t.Fatalf("premise: no rule length fills the cap under the graph head (words %d)", words)
	}
	if n := Rebind(rev, knowLines(words+1)).Kept; n >= 5 {
		t.Fatalf("premise: one more word each still keeps %d lines, the cap does not bind", n)
	}
	b := Rebind(rev, knowLines(words))
	if m, _, ok := b.UnitSaid("u", nil); !ok || strings.Count(m.Text(), "\n") != 5 {
		t.Fatalf("witness: a short unit's note %q (ok %v), want all five lines", m.Text(), ok)
	}
	long := slot.RelPath(strings.Repeat("abcdefghij/", 22) + "u")
	m, _, ok := b.UnitSaid(long, nil)
	if n := strings.Count(m.Text(), "\n"); !ok || n < 1 || n > 4 {
		t.Errorf("a long unit's note keeps %d lines (ok %v), want one to four:\n%s", n, ok, m.Text())
	}
}

// TestUnitSaid_SaysNothingItCannotSayAsTheUnit: a brief whose only line is
// keyed on no node of the graph says nothing, as deliver sends nothing,
// and a unit path the slot refuses says nothing, never the brief under
// another head. Witness: the same line keyed on a node is said.
func TestUnitSaid_SaysNothingItCannotSayAsTheUnit(t *testing.T) {
	rev := graph.RevCommit("c1")
	nk := slot.NewNotKnownLine(slot.NotKnownFact{Unknown: []string{slot.UnknownUnit}})
	if _, _, ok := (Brief{Rev: rev, Lines: []Line{{Key: "dec:D1", Slot: nk}}, Kept: 1}).UnitSaid("store", nil); !ok {
		t.Fatal("witness: a keyed line was not said")
	}
	if m, _, ok := (Brief{Rev: rev, Lines: []Line{{Slot: nk}}, Kept: 1}).UnitSaid("store", nil); ok || m.Text() != "" {
		t.Errorf("a brief with no keyed line said %q", m.Text())
	}
	b := wideConsumer(t)
	if m, rec, ok := b.UnitSaid("/abs/store", func(string) string { return strings.Repeat("ef", 32) }); ok || m.Text() != "" || rec.Text() != "" {
		t.Errorf("a refused unit path said %q, record %q", m.Text(), rec.Text())
	}
}
