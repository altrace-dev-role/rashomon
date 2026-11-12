package engine

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
)

// consumerOverlayRepo is a Go store unit with one function and one consumer
// file in another unit, both at the commit. With call set the consumer's
// function calls the store function at the commit. Either way the consumer
// file then gets the real Go overlay at seq 3, applied with the resolver's
// carried set ({Declares}), so the Calls edge is not carried and stands. The
// returned rev is the overlay revision the unit brief reads.
func consumerOverlayRepo(t *testing.T, call bool) (*graph.Store, graph.Rev) {
	t.Helper()
	c1 := graph.RevCommit(graphtest.Commit)
	prov := graph.Prov{Extractor: "synth", Version: "1"}
	g := graph.NewStore()
	node := func(id graph.ID, kind string) {
		g.AssertNode(graph.Node{ID: id, Kind: kind, Tier: graph.Proven, Conf: 1, Rev: c1, Prov: prov})
	}
	edge := func(src graph.ID, pred graph.Pred, dst graph.ID) {
		g.AssertEdge(graph.Edge{Src: src, Pred: pred, Dst: dst, Tier: graph.Proven, Conf: 1, Rev: c1, Prov: prov})
	}
	load := graph.SymID(gopg.Lang, "store", "func", "Load")
	brief := graph.SymID(gopg.Lang, "brief", "func", "Brief")
	for _, p := range [][3]string{{"store", "store/loader.go", string(load)}, {"brief", "brief/brief.go", string(brief)}} {
		unit, file := graph.UnitID(gopg.Lang, p[0]), graph.FileID(p[1])
		node(unit, "unit")
		node(file, "file")
		node(graph.ID(p[2]), "sym")
		edge(unit, graph.Contains, file)
		edge(file, graph.Declares, graph.ID(p[2]))
	}
	if call {
		edge(brief, graph.Calls, load)
	}
	src := []byte("package brief\n\nfunc Brief() { store.Load() }\n")
	rev := graph.RevOverlay(graphtest.Commit, 3)
	r := gopg.Resolver{}
	recs, err := r.Overlay("brief/brief.go", src, c1, 3)
	if err != nil || len(recs) == 0 {
		t.Fatalf("overlay: %v, %d records", err, len(recs))
	}
	g.ApplyOverlay(graph.FileID("brief/brief.go"), src, recs, rev, r.OverlayCarried())
	return g, rev
}

// TestUnitBrief_ConsumerFileWithOverlayStillListed: a consumer file of the
// unit that has an overlay at the reading revision (its Calls edges stand,
// the overlay carries only Declares) is still listed in the consumer group.
// The control removes the consumer's Calls edge at the commit: then it is
// not listed, so the listing above comes from that edge and not from the
// overlay's presence.
func TestUnitBrief_ConsumerFileWithOverlayStillListed(t *testing.T) {
	storeUnit := graph.UnitID(gopg.Lang, "store")
	g, rev := consumerOverlayRepo(t, true)
	b := UnitBrief(g, rev, storeUnit, Brief{}, Options{})
	if got, want := keysOf(b.Lines), []graph.ID{"file:brief/brief.go"}; !slices.Equal(got, want) {
		t.Fatalf("lines with the Calls edge and an overlay = %v, want %v", got, want)
	}
	if b.Lines[0].Role != RoleConsumer {
		t.Errorf("role %q, want %q", b.Lines[0].Role, RoleConsumer)
	}
	g, rev = consumerOverlayRepo(t, false)
	if b := UnitBrief(g, rev, storeUnit, Brief{}, Options{}); len(b.Lines) != 0 {
		t.Errorf("control: without the Calls edge the lines are %v, want none", keysOf(b.Lines))
	}
}
