package hook

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// These tests read callersMessage alone: the note's words for a function and
// its callers by their graph ids, with no store and no call.

const (
	noteHead  = "rashomon record: the repository graph names "
	noteTail  = ", which this call edited."
	staleTail = ", which this call edited; the graph does not hold the present content of the edited file."
	recHead   = "rashomon record: the repository graph names callers of the function this call edited: "
	staleRec  = "; the graph does not hold the present content of the edited file."
)

// noteSym is the symbol of the function named qualified in one unit,
// checked first to pass the identifier slot.
func noteSym(t *testing.T, qualified string) graph.ID {
	t.Helper()
	if err := slot.Ident(qualified).Validate(); err != nil {
		t.Fatalf("fixture: %q is no identifier: %v", qualified, err)
	}
	return graph.SymID(pylite.Lang, "unit", "func", qualified)
}

// shortCallers is n callers, mod.caller1 to mod.caller{n}.
func shortCallers(t *testing.T, n int) []graph.ID {
	t.Helper()
	var out []graph.ID
	for i := 1; i <= n; i++ {
		out = append(out, noteSym(t, "mod.caller"+strconv.Itoa(i)))
	}
	return out
}

// longName is segs segments of 64 copies of letter, joined by dots: at six
// segments 389 bytes, at eight the longest name the identifier slot takes.
// letter is never a hex digit, since the slot refuses a run of 64 hex
// digits as secret-shaped.
func longName(letter byte, segs int) string {
	return strings.Join(slices.Repeat([]string{strings.Repeat(string(letter), 64)}, segs), ".")
}

// failing is two ids that give no name: a file's, and a symbol's whose name
// the identifier slot refuses.
func failing(t *testing.T) []graph.ID {
	t.Helper()
	if slot.Ident("1abc").Validate() == nil {
		t.Fatalf("fixture: 1abc passes the identifier slot")
	}
	return []graph.ID{graph.FileID("mod/x.py"), graph.SymID(pylite.Lang, "unit", "func", "1abc")}
}

// wantNote fails the test unless callersMessage over fn and callers lists
// listed, says text and records rec.
func wantNote(t *testing.T, what string, fn graph.ID, callers []graph.ID, stale bool, listed []graph.ID, text, rec string) {
	t.Helper()
	m, r, got, ok := callersMessage(fn, callers, stale)
	if !ok {
		t.Fatalf("%s: no note, want %q", what, text)
	}
	if !slices.Equal(got, listed) || m.Text() != text || r.Text() != rec {
		t.Errorf("%s: listed %q, said %q and recorded %q; want %q, %q and %q", what, got, m.Text(), r.Text(), listed, text, rec)
	}
}

// TestCallersMessage_ListsFiveAndCountsTheRest: seven callers list the first
// five and count two; one caller is listed with no count. The record holds
// the two counts alone, and the stale record adds its clause.
func TestCallersMessage_ListsFiveAndCountsTheRest(t *testing.T) {
	fn, c := noteSym(t, "edited"), shortCallers(t, 7)
	wantNote(t, "seven callers", fn, c, false, c[:5],
		noteHead+"'mod.caller1', 'mod.caller2', 'mod.caller3', 'mod.caller4', 'mod.caller5' and 2 more as calling 'edited'"+noteTail,
		recHead+"5 listed, 2 more.")
	wantNote(t, "one caller", fn, c[:1], false, c[:1], noteHead+"'mod.caller1' as calling 'edited'"+noteTail, recHead+"1 listed, 0 more.")
	wantNote(t, "one caller, stale", fn, c[:1], true, c[:1], noteHead+"'mod.caller1' as calling 'edited'"+staleTail,
		recHead+"1 listed, 0 more"+staleRec)
}

// TestCallersMessage_DropsTheLastNameUntilTheNoteFits: seven callers whose
// names are 389 bytes each: three of them go over render's message cap, so
// the note drops its last listed name into the count until it fits, at two.
func TestCallersMessage_DropsTheLastNameUntilTheNoteFits(t *testing.T) {
	fn := noteSym(t, "edited")
	var c []graph.ID
	var q []string
	for i := 0; i < 7; i++ {
		n := longName(byte('g'+i), 6)
		c = append(c, noteSym(t, n))
		q = append(q, "'"+n+"'")
	}
	if three := noteHead + strings.Join(q[:3], ", ") + " and 4 more as calling 'edited'" + noteTail; len(three) <= render.MaxMessageBytes {
		t.Fatalf("fixture: three names come to %d bytes, within the cap of %d", len(three), render.MaxMessageBytes)
	}
	two := noteHead + strings.Join(q[:2], ", ") + " and 5 more as calling 'edited'"
	wantNote(t, "seven long names", fn, c, false, c[:2], two+noteTail, recHead+"2 listed, 5 more.")
	wantNote(t, "seven long names, stale", fn, c, true, c[:2], two+staleTail, recHead+"2 listed, 5 more"+staleRec)
}

// TestCallersMessage_ANameThatFailsIsCountedNotListed: a caller whose id
// gives no name, between two that do, is left out of the list and counted.
func TestCallersMessage_ANameThatFailsIsCountedNotListed(t *testing.T) {
	fn, c := noteSym(t, "edited"), shortCallers(t, 2)
	for _, bad := range failing(t) {
		wantNote(t, string(bad), fn, []graph.ID{c[0], bad, c[1]}, false, c,
			noteHead+"'mod.caller1', 'mod.caller2' and 1 more as calling 'edited'"+noteTail, recHead+"2 listed, 1 more.")
	}
}

// TestCallersMessage_NoNameNoNote: no note where no caller is given, where
// no caller gives a name, or where the function gives none.
func TestCallersMessage_NoNameNoNote(t *testing.T) {
	fn, c, bad := noteSym(t, "edited"), shortCallers(t, 1), failing(t)
	for _, tc := range []struct {
		what    string
		fn      graph.ID
		callers []graph.ID
	}{
		{"no caller", fn, nil},
		{"no caller gives a name", fn, bad},
		{"a function that is a file", bad[0], c},
		{"a function name the slot refuses", bad[1], c},
	} {
		if _, _, listed, ok := callersMessage(tc.fn, tc.callers, false); ok {
			t.Errorf("%s: a note listing %q, want none", tc.what, listed)
		}
	}
}

// TestCallersMessage_OneNameThatDoesNotFitSaysNothing: the function and its
// one named caller each carry the longest name the identifier slot takes,
// beside a thousand callers that give no name. The note fits; the stale
// note, its clause added, goes over render's cap with one name left, so it
// says nothing.
func TestCallersMessage_OneNameThatDoesNotFitSaysNothing(t *testing.T) {
	fnName, callerName := longName('p', 8), longName('q', 8)
	fn, caller := noteSym(t, fnName), noteSym(t, callerName)
	callers := append([]graph.ID{caller}, slices.Repeat([]graph.ID{graph.FileID("mod/x.py")}, 1000)...)
	said := noteHead + "'" + callerName + "' and 1000 more as calling '" + fnName + "'"
	if len(said+noteTail) > render.MaxMessageBytes || len(said+staleTail) <= render.MaxMessageBytes {
		t.Fatalf("fixture: the note is %d bytes and the stale one %d, want them either side of %d",
			len(said+noteTail), len(said+staleTail), render.MaxMessageBytes)
	}
	wantNote(t, "the note", fn, callers, false, callers[:1], said+noteTail, recHead+"1 listed, 1000 more.")
	if _, _, listed, ok := callersMessage(fn, callers, true); ok {
		t.Errorf("the stale note listed %q, want no note", listed)
	}
}
