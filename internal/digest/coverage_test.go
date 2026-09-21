package digest

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

func startCoverage(installID string) store.Coverage {
	return store.Coverage{Phase: store.PhaseStart, State: store.StateVerified, InstallID: installID}
}

// TestBuildTurnCoverage_NeverAddsRunNotClosed is H-84's unit-level guard: an
// open run (no PhaseEnd record at all) must not read as a coverage gap.
func TestBuildTurnCoverage_NeverAddsRunNotClosed(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 100)},
		Coverage:     []store.Coverage{startCoverage("inst-1")},
	}
	w := selectTurn(run, "prompt-1")

	tc, _ := buildTurnCoverage(run, nil, w)
	for _, r := range tc.Reasons {
		if r == report.ReasonRunNotClosed {
			t.Fatalf("turn coverage carries run_not_closed: %v -- the run being open is a fact "+
				"true of every turn read at Stop and a finding against none of them", tc.Reasons)
		}
	}
	if tc.State != store.StateVerified {
		t.Errorf("state = %q, want verified: a probe at start, one call, and nothing else wrong "+
			"is a clean turn even though the session has not ended", tc.State)
	}
}

// TestBuildTurnCoverage_BreakOnSessionCoverageAddsRunNotClosed demonstrates
// the break H-84 names: projecting SESSION coverage (report.build's own
// rule) onto an open run adds run_not_closed to every healthy turn.
func TestBuildTurnCoverage_BreakOnSessionCoverageAddsRunNotClosed(t *testing.T) {
	run := &store.Run{Coverage: []store.Coverage{startCoverage("inst-1")}}
	facts := report.RollupCoverage(run.Coverage)
	if facts.EndRecorded {
		t.Fatal("premise: no PhaseEnd record was written")
	}
	// This is exactly what report.build does with that fact; digest must not.
	var reasons []string
	if !facts.EndRecorded {
		reasons = append(reasons, report.ReasonRunNotClosed)
	}
	if len(reasons) == 0 {
		t.Fatal("premise: the session-coverage rule adds run_not_closed here")
	}
}

// TestBuildTurnCoverage_GapIntersectingTheWindowIsAReason.
func TestBuildTurnCoverage_GapIntersectingTheWindowIsAReason(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 1000)},
		Coverage:     []store.Coverage{startCoverage("inst-1")},
	}
	w := selectTurn(run, "prompt-1")
	gap := store.Gap{FromUnixMS: 900, ToUnixMS: 1100} // overlaps [1000, +inf)

	tc, _ := buildTurnCoverage(run, []store.Gap{gap}, w)
	if !hasReason(tc.Reasons, report.ReasonGap) {
		t.Errorf("reasons = %v, want gap: the gap's window overlaps the turn's", tc.Reasons)
	}
}

// TestBuildTurnCoverage_GapBeforeTheWindowIsNotAReason: a forget that
// happened before this turn even started must not taint it.
func TestBuildTurnCoverage_GapBeforeTheWindowIsNotAReason(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 1000)},
		Coverage:     []store.Coverage{startCoverage("inst-1")},
	}
	w := selectTurn(run, "prompt-1")
	gap := store.Gap{FromUnixMS: 100, ToUnixMS: 200} // long before startMS=1000

	tc, _ := buildTurnCoverage(run, []store.Gap{gap}, w)
	if hasReason(tc.Reasons, report.ReasonGap) {
		t.Errorf("reasons = %v, want no gap: this gap's window ended before the turn started", tc.Reasons)
	}
}

// TestBuildTurnCoverage_UnterminatedIsInFlightWhileTheRunIsOpen is the
// "in-flight, not missing" fix: a declaration with no terminal yet must not
// flip a still-open turn to unverified.
func TestBuildTurnCoverage_UnterminatedIsInFlightWhileTheRunIsOpen(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 100)},
		Coverage:     []store.Coverage{startCoverage("inst-1")}, // no PhaseEnd: run is open
		// No Terminal for a1: exactly what an unterminated entry looks like.
	}
	w := selectTurn(run, "prompt-1")

	tc, facts := buildTurnCoverage(run, nil, w)
	if facts.EndRecorded {
		t.Fatal("premise: the run must be open for this test")
	}
	if hasReason(tc.Reasons, store.ReasonUnterminatedEntry) {
		t.Errorf("reasons = %v, want no unterminated_entry while the run is still open", tc.Reasons)
	}
	if tc.State != store.StateVerified {
		t.Errorf("state = %q, want verified", tc.State)
	}
}

// TestBuildTurnCoverage_UnterminatedCountsOnceTheRunHasEnded is the other
// half: once EndRecorded is true, "in flight" is no longer a possible
// explanation and the SAME unterminated entry is a real finding.
func TestBuildTurnCoverage_UnterminatedCountsOnceTheRunHasEnded(t *testing.T) {
	run := &store.Run{
		Declarations: []store.Declaration{decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 100)},
		Coverage: []store.Coverage{
			startCoverage("inst-1"),
			{Phase: store.PhaseEnd, State: store.StateVerified, InstallID: "inst-1"},
		},
	}
	w := selectTurn(run, "prompt-1")

	tc, facts := buildTurnCoverage(run, nil, w)
	if !facts.EndRecorded {
		t.Fatal("premise: the run must be closed for this test")
	}
	if !hasReason(tc.Reasons, store.ReasonUnterminatedEntry) {
		t.Errorf("reasons = %v, want unterminated_entry: the run has ended and this call still "+
			"has no terminal, which is no longer explained by being in flight", tc.Reasons)
	}
}

// TestBuildTurnCoverage_WindowedReasonDoesNotLeakAcrossTurns is a second
// angle on H-83/H-89: a hook_entry problem recorded during turn 1's window
// must not appear on turn 2's coverage.
func TestBuildTurnCoverage_WindowedReasonDoesNotLeakAcrossTurns(t *testing.T) {
	bad := store.ReasonHookEntryAbsent
	run := &store.Run{
		Declarations: []store.Declaration{
			decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 100),
			decl(2, "b1", "Bash", "prompt-2", "/t.jsonl", 500),
		},
		Coverage: []store.Coverage{
			startCoverage("inst-1"),
			{Phase: store.PhaseCall, State: store.StateUnverified, Reason: &bad, RecordedAtMS: 150},
		},
	}

	w2 := selectTurn(run, "prompt-2")
	tc2, _ := buildTurnCoverage(run, nil, w2)
	if hasReason(tc2.Reasons, store.ReasonHookEntryAbsent) {
		t.Errorf("prompt-2's coverage = %v, carries a reason recorded during prompt-1's window", tc2.Reasons)
	}

	w1 := selectTurn(run, "prompt-1")
	tc1, _ := buildTurnCoverage(run, nil, w1)
	if !hasReason(tc1.Reasons, store.ReasonHookEntryAbsent) {
		t.Errorf("prompt-1's coverage = %v, want hook_entry_absent: it happened inside this turn's window", tc1.Reasons)
	}
}

func hasReason(reasons []string, want string) bool {
	for _, r := range reasons {
		if r == want {
			return true
		}
	}
	return false
}
