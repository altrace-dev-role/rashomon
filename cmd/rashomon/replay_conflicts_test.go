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

// TestLedger_ReplayFoldsConflictsAtThePoint: a conflict raised after the
// last Stop's checkpoint is not among the replayed rows. The replay folds
// the conflict_check records at the checkpoint's point_seq, the seq the live
// Stop folded at, so an edit of the next turn that raises protected_path
// under rule p1 stays out of the rows the Stop was shown. The controls are
// the record: the second conflict_check holds the one protected_path row
// under p1, and the fold of the whole record holds both conflicts, or the
// test would pass with the later one never recorded. The later edit's note
// is not read: the fixture's prompts share one prompt_id, so the note
// budget may withhold it, and the check is recorded before any delivery.
// Break: fold at the record's last seq, or at 0, in place of the point's
// seq.
func TestLedger_ReplayFoldsConflictsAtThePoint(t *testing.T) {
	s := cfSession(t, store.RecordScopeV5, true)
	s.prompt("fix " + cfGraph)
	if out := s.land("toolu_cf1", cfGraph, cfCross); !strings.Contains(out, cfNote) {
		t.Fatalf("post output %q lacks %q: the fixture raised no conflict", out, cfNote)
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

	s.prompt("fix " + cfKeep)
	s.land("toolu_cf2", cfKeep, "package keep\n\nfunc P() {}\n")
	after := s.run()
	if cs := after.ConflictChecks; len(cs) != 2 || len(cs[1].Rows) != 1 || cs[1].Rows[0].Kind != "protected_path" || cs[1].Rows[0].RuleID != "p1" {
		t.Fatalf("checks %+v, want a second check with the one protected_path row under p1: the later edit raised no conflict", cs)
	}
	if all := foldConflictChecks(after, 0); len(all) != 2 {
		t.Fatalf("fold of the whole record %+v, want the two conflicts", all)
	}

	r, err := replayLastStop(st, benchSessionID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Checkpoint == nil {
		t.Fatal("the replay read no checkpoint: the live Stop cut none")
	}
	if cs := after.ConflictChecks; len(cs) != 2 || cs[1].LandedSeq <= r.Checkpoint.PointSeq {
		t.Fatalf("checks %+v, want the second landed above point_seq %d", cs, r.Checkpoint.PointSeq)
	}
	if got := r.View.Conflicts; !reflect.DeepEqual(got, live) {
		t.Errorf("replayed conflicts %+v, want the live Stop's %+v", got, live)
	}
}
