package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// lastIndexLineOf is the line for edits whose claiming reader builds no
// overlay, the words what and the commit rev of the index whose facts stand,
// spelled out as kindLineOf is.
func lastIndexLineOf(what, rev string) string {
	return "rashomon: edits to " + what + " files build no new facts here; their unit, caller and conflict facts are those of the last index (" + rev + ")."
}

// withNoOverlay registers graphtest.NoOverlay, indexed, after this build's
// readers in the registry the overlay writer builds, for the test's length.
func withNoOverlay(t *testing.T) {
	t.Helper()
	old := overlayWire
	overlayWire = func() *lang.Registry {
		reg := index.Wire()
		reg.RegisterIndexed(graphtest.NoOverlay{}, func(string) (lang.Resolver, error) { return graphtest.NoOverlay{}, nil })
		return reg
	}
	t.Cleanup(func() { overlayWire = old })
}

// TestOverlayNilSaysLastIndex: with graphtest.NoOverlay registered after
// this build's readers, under an index whose header names that language's
// producer at commit c2, an edit of a .fake file is claimed and indexed and
// its reader's Overlay returns nil with no error; the post says, as a system
// message alone, that the file's facts are those of the last index, naming
// the header's commit, and keeps no overlay group. A second .fake file in the
// session says nothing, and a second session says the line again. Break: stay
// silent where Overlay returns nothing (the first edit says nothing), key the
// marker by the file (the second file repeats), keep it store-wide (the second
// session is silent), or name a commit other than the header's.
func TestOverlayNilSaysLastIndex(t *testing.T) {
	f := newOvFix(t, "1\n")
	const rev = "c2"
	hdr := graph.Header{Repo: "r", Rev: graph.HeaderRev{Commit: rev},
		Producers: map[string]string{graphtest.NoOverlayLang: graphtest.NoOverlayLang + "/v0"}}
	if err := graph.SaveFile(f.repo, graph.NewStore(), hdr); err != nil {
		t.Fatal(err)
	}
	withNoOverlay(t)
	want := lastIndexLineOf(".fake", rev)
	if got := kindSaid(t, f.kindPost(ovSess, "toolu_nf1", "inv/a.fake")); got != want {
		t.Errorf("the first .fake edit said %q; want %q", got, want)
	}
	if b := f.kindPost(ovSess, "toolu_nf2", "inv/b.fake"); b != nil {
		t.Errorf("a second .fake file in the session printed %q; want nothing", b)
	}
	if got := kindSaid(t, f.kindPost("sess-ov-2", "toolu_nf3", "inv/a.fake")); got != want {
		t.Errorf("a second session said %q; want %q", got, want)
	}
	if groups, _, _ := f.read(); len(groups) != 0 {
		t.Errorf("the .fake edits kept %d overlay groups; want none", len(groups))
	}
}

// TestOverlayNilKeepsTheIndexFacts: under the same index, an edit of a .fake
// file calls no Overlay, at the declaration or after the tool, keeps no
// group for the conflict check, and passes the gate to the post's unit-brief
// read, whose unit is the reader's for the file, so the facts the line names
// are the ones read. Break: clear the gate where the reader has no overlay
// (no landed edit), keep a group, or call the reader's Overlay first.
func TestOverlayNilKeepsTheIndexFacts(t *testing.T) {
	f := newOvFix(t, "1\n")
	hdr := graph.Header{Repo: "r", Rev: graph.HeaderRev{Commit: "c2"},
		Producers: map[string]string{graphtest.NoOverlayLang: graphtest.NoOverlayLang + "/v0"}}
	if err := graph.SaveFile(f.repo, graph.NewStore(), hdr); err != nil {
		t.Fatal(err)
	}
	withNoOverlay(t)
	calls := 0
	old := overlayResolve
	overlayResolve = func(r lang.Resolver, rel string, content []byte, base graph.Rev, seq int64) ([]graph.Record, error) {
		calls++
		return r.Overlay(rel, content, base, seq)
	}
	t.Cleanup(func() { overlayResolve = old })
	file := filepath.Join(f.repo, "inv", "a.fake")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"file_path": file, "old_string": "x", "new_string": "y"}
	f.pre("toolu_nk1", "Edit", input)
	p := NewPost(f.st, time.Now)
	p.Close(&Signals{}, p.Capture(strings.NewReader(f.payload("PostToolUse", "toolu_nk1", "Edit", input))))
	if calls != 0 {
		t.Errorf("the .fake edit called Overlay %d times; want none", calls)
	}
	if _, kept := p.LandedOverlay(); kept {
		t.Error("the .fake edit kept a group for the conflict check; want none")
	}
	e, ok := p.LandedGoEdit()
	if !ok || e.Rel != "inv/a.fake" {
		t.Fatalf("the unit-brief read takes %+v, %v; want the edit of inv/a.fake", e, ok)
	}
	if u, ok := e.Unit(); !ok || u != graph.UnitID(graphtest.NoOverlayLang, "inv") {
		t.Errorf("the edit's unit is %q, %v; want the reader's unit for inv", u, ok)
	}
}

// TestOverlayNil_AnUnprintableKindNamesTheLanguage: where the edited file's
// kind does not print (printableKind), the line names the language's files
// and the gate's commit, and the marker is the language's. Break: print the
// key raw, or take the unindexed family's marker.
func TestOverlayNil_AnUnprintableKindNamesTheLanguage(t *testing.T) {
	st := mustOpen(t)
	if err := os.MkdirAll(st.RunDir("s1"), 0o700); err != nil {
		t.Fatal(err)
	}
	var reg lang.Registry
	p := &Post{st: st, sessionID: "s1", event: "PostToolUse",
		gate: keptGate{rel: "a/b.f\x01k", reg: &reg, ok: true, lastIndex: "fake", lastRev: "c9"}}
	line, marker := p.takeKindLine()
	if want := lastIndexLineOf("fake", "c9"); line != want {
		t.Errorf("the line reads %q; want %q", line, want)
	}
	if want := store.LastIndexNoticeName("fake"); marker != want {
		t.Errorf("the marker taken is %q; want the language's %q", marker, want)
	}
}
