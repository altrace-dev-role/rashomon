package hook

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/graph/index"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
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
