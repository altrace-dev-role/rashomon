package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestLedger_ReplayCarriesRuleConflicts: one session with one recorded
// conflict, a boundary_cross under rule b1 raised by a real post entry; the
// replay of its last Stop carries the same unmet conflict rows as the live
// Stop's view, so report --ledger shows the rows the session was shown. The
// control is the live side: one row, or the test would pass with both sides
// empty. Break: leave the replay's Stop input without the run's conflicts.
func TestLedger_ReplayCarriesRuleConflicts(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	s.prompt("fix " + cfGraph)
	if out := s.land("toolu_cf1", cfGraph, cfCross); !strings.Contains(out, cfNote) {
		t.Fatalf("post output %q lacks %q: the fixture raised no conflict", out, cfNote)
	}
	if cs := s.run().ConflictChecks; len(cs) != 1 || len(cs[0].Rows) != 1 {
		t.Fatalf("records %+v, want one check with one row", cs)
	}

	st, err := store.OpenExisting(s.root)
	if err != nil {
		t.Fatal(err)
	}
	run, err := digest.ReadRun(st, benchSessionID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1_800_000_000_000)
	d, err := digest.BuildFrom(run, st, benchSessionID, "", "", now)
	if err != nil {
		t.Fatal(err)
	}
	p := recapPayload{SessionID: benchSessionID, Harness: harnessSignal{HookEventName: install.EventStop}, OK: true, CWD: s.project}
	turn, err := stopView(st, run, d, p, false, now)
	if err != nil {
		t.Fatal(err)
	}
	live := turn.view.Conflicts
	if len(live) != 1 || live[0].Kind != "boundary_cross" {
		t.Fatalf("live Stop conflicts %+v, want the one boundary_cross row", live)
	}

	r, err := replayLastStop(st, benchSessionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.View.Conflicts; !reflect.DeepEqual(got, live) {
		t.Errorf("replayed conflicts %+v, want the live Stop's %+v", got, live)
	}
}
