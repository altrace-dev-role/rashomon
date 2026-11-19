package graphtest

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang"
)

// NoOverlay is a lang.Resolver: the sixth method is part of the contract.
var _ lang.Resolver = NoOverlay{}

// TestNoOverlay_ClaimsFakeFilesAndHasNoOverlay: the s5 witness claims .fake
// files alone, and its Overlay and OverlayCarried are nil.
func TestNoOverlay_ClaimsFakeFilesAndHasNoOverlay(t *testing.T) {
	var r NoOverlay
	if !r.Claims("", "a/b.fake") || r.Claims("", "a/b.py") {
		t.Error("NoOverlay claims other than every .fake file")
	}
	if recs, err := r.Overlay("a/b.fake", []byte("x"), graph.RevCommit("C1"), 1); recs != nil || err != nil {
		t.Errorf("Overlay = %v, %v; want nil, nil", recs, err)
	}
	if c := r.OverlayCarried(); c != nil {
		t.Errorf("OverlayCarried = %v, want nil", c)
	}
	if u, ok := r.Unit("", "a/b.fake"); !ok || u != "unit:fake:a" {
		t.Errorf("Unit = %q, %v; want unit:fake:a", u, ok)
	}
}
