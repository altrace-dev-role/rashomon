package verify

import (
	"regexp"
	"slices"
	"testing"
)

// TestDEL22_GateReasonsAreDistinct pins the gate Reason vocabulary of 2.2.4:
// the eighteen reasons, in the spec's order, each a distinct snake_case
// identifier (DEL-22, "gate Reason vocabulary").
func TestDEL22_GateReasonsAreDistinct(t *testing.T) {
	frozen := []Reason{
		"failed_exit", "failed_output",
		"masked_with_success_line", "masked_no_success_line", "masked_unattributable",
		"empty_run", "partial_after_red", "partial_only",
		"red_then_writes", "writes_after_green", "write_during_run", "never_run_after_write",
		"background_launch", "order_unknown", "outcome_unknown", "recording_incomplete",
		"budget", "ok",
	}
	if !slices.Equal(Reasons, frozen) {
		t.Fatalf("Reasons drifted from the spec's list:\n got %q\nwant %q", Reasons, frozen)
	}
	form := regexp.MustCompile(`^[a-z][a-z_]*[a-z]$`)
	seen := map[Reason]bool{}
	for _, r := range Reasons {
		if seen[r] {
			t.Errorf("reason %q is listed twice", r)
		}
		seen[r] = true
		if !form.MatchString(string(r)) {
			t.Errorf("reason %q is not a snake_case identifier", r)
		}
	}
}

// TestVER65b_OneOrderForComparatorAndTurnSummary pins VER 11 (a): one ranking
// of states, worst first, read by the claim comparator (2.2.5 step 7) and by
// TurnSummary (2.2.11) alike, with a gate's own unknown below every non-green
// state and above green.
func TestVER65b_OneOrderForComparatorAndTurnSummary(t *testing.T) {
	frozen := []State{"red", "masked_inconclusive", "backgrounded", "stale", "absent", "unknown", "green"}
	if !slices.Equal(Order, frozen) {
		t.Fatalf("Order drifted:\n got %q\nwant %q", Order, frozen)
	}
	wantStates := []State{"green", "red", "masked_inconclusive", "stale", "absent", "backgrounded", "unknown"}
	if !slices.Equal(States, wantStates) {
		t.Fatalf("States\n got %q\nwant %q", States, wantStates)
	}
	for _, s := range []State{Red, MaskedInconclusive, Backgrounded, Stale, Absent} {
		if !Worse(s, Unknown) {
			t.Errorf("%q must rank before a gate's own unknown (VER 11 (a))", s)
		}
	}
	if !Worse(Unknown, Green) {
		t.Errorf("unknown must rank before green: an unknown is never a pass")
	}
	if Rank(State("purple")) >= Rank(Red) {
		t.Errorf("a state outside the vocabulary must rank before red, got %d", Rank(State("purple")))
	}
	if v := VerdictOf(nil); v != NoGateKnown {
		t.Errorf("no gate: verdict %q, want %q", v, NoGateKnown)
	}
	// Worst reads every state, wherever the worst one sits.
	for i, states := range [][]State{
		{Green, Green, Red}, {Red, Green, Green}, {Green, Red, Green},
		{Stale, Absent, Unknown, Green}, {Green, Unknown, Absent},
	} {
		want := states[0]
		for _, s := range states {
			if Worse(s, want) {
				want = s
			}
		}
		if got, _ := Worst(states); got != want {
			t.Errorf("case %d: Worst(%q) = %q, want %q", i, states, got, want)
		}
	}

	// Both consumers read the one order: over every pair and every triple of
	// states, the row TurnSummary picks is the row of the comparator's
	// verdict, whichever gate carries the dominant state.
	rowOf := map[Verdict]Row{
		Verdict(Red): RowRed, Verdict(MaskedInconclusive): RowMasked,
		Verdict(Backgrounded): RowBackgrounded, Verdict(Stale): RowUnverified,
		Verdict(Absent): RowUnverified, Verdict(Unknown): RowUnknown,
		Verdict(Green): RowFindings, // writes, every gate green, no record-green point
	}
	check := func(states ...State) {
		t.Helper()
		var gates []GateState
		for i, s := range states {
			gates = append(gates, GateState{Gate: string(rune('a' + i)), Required: true, State: s})
		}
		got := TurnSummary(Snapshot{Recording: Complete, Gates: gates}, TurnCounts{Writes: 1}).Row
		verdict := VerdictOf(states)
		if got != rowOf[verdict] {
			t.Errorf("gates %q: TurnSummary row %q, comparator verdict %q wants row %q", states, got, verdict, rowOf[verdict])
		}
	}
	for _, a := range States {
		for _, b := range States {
			check(a, b)
			for _, c := range States {
				check(a, b, c)
			}
		}
	}
}

// TestVER65c_TurnSummaryRowsFollowTheTable pins the rows that the dominant state
// alone does not decide: no line without a write or run, unknown on
// incomplete recording, no_gate, ran_only, unnamed_write against the
// EARLIEST last green, verified, and the out-of-vocabulary defect.
func TestVER65c_TurnSummaryRowsFollowTheTable(t *testing.T) {
	green := func(name string, lastGreen int64) GateState {
		return GateState{Gate: name, Required: true, State: Green, LastGreen: &RunRef{Seq: lastGreen}}
	}
	neverGreen := GateState{Gate: "never", Required: true, State: Green}
	cases := []struct {
		name string
		snap Snapshot
		turn TurnCounts
		want Row
	}{
		{"no write and no run: no line", Snapshot{Recording: Complete}, TurnCounts{}, RowNone},
		{"recording incomplete", Snapshot{Recording: Incomplete, Gates: []GateState{green("a", 9)}}, TurnCounts{Writes: 1}, RowUnknown},
		{"writes and no gate", Snapshot{Recording: Complete}, TurnCounts{Writes: 2}, RowNoGate},
		{"untrusted gates are not turn gates", Snapshot{Recording: Complete, Gates: []GateState{{Gate: "x", State: Red}}}, TurnCounts{Writes: 2}, RowNoGate},
		{"no write, a green run", Snapshot{Recording: Complete, Gates: []GateState{green("a", 9)}}, TurnCounts{GateRuns: 1}, RowRanOnly},
		{"unnamed write after the green run", Snapshot{Recording: Complete, Gates: []GateState{green("a", 9)}}, TurnCounts{Writes: 1, UnnamedWriteSeq: 12}, RowUnnamedWrite},
		{"unnamed write before the green run", Snapshot{Recording: Complete, Gates: []GateState{green("a", 9)}, RecordGreen: true}, TurnCounts{Writes: 1, UnnamedWriteSeq: 7}, RowVerified},
		{"two gates green at 5 and 20, unnamed write at 10: after the earliest", Snapshot{Recording: Complete, Gates: []GateState{green("a", 5), green("b", 20)}, RecordGreen: true}, TurnCounts{Writes: 1, UnnamedWriteSeq: 10}, RowUnnamedWrite},
		{"two gates green at 20 and 5, unnamed write at 10: order of gates is irrelevant", Snapshot{Recording: Complete, Gates: []GateState{green("b", 20), green("a", 5)}, RecordGreen: true}, TurnCounts{Writes: 1, UnnamedWriteSeq: 10}, RowUnnamedWrite},
		{"two gates green at 5 and 20, unnamed write at 3: before both", Snapshot{Recording: Complete, Gates: []GateState{green("a", 5), green("b", 20)}, RecordGreen: true}, TurnCounts{Writes: 1, UnnamedWriteSeq: 3}, RowVerified},
		{"a green gate with no green ref counts as never: any unnamed write is after it", Snapshot{Recording: Complete, Gates: []GateState{green("a", 20), neverGreen}, RecordGreen: true}, TurnCounts{Writes: 1, UnnamedWriteSeq: 1}, RowUnnamedWrite},
		{"record-green point", Snapshot{Recording: Complete, Gates: []GateState{green("a", 9)}, RecordGreen: true}, TurnCounts{Writes: 1}, RowVerified},
		{"all green, no record-green point", Snapshot{Recording: Complete, Gates: []GateState{green("a", 9)}}, TurnCounts{Writes: 1}, RowFindings},
		{"a state outside the vocabulary beside green is unknown, never a pass", Snapshot{Recording: Complete, Gates: []GateState{green("a", 9), {Gate: "b", Required: true, State: State("purple")}}, RecordGreen: true}, TurnCounts{Writes: 1}, RowUnknown},
		{"a state outside the vocabulary never hides a red", Snapshot{Recording: Complete, Gates: []GateState{{Gate: "a", Required: true, State: Red}, {Gate: "b", Required: true, State: State("purple")}}}, TurnCounts{Writes: 1}, RowRed},
		{"only a defective gate: unknown, not no_gate", Snapshot{Recording: Complete, Gates: []GateState{{Gate: "b", Required: true, State: State("purple")}}}, TurnCounts{Writes: 1}, RowUnknown},
	}
	for _, c := range cases {
		got := TurnSummary(c.snap, c.turn)
		if got.Row != c.want {
			t.Errorf("%s: row %q, want %q", c.name, got.Row, c.want)
		}
		// P-45 adds the sentences; until then, every row this table picks
		// carries none, recording-incomplete's sentenceRecordingIncomplete
		// (checked separately below) excepted.
		wantSentence := ""
		if c.name == "recording incomplete" {
			wantSentence = sentenceRecordingIncomplete
		}
		if got.Sentence != wantSentence {
			t.Errorf("%s: sentence %q, want %q", c.name, got.Sentence, wantSentence)
		}
	}
	if s := TurnSummary(Snapshot{Recording: Incomplete}, TurnCounts{Writes: 1}); s.Sentence != sentenceRecordingIncomplete {
		t.Errorf("incomplete recording sentence %q", s.Sentence)
	}
	if VerdictOf([]State{Green, State("purple")}) != Verdict("purple") {
		t.Error("the comparator must surface a state outside the vocabulary, never sink it to green")
	}
}
