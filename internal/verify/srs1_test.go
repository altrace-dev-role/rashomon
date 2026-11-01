package verify

import (
	"slices"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestSRS1e2_TheReplayKeepsABindingViolationRed (card SPEC-RULES-STOP1, #173
// F4): a trusted protect_path rule violated by an edit, then a green
// required gate. The live Stop (no pins) and the replay (the pins laststop
// fills from the checkpoint's gates) both read record_green false: the two
// count the same thing.
func TestSRS1e2_TheReplayKeepsABindingViolationRed(t *testing.T) {
	sp := ruleSpec(spec.Trusted, frozen(allModes...))
	r := runOf(t, edit(1, "db/migrations/001.sql"), gateCall(2, "go-test", store.ExecOK))
	pt := stopPoint(r)
	live := EvaluateStop(r, nil, sp, pt, time.Time{}, StopInput{})
	if v := oneVerdict(t, live.Snap); v.Verdict != Violation {
		t.Fatalf("premise: verdict %+v, want a violation", v)
	}
	pins := map[string]bool{}
	for _, g := range live.Snap.Gates {
		if !g.Required || g.State != Green {
			t.Fatalf("premise: gate %+v, want required and green", g)
		}
		pins[g.Gate] = g.Required
	}
	if len(pins) == 0 {
		t.Fatal("premise: no gate")
	}
	replay := EvaluateStop(r, nil, sp, pt, time.Time{}, StopInput{Required: pins})
	if live.Snap.RecordGreen || replay.Snap.RecordGreen {
		t.Errorf("record_green live %v, replay %v; want both false over a binding violation", live.Snap.RecordGreen, replay.Snap.RecordGreen)
	}
}

// TestSRS1n_EveryRowThatPrintsCarriesItsFindings (card SPEC-RULES-STOP1,
// D-f; QA-2's beat): a trusted protect_path violation in a project with no
// gate reads no_gate, and its Summary carries k = 1, whose count clause the
// line ends with. Every row that prints carries k, the incomplete-recording
// row too; k = 0 adds no clause; the findings row's Sentence is the clause,
// so FindingsClause adds none there; and the row with no line is returned
// before k is counted.
func TestSRS1n_EveryRowThatPrintsCarriesItsFindings(t *testing.T) {
	const clause = "1 record finding(s) in this turn"
	sp := ruleSpec(spec.Trusted, frozen(allModes...))
	r := runOf(t, edit(1, "db/migrations/001.sql"))
	live := EvaluateStop(r, nil, sp, stopPoint(r), time.Time{}, StopInput{})
	if v := oneVerdict(t, live.Snap); v.Verdict != Violation || len(live.Snap.Gates) != 0 {
		t.Fatalf("premise: verdict %+v and gates %+v, want a violation and no gate", v, live.Snap.Gates)
	}
	// The fixture's calls carry no prompt id, so the Stop's own turn window
	// is empty; the turn is given as the hook's would count it.
	if s := TurnSummary(live.Snap, TurnCounts{Writes: 1, PathsWritten: 1}); s.Row != RowNoGate || s.Findings != 1 || s.FindingsClause() != clause {
		t.Errorf("summary %+v with clause %q, want no_gate, k 1 and %q", s, s.FindingsClause(), clause)
	}

	unresolved := Integrity{Unresolved: []IntegrityEntry{{Check: TestRemoved, Seq: 1}}}
	for _, c := range []struct {
		name string
		snap Snapshot
		turn TurnCounts
		row  Row
		want string
	}{
		{"incomplete", Snapshot{Recording: Incomplete, Integrity: unresolved}, TurnCounts{Writes: 1}, RowUnknown, clause},
		{"no finding", Snapshot{Recording: Complete}, TurnCounts{Writes: 1}, RowNoGate, ""},
		{"no line", Snapshot{Recording: Complete, Integrity: unresolved}, TurnCounts{}, RowNone, ""},
	} {
		if s := TurnSummary(c.snap, c.turn); s.Row != c.row || s.FindingsClause() != c.want {
			t.Errorf("%s: row %q clause %q, want %q and %q", c.name, s.Row, s.FindingsClause(), c.row, c.want)
		}
	}
	if s := (Summary{Row: RowFindings, Sentence: clause, Findings: 1}); s.FindingsClause() != "" {
		t.Errorf("the findings row adds clause %q to its own", s.FindingsClause())
	}
}

// hitOf is the rule_hit the post writes for call n of r, under digest.
func hitOf(t *testing.T, r *store.Run, n int64, id, digest string) store.SpecEvent {
	t.Helper()
	x := execOf(r, n)
	if x == nil || x.Seq == nil {
		t.Fatalf("premise: call %d has no landed execution", n)
	}
	return *store.NewRuleHit("s1", postEvent, id, digest, *x.Seq, 0)
}

// TestSRS1_ARuleHitIsAViolationAtStop (card SPEC-RULES-STOP1, #173 F1): at
// record scope v4 no line is recorded, so the lines read undecided; the
// post's rule_hit, under the rule's source digest, makes the verdict a
// violation citing the call's declaration seq, the ledger row unmet, and
// record-green false. Without the hit the verdict stays undecided, never
// clear (TestSRS1c_).
func TestSRS1_ARuleHitIsAViolationAtStop(t *testing.T) {
	sp := ruleSpec(spec.Trusted, noForce())
	r := runOf(t, edit(1, "a.go"), sh(2, "git push --force origin main"), gateCall(3, "go-test", store.ExecOK))
	r.SpecEvents = append(r.SpecEvents, hitOf(t, r, 2, "spec:no-force", sp.Sources[0].Digest))
	snap, obs := specLedgerAt(r, sp, stopPoint(r))
	bad := declOf(r, 2).Seq
	if v := oneVerdict(t, snap); v.Verdict != Violation || !slices.Equal(v.Seqs, []int64{bad}) {
		t.Fatalf("verdict %+v, want a violation citing declaration %d", v, bad)
	}
	if rows := specRows(obs); len(rows) != 1 || rows[0].State != ObligationUnmet || !slices.Equal(rows[0].Evidence, []int64{bad}) {
		t.Fatalf("rows %+v, want one unmet citing %d", rows, bad)
	}
	if snap.RecordGreen {
		t.Error("record_green true over a binding rule's hit")
	}
}

// TestSRS1c_AnUnmatchedCallStaysUndecided: no hit, v4 lines: undecided,
// never no_recorded_violation.
func TestSRS1c_AnUnmatchedCallStaysUndecided(t *testing.T) {
	sp := ruleSpec(spec.Trusted, noForce())
	r := runOf(t, sh(1, "git push origin main"))
	snap, _ := specLedgerAt(r, sp, stopPoint(r))
	if v := oneVerdict(t, snap); v.Verdict != RuleUnknown {
		t.Fatalf("verdict %+v, want unknown", v)
	}
}

// TestSRS1d_AHitCountsOnlyUnderItsSourceAndWindow: a hit recorded under
// another digest of the declaring file (an edit since), of another rule id,
// or whose call landed after the point, counts nothing; one recorded under a
// managed source's digest is unaffected by the repository source's; the
// merged spec digest is not a source digest; a hit whose execution is not in
// the run cites its landed seq.
func TestSRS1d_AHitCountsOnlyUnderItsSourceAndWindow(t *testing.T) {
	sp := ruleSpec(spec.Trusted, noForce())
	r := runOf(t, sh(1, "git push --force origin main"), sh(2, "git status"))
	pt := stopPoint(r)
	landed := *execOf(r, 1).Seq
	for _, c := range []struct {
		name string
		hit  store.SpecEvent
		want RuleVerdictValue
	}{
		{"edited source", *store.NewRuleHit("s1", postEvent, "spec:no-force", spec.DigestPrefix+"00", landed, 0), RuleUnknown},
		{"merged digest", *store.NewRuleHit("s1", postEvent, "spec:no-force", sp.Digest, landed, 0), RuleUnknown},
		{"other rule", *store.NewRuleHit("s1", postEvent, "spec:other", sp.Sources[0].Digest, landed, 0), RuleUnknown},
		{"after the point", *store.NewRuleHit("s1", postEvent, "spec:no-force", sp.Sources[0].Digest, pt.Seq+1, 0), RuleUnknown},
		{"other reason", func() store.SpecEvent {
			h := *store.NewRuleHit("s1", postEvent, "spec:no-force", sp.Sources[0].Digest, landed, 0)
			h.Reason = "spec_edit"
			return h
		}(), RuleUnknown},
		{"in window", *store.NewRuleHit("s1", postEvent, "spec:no-force", sp.Sources[0].Digest, landed, 0), Violation},
	} {
		rr := *r
		rr.SpecEvents = []store.SpecEvent{c.hit}
		snap, _ := specLedgerAt(&rr, sp, pt)
		if v := oneVerdict(t, snap); v.Verdict != c.want {
			t.Errorf("%s: verdict %+v, want %s", c.name, v, c.want)
		}
	}

	managed := ruleSpec(spec.Trusted, noForce())
	managed.Sources = append([]spec.Source{{Path: "/managed/spec.json", Digest: spec.DigestPrefix + "22", Trust: spec.Managed}}, managed.Sources...)
	managed.Rules[0].Source = 0
	rr := *r
	rr.SpecEvents = []store.SpecEvent{*store.NewRuleHit("s1", postEvent, "spec:no-force", spec.DigestPrefix+"22", landed, 0)}
	if snap, _ := specLedgerAt(&rr, managed, pt); oneVerdict(t, snap).Verdict != Violation {
		t.Errorf("managed: verdict %+v, want a violation under the managed source's digest", oneVerdict(t, snap))
	}

	if got := hitSeq(r, 999); got != 999 {
		t.Errorf("hitSeq of a landed seq no execution carries = %d, want 999", got)
	}
}

// TestSRS1h_ARuleCallHitsAtEitherPostEvent: RulesAtCall hands back the
// matched rule with its source's digest at PostToolUse and at
// PostToolUseFailure, and raises platform-rule only at PostToolUse; an
// untrusted rule hits too (the Stop's trust decides whether it binds); a
// clean line hits nothing.
func TestSRS1h_ARuleCallHitsAtEitherPostEvent(t *testing.T) {
	failed := string(steer.EventPostToolUseFailure)
	for _, tr := range []spec.Trust{spec.Trusted, spec.Untrusted} {
		sp := ruleSpec(tr, noForce())
		want := []RuleHit{{RuleID: "spec:no-force", SourceDigest: sp.Sources[0].Digest}}
		call := RuleCall{Seq: 9, ToolName: "Bash", Command: "git push -f origin main"}
		post := RulesAtCall(sp, call, atPost(9), postEvent)
		fail := RulesAtCall(sp, call, atPost(9), failed)
		if !slices.Equal(post.Hits, want) || !slices.Equal(fail.Hits, want) {
			t.Errorf("%s: hits post %+v, failure %+v, want %+v at both", tr, post.Hits, fail.Hits, want)
		}
		if len(post.Findings) != 1 || len(fail.Findings) != 0 {
			t.Errorf("%s: findings post %d, failure %d, want 1 and 0", tr, len(post.Findings), len(fail.Findings))
		}
		call.Command = "git push origin main"
		if v := RulesAtCall(sp, call, atPost(9), failed); len(v.Hits) != 0 {
			t.Errorf("%s: a clean line hit %+v", tr, v.Hits)
		}
	}
}
