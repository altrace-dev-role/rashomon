package digest

import (
	"slices"
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

// TestREC34g_FlushFailedMarkerLeavesAnUnterminatedTurnUnterminated: a post
// whose flush failed marks the call with a flush_failed terminal (REC-34).
// That marker is not the hook's ending, so a declaration whose hook was killed
// before its own terminal still reads as unterminated once the run has ended.
func TestREC34g_FlushFailedMarkerLeavesAnUnterminatedTurnUnterminated(t *testing.T) {
	reason := store.ReasonFlushFailed
	run := &store.Run{
		Declarations: []store.Declaration{decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 100)},
		Terminals:    []store.Terminal{{ToolUseID: "a1", Outcome: store.OutcomeError, Reason: &reason, RecordedAtMS: 150}},
		Coverage: []store.Coverage{
			startCoverage("inst-1"),
			{Phase: store.PhaseEnd, State: store.StateVerified, InstallID: "inst-1"},
		},
	}
	w := selectTurn(run, "prompt-1")
	if got := turnUnterminated(w, run.Terminals); len(got) != 1 || got[0] != "a1" {
		t.Errorf("turnUnterminated = %v, want [a1]: the flush_failed marker closed the declaration", got)
	}
	tc, _ := buildTurnCoverage(run, nil, w)
	if !hasReason(tc.Reasons, store.ReasonUnterminatedEntry) {
		t.Errorf("reasons = %v, want unterminated_entry", tc.Reasons)
	}
}

// TestREC34n_FlushFailedMarkerDoesNotStretchADroppedTerminalsRecordedTime:
// turnDropped windows a dropped terminal by its OWN recorded time, because a
// dropped declaration never carried a prompt_id to key on (turnDropped's
// doc). A flush_failed marker (REC-34) sharing that terminal's tool_use_id --
// written by a later process, long after the terminal it marks -- must not
// overwrite that time in the attribution map, or the dropped terminal is
// windowed into whichever turn the MARKER happened to land in instead of the
// one it actually belongs to.
func TestREC34n_FlushFailedMarkerDoesNotStretchADroppedTerminalsRecordedTime(t *testing.T) {
	reason := store.ReasonFlushFailed
	run := &store.Run{
		Declarations: []store.Declaration{
			decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 100),
			decl(2, "a2", "Bash", "prompt-2", "/t.jsonl", 1000),
		},
		Terminals: []store.Terminal{
			// The real, genuinely dropped terminal: no declaration names
			// toolu_dropped, recorded inside prompt-1's window [100, 1000).
			{ToolUseID: "toolu_dropped", Outcome: store.OutcomeError, RecordedAtMS: 150},
			// A flush_failed marker for the SAME tool_use_id, recorded well
			// into prompt-2's window.
			{ToolUseID: "toolu_dropped", Outcome: store.OutcomeError, Reason: &reason, RecordedAtMS: 5000},
		},
	}
	w := selectTurn(run, "prompt-1")
	if got := turnDropped(run, w); len(got) != 1 || got[0] != "toolu_dropped" {
		t.Errorf("turnDropped(prompt-1) = %v, want [toolu_dropped]: the flush_failed marker's later "+
			"time pushed the dropped terminal's own out of the turn it was actually recorded in", got)
	}
}

// TestRubricERRORS5_TurnCoverageFailsClosedOnACoverageRecordItCannotRead is
// report's ERRORS5 table through the turn digest, which shares
// report.RollupCoverage: a call-phase coverage record that is unverified with
// no reason, or carries a state this build does not write, makes the turn
// unverified with records_unreadable.
func TestRubricERRORS5_TurnCoverageFailsClosedOnACoverageRecordItCannotRead(t *testing.T) {
	internalError := store.ReasonInternalError
	for _, c := range []struct {
		name   string
		state  string
		reason *string
	}{
		{"unverified with a null reason", store.StateUnverified, nil},
		{"an unknown state", "degraded", nil},
		{"an unknown state with a reason", "degraded", &internalError},
		{"an empty state", "", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			run := &store.Run{
				Declarations: []store.Declaration{decl(1, "a1", "Bash", "prompt-1", "/t.jsonl", 1000)},
				Coverage: []store.Coverage{
					startCoverage("inst-1"),
					{Phase: store.PhaseCall, State: c.state, Reason: c.reason, RecordedAtMS: 1000},
				},
			}
			tc, _ := buildTurnCoverage(run, nil, selectTurn(run, "prompt-1"))
			if tc.State != store.StateUnverified || !slices.Contains(tc.Reasons, report.ReasonRecordsUnreadable) {
				t.Errorf("turn coverage = %s %v, want unverified with %s", tc.State, tc.Reasons, report.ReasonRecordsUnreadable)
			}
		})
	}
}
