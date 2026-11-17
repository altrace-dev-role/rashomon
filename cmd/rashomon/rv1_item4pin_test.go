package main

import (
	"bytes"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// deliveredUnder is the notes the Stop reads as delivered under promptID
// (recap.DeliveredNotes over the session's steer lines, as stopView reads
// them).
func (s *planSession) deliveredUnder(promptID string) []recap.Note {
	s.t.Helper()
	st, err := store.OpenExisting(s.root)
	if err != nil {
		s.t.Fatal(err)
	}
	lines, _, err := st.ReadSteer(benchSessionID)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.t.Fatal(err)
	}
	return recap.DeliveredNotes(lines, promptID)
}

// promptFiles is every file the prompt side keeps for the session, by name:
// the unit briefs and the conflict base, all in the session's unit-brief
// directory.
func (s *planSession) promptFiles() map[string][]byte {
	s.t.Helper()
	dir := graph.UnitBriefDir(s.root, benchSessionID)
	ents, err := os.ReadDir(dir)
	if err != nil {
		s.t.Fatal(err)
	}
	out := make(map[string][]byte, len(ents))
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			s.t.Fatal(err)
		}
		out[e.Name()] = b
	}
	return out
}

// sameNotes is whether a and b are the same notes in the same order, by
// pattern, event and the text the model was given.
func sameNotes(a, b []recap.Note) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Pattern != b[i].Pattern || a[i].Event != b[i].Event || !bytes.Equal(a[i].ModelText, b[i].ModelText) {
			return false
		}
	}
	return true
}

// TestRV1_4pin_AFramedFireUnderATypedTurnReplacesNothing: one prompt id can
// carry more than one prompt-hook fire, each classed from its own payload. A
// typed fire naming a file, then a peer's message drained into the same turn
// (a framed fire under the same prompt id): the framed fire prints nothing
// and adds no steer line, every file the prompt side keeps for the session
// (the unit brief and the conflict base the typed fire wrote) is byte-equal
// after it, and the turn reads as typed at the edit and at the Stop: the
// turn's first edit in the unit prints what a twin session given the typed
// fire alone prints, and the notes read as delivered under the prompt id are
// the twin's, the typed fire's first. The twin carries the comparison: a
// typed fire naming a file a rule protects writes the conflict base, and at
// the edit the conflict outranks the unit brief by priority (post.go), so
// which note the edit prints is the typed turn's, not this test's, to say.
// Break: let a later fire under an id replace what an earlier one wrote for
// the turn.
func TestRV1_4pin_AFramedFireUnderATypedTurnReplacesNothing(t *testing.T) {
	const promptID = "prompt-1" // planSession.prompt's id, the same for every fire
	ask := "fix the reload in " + planHookHeld
	typedTurn := func() *planSession {
		s := ubPostSession(t, store.RecordScopeV5)
		s.prompt(ask)
		s.writeBrief("c1")
		return s
	}

	twin := typedTurn()
	want := twin.edit("toolu_pin_1")
	if want == "" {
		t.Fatalf("premise: the twin's first edit printed nothing, so an equal print would show nothing")
	}
	t.Logf("the twin's first edit printed %q", want)
	wantAt := twin.deliveredUnder(promptID)

	s := typedTurn()
	typed := s.deliveredUnder(promptID)
	lines := len(s.briefLines())
	files := s.promptFiles()
	t.Logf("the typed fire: %d delivered notes, %d task-preflight lines, %d prompt-side files", len(typed), lines, len(files))
	if len(typed) == 0 {
		t.Fatalf("premise: the typed fire delivered no note under %s, so a replacement could not be seen", promptID)
	}
	if _, err := os.Stat(conflictBasePath(s.root, benchSessionID, s.project)); err != nil {
		t.Fatalf("premise: the typed fire left no conflict base: %v", err)
	}
	if _, ok := files[filepath.Base(s.briefPath())]; !ok {
		t.Fatalf("premise: the unit brief is not among the prompt side's files")
	}

	if out := s.prompt(rv3Bare(ask)); out != "" {
		t.Errorf("the framed fire under %s printed %q, want nothing", promptID, out)
	}
	if n := len(s.briefLines()); n != lines {
		t.Errorf("%d task-preflight lines after the framed fire, want the typed fire's %d", n, lines)
	}
	if got := s.promptFiles(); !maps.EqualFunc(got, files, bytes.Equal) {
		t.Errorf("the prompt side's files after the framed fire differ from the typed fire's (%d files, want %d), want them byte-equal", len(got), len(files))
	}
	if out := s.edit("toolu_pin_1"); out != want {
		t.Errorf("the turn's first edit in the unit printed %q, want the typed-only twin's %q", out, want)
	}
	at := s.deliveredUnder(promptID)
	if !sameNotes(at, wantAt) {
		t.Errorf("at the Stop, %d notes delivered under %s differ from the typed-only twin's %d", len(at), promptID, len(wantAt))
	}
	if len(at) < len(typed) || !sameNotes(at[:len(typed)], typed) {
		t.Errorf("at the Stop, the notes delivered under %s do not open with the typed fire's %d unchanged", promptID, len(typed))
	}
}
