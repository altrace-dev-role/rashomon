package steer_test

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/session"
	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// TestReqID_EqualsFoldKey: steer.ReqID is the id the session fold gives a
// requirement a delivered line states (session's require), so a Stop row's
// Req and a frontier's ReqState.ID match by equality, with no prefix to cut
// (W-M3.4). One delivered steer line states one requirement of
// each graph predicate; the fold's id of each must equal ReqID(Pred, Arg).
// The test sits outside both packages, so no file of the session package is
// edited for it.
func TestReqID_EqualsFoldKey(t *testing.T) {
	stated := []session.StatedReq{
		{Pred: steer.PredOpenedUnit, Arg: "internal/calc"},
		{Pred: steer.PredCalledSymbol, Arg: "internal/calc/add.go"},
		{Pred: steer.PredEditedOrVerified, Arg: "internal/calc/add.go"},
		{Pred: steer.PredPathsUntouched, Arg: "dec:k1"},
	}
	line := session.Event{Seq: 3, Steer: true, AtUnixMS: 1, ID: "line-1", Type: steer.LineType, State: string(steer.LineStateDelivered), Stated: stated}
	f := session.Fold("sess-1", []session.Event{line})
	if len(f.Reqs) != len(stated) {
		t.Fatalf("the fold kept %d Reqs, want %d: %+v", len(f.Reqs), len(stated), f.Reqs)
	}
	for _, r := range f.Reqs {
		if got := steer.ReqID(r.Pred, r.Arg); got != r.ID {
			t.Errorf("ReqID(%q, %q) = %q; the fold's id is %q", r.Pred, r.Arg, got, r.ID)
		}
	}
	// The predicate is part of the id: one path under two predicates is two
	// requirements, and a path holding a colon stays whole after the first.
	if steer.ReqID("file_exists", "a.go") == steer.ReqID("opened_unit", "a.go") {
		t.Error("ReqID drops the predicate")
	}
	if got := steer.ReqID("gate_green", "spec:unit"); got != "gate_green:spec:unit" {
		t.Errorf("ReqID(gate_green, spec:unit) = %q, want gate_green:spec:unit", got)
	}
}
