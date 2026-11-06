package gopg

import (
	"context"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/platform"
)

// TestGopg_FactsDedupAndTiers (W-DEBT.6): one fact under two build configs
// is one edge; a model-proposed row is an inferred edge at the may-call
// confidence, never proven; a row at a tier this mapping does not know is
// no edge at all. The snapshot is the fixture's with three rows appended
// in memory, since the vouched files cannot be edited.
func TestGopg_FactsDedupAndTiers(t *testing.T) {
	s, err := platform.ReadSnapshot(context.Background(), shapes(t))
	if err != nil {
		t.Fatal(err)
	}
	const run, sum, area = "func:example.com/shapes/app.Run", "func:example.com/shapes/shape.Sum", "method:example.com/shapes/shape.Square.Area"
	s.Facts = append(s.Facts,
		platform.Fact{Subject: run, Predicate: platform.PredCalls, Object: sum, Tier: platform.TierProven, Config: "linux/amd64/cgo=1/tags="},
		platform.Fact{Subject: run, Predicate: platform.PredMayCall, Object: area, Tier: platform.TierProposed, Config: "x"},
		platform.Fact{Subject: sum, Predicate: platform.PredCalls, Object: area, Tier: "GUESSED", Config: "x"},
	)
	recs := newMapper(s, nil, graph.RevCommit("c1")).records()
	seen := map[string]int{}
	var proposed *graph.Edge
	for _, r := range recs {
		if r.Edge == nil {
			continue
		}
		seen[r.Edge.Key()]++
		if r.Edge.Src == "sym:go:app:func:Run" && r.Edge.Pred == graph.MayCall {
			e := *r.Edge
			proposed = &e
		}
	}
	if n := seen[key("sym:go:app:func:Run", graph.Calls, "sym:go:shape:func:Sum")]; n != 1 {
		t.Errorf("Run calls Sum under two configs = %d edges; want one", n)
	}
	if proposed == nil || proposed.Tier != graph.Inferred || proposed.Conf != mayCallConf {
		t.Errorf("a model-proposed may-call = %+v; want inferred at %.2f", proposed, mayCallConf)
	}
	if n := seen[key("sym:go:shape:func:Sum", graph.Calls, "sym:go:shape:method:Square.Area")]; n != 0 {
		t.Errorf("a row at an unknown tier made %d edge(s); want none", n)
	}
	for k, n := range seen {
		if n != 1 {
			t.Errorf("edge %q emitted %d times", k, n)
		}
	}
}
