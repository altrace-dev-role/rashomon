package graph

import (
	"errors"
	"testing"
)

// TestObligation_NeverFromUncertainEdge: contracts-v2 section 3.3. The
// store sets Certain by one writer rule; a may_call edge at conf 0.6, an
// edge at an overlay revision, an inferred edge and a low-confidence edge
// are never certain; CheckBasis refuses every one of them and an empty
// basis, and admits a committed algorithmic calls edge at 0.9. The 11-04a
// records had no such field and projected may_call onto calls at 0.6.
func TestObligation_NeverFromUncertainEdge(t *testing.T) {
	s := NewStore()
	c1 := RevCommit("C1")
	sweep := SymID("py", "jobs", "func", "jobs.nightly.sweep")
	create := SymID("py", "services/auth", "func", "auth.sessions.create_session")
	handler := SymID("py", "services/auth", "func", "auth.api.create_session_handler")
	s.AssertEdge(Edge{Src: sweep, Pred: MayCall, Dst: create, Tier: Algorithmic, Conf: 0.6, Rev: c1})
	s.AssertEdge(Edge{Src: handler, Pred: Calls, Dst: create, Tier: Algorithmic, Conf: 0.9, Rev: c1})
	s.AssertEdge(Edge{Src: handler, Pred: Calls, Dst: sweep, Tier: Algorithmic, Conf: 0.9, Rev: RevOverlay("C1", 12)})
	s.AssertEdge(Edge{Src: sweep, Pred: Calls, Dst: handler, Tier: Inferred, Conf: 0.95, Rev: c1})
	s.AssertEdge(Edge{Src: create, Pred: Calls, Dst: handler, Tier: Algorithmic, Conf: 0.7, Rev: c1})
	s.AssertEdge(Edge{Src: create, Pred: Calls, Dst: sweep, Tier: Algorithmic, Conf: 0.9, Rev: c1, Certain: true, Status: Retracted})

	for _, tc := range []struct {
		name string
		ref  EdgeRef
		at   Rev
	}{
		{"may_call at 0.6", EdgeRef{sweep, MayCall, create}, c1},
		{"overlay revision", EdgeRef{handler, Calls, sweep}, RevOverlay("C1", 12)},
		{"inferred tier", EdgeRef{sweep, Calls, handler}, c1},
		{"conf under 0.8", EdgeRef{create, Calls, handler}, c1},
		{"retracted with certain handed in", EdgeRef{create, Calls, sweep}, c1},
		{"absent", EdgeRef{create, Calls, create}, c1},
	} {
		if e, ok := s.Edge(tc.ref.Src, tc.ref.Pred, tc.ref.Dst, tc.at); ok && e.Certain {
			t.Fatalf("%s: store marked certain: %+v", tc.name, e)
		}
		err := CheckBasis(s, tc.at, []EdgeRef{tc.ref})
		if !errors.Is(err, ErrUncertainBasis) {
			t.Fatalf("%s: CheckBasis = %v, want ErrUncertainBasis", tc.name, err)
		}
	}
	if err := CheckBasis(s, c1, nil); !errors.Is(err, ErrUncertainBasis) {
		t.Fatalf("empty basis = %v", err)
	}
	e, ok := s.Edge(handler, Calls, create, c1)
	if !ok || !e.Certain {
		t.Fatalf("committed algorithmic calls at 0.9 = %+v ok=%v, want certain", e, ok)
	}
	if err := CheckBasis(s, c1, []EdgeRef{{handler, Calls, create}}); err != nil {
		t.Fatalf("certain basis refused: %v", err)
	}
	if err := CheckBasis(s, c1, []EdgeRef{{handler, Calls, create}, {sweep, MayCall, create}}); !errors.Is(err, ErrUncertainBasis) {
		t.Fatalf("mixed basis = %v, want refusal on the uncertain edge", err)
	}
}
