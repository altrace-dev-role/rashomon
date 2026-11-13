package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestPost_UnitBriefLineSlotsAreTheDeliveredLines (debt row (m), a merge
// condition, ruling 07:09; PLAN's unit-brief clause): the note may say
// fewer lines than the file holds, and the steer line records what was
// said. The widest five-line knowledge brief fills the briefing cap under
// the graph head at c1, so at the first edit in a unit whose path is long
// the post says it stepped down, n of its five lines, n at least one. At
// both record scopes the line's key names the stored file by its digest,
// its model text says n lines (the note itself at v5), and its line slots
// name what was said: line1 to line<n> hold the kind and the id of the
// file's first n lines in order, and each slot past n is present with kind
// "" and no id (the built behaviour: the binding has five line slots and
// the steer line writes each). A reader that counts the slots with a kind
// reads n. Break: bind all five lines to the slots, and they name five
// where the note said n.
func TestPost_UnitBriefLineSlotsAreTheDeliveredLines(t *testing.T) {
	rev := graph.RevCommit("c1")
	shape := maxBriefShapes()["knowledge"]
	w := 0
	for x := 1; x <= 160 && engine.Rebind(rev, shape(x)).Kept == 5; x++ {
		w = x
	}
	if w == 0 || w == 160 {
		t.Fatalf("premise: no width fills the cap under the graph head (width %d)", w)
	}
	lines := shape(w)
	b := engine.Rebind(rev, lines)
	dir := strings.Repeat("abcdefghij/", 22) + "u"
	said, _, ok := b.UnitSaid(slot.RelPath(dir), nil)
	n := strings.Count(said.Text(), "\n")
	if !ok || n < 1 || n >= b.Kept {
		t.Fatalf("premise: in the long unit the brief says %d of %d lines (ok %v), want a step down", n, b.Kept, ok)
	}
	for _, scope := range []string{store.RecordScopeV4, store.RecordScopeV5} {
		t.Run(scope, func(t *testing.T) {
			s := ubPostSession(t, scope)
			rel := dir + "/x.go"
			unit := s.addUnit(rel)
			digest, err := unitBriefDigest(s.storeLines(unit, graph.GlobalID(planRID, unit), "c1", lines))
			if err != nil {
				t.Fatal(err)
			}
			out := s.editFile("toolu_ub_m", rel)
			ls := s.briefLines()
			if len(ls) != 1 {
				t.Fatalf("%d task-preflight lines, post output %q; want 1", len(ls), out)
			}
			l := ls[0]
			if !strings.HasSuffix(l.Key, "@"+digest) {
				t.Errorf("key %q, want the stored file's digest %s", l.Key, digest)
			}
			if got := strings.Count(l.ModelText, "\n"); got != n || scope == store.RecordScopeV5 && l.ModelText != said.Text() {
				t.Errorf("the line's model text says %d lines, want %d (the note at v5):\n%s", got, n, l.ModelText)
			}
			for k := 1; k <= 5; k++ {
				name := "line" + strconv.Itoa(k)
				raw, held := l.Slots[name]
				var got struct {
					Kind string `json:"kind"`
					ID   string `json:"id"`
				}
				if held {
					if err := json.Unmarshal(raw, &got); err != nil {
						t.Fatalf("%s: %v", name, err)
					}
				}
				kind, id := "", ""
				if k <= n {
					kind, id = b.Lines[k-1].Slot.Kind(), string(b.Lines[k-1].Key)
				}
				if !held || got.Kind != kind || got.ID != id {
					t.Errorf("slot %s %s (held %v), want held with kind %q and id %q", name, raw, held, kind, id)
				}
			}
		})
	}
}
