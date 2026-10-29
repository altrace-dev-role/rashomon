package learn

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// TestP6_Wilson95MatchesTheDetectorTable: the production interval gives the
// rows test/acceptance/detector_stats_test.go pins for the helper it
// replaced, and no interval where there is none.
func TestP6_Wilson95MatchesTheDetectorTable(t *testing.T) {
	for _, tc := range []struct {
		k, n   int
		lo, hi float64
	}{
		{30, 30, 0.886487, 1.0},
		{27, 30, 0.743789, 0.965400},
		{15, 30, 0.331541, 0.668459},
		{0, 30, 0.0, 0.113513},
		{1, 1, 0.206549, 1.0},
		{0, 10, 0.0, 0.277533},
		{9, 10, 0.595850, 0.982124},
	} {
		lo, hi, ok := Wilson95(tc.k, tc.n)
		if !ok || math.Abs(lo-tc.lo) > 1e-6 || math.Abs(hi-tc.hi) > 1e-6 {
			t.Errorf("Wilson95(%d, %d) = [%.6f, %.6f] ok=%v, want [%.6f, %.6f]", tc.k, tc.n, lo, hi, ok, tc.lo, tc.hi)
		}
	}
	for _, bad := range [][2]int{{0, 0}, {5, 3}, {-1, 4}, {1, -2}} {
		if _, _, ok := Wilson95(bad[0], bad[1]); ok {
			t.Errorf("Wilson95(%d, %d) gave an interval; k outside 0..n or n <= 0 has none", bad[0], bad[1])
		}
	}
}

// TestP6_ThresholdsArePinned: the numbers a person recomputes a tier with.
func TestP6_ThresholdsArePinned(t *testing.T) {
	if Threshold != 0.75 || MinProven != 30 || MinEpisodes != 3 || MinSessions != 3 || MinDays != 2 {
		t.Errorf("Threshold %v, MinProven %d, MinEpisodes %d, MinSessions %d, MinDays %d; want 0.75, 30, 3, 3, 2",
			Threshold, MinProven, MinEpisodes, MinSessions, MinDays)
	}
}

// TestP6_TierMovesOnlyByTheBound: proposed waits for a person whatever its
// labels; an approved lesson is proven at n >= 30 with the lower bound at
// 0.75, demoted with the upper bound under it, on probation between, and
// back from demoted when the upper bound returns. Unresolved labels never
// enter n.
func TestP6_TierMovesOnlyByTheBound(t *testing.T) {
	for _, tc := range []struct {
		name string
		from Tier
		t    Tally
		want Tier
	}{
		{"proposed with every label confirmed", TierProposed, Tally{Confirmed: 40}, TierProposed},
		{"no labels", TierProbation, Tally{}, TierProbation},
		{"30 of 30: lower 0.886", TierProbation, Tally{Confirmed: 30}, TierProven},
		{"28 of 30: lower 0.787", TierProbation, Tally{Confirmed: 28, Refuted: 2}, TierProven},
		{"27 of 30: lower 0.744", TierProbation, Tally{Confirmed: 27, Refuted: 3}, TierProbation},
		{"29 of 29: lower 0.883, n under 30", TierProbation, Tally{Confirmed: 29}, TierProbation},
		{"0 of 1: upper 0.794", TierProbation, Tally{Refuted: 1}, TierProbation},
		{"0 of 2: upper 0.658", TierProbation, Tally{Refuted: 2}, TierDemoted},
		{"3 of 7: upper 0.7495", TierProven, Tally{Confirmed: 3, Refuted: 4}, TierDemoted},
		{"demoted, 4 of 7: upper 0.842", TierDemoted, Tally{Confirmed: 4, Refuted: 3}, TierProbation},
		{"proven, 27 of 30", TierProven, Tally{Confirmed: 27, Refuted: 3}, TierProbation},
		{"unresolved never in n: 0 of 2", TierProbation, Tally{Refuted: 2, Unresolved: 500}, TierDemoted},
		{"unresolved never in n: 30 of 30", TierProbation, Tally{Confirmed: 30, Unresolved: 500}, TierProven},
		{"unresolved alone", TierDemoted, Tally{Unresolved: 9}, TierProbation},
	} {
		if got := tc.from.Next(tc.t); got != tc.want {
			t.Errorf("%s: %s.Next(%+v) = %s, want %s", tc.name, tc.from, tc.t, got, tc.want)
		}
	}
}

// TestP6_CandidateGate: PF-HD5's floors, each at and under its edge.
func TestP6_CandidateGate(t *testing.T) {
	ok := Evidence{Episodes: 3, Sessions: 3, Days: 2, Consistency: 0.75}
	if !Candidate(ok) {
		t.Fatalf("Candidate(%+v) = false at every floor", ok)
	}
	for name, e := range map[string]Evidence{
		"2 episodes":      {Episodes: 2, Sessions: 3, Days: 2, Consistency: 0.75},
		"2 sessions":      {Episodes: 3, Sessions: 2, Days: 2, Consistency: 0.75},
		"1 day":           {Episodes: 3, Sessions: 3, Days: 1, Consistency: 0.75},
		"consistency .74": {Episodes: 3, Sessions: 3, Days: 2, Consistency: 0.74},
		"a contradiction": {Episodes: 9, Sessions: 9, Days: 9, Consistency: 1, Contradictions: 1},
	} {
		if Candidate(e) {
			t.Errorf("%s: Candidate(%+v) = true", name, e)
		}
	}
}

// TestP6_LabelRowIsLabelWriter1sContract: labels.ndjson's row, field by field,
// as LABEL-WRITER1 must write it; the label values are steer.Label's three.
func TestP6_LabelRowIsLabelWriter1sContract(t *testing.T) {
	b, err := json.Marshal(Label{LessonID: "l1", SessionID: "s1", AtUnixMS: 5, Label: steer.LabelConfirmed})
	if want := `{"lesson_id":"l1","session_id":"s1","at_unix_ms":5,"label":"confirmed"}`; err != nil || string(b) != want {
		t.Errorf("a label row marshals to %s (%v), want %s", b, err, want)
	}
	var tl Tally
	for _, l := range []steer.Label{steer.LabelConfirmed, steer.LabelRefuted, steer.LabelUnresolved, "fixed", ""} {
		tl.add(l)
	}
	if tl != (Tally{Fires: 3, Confirmed: 1, Refuted: 1, Unresolved: 1}) {
		t.Errorf("tally %+v: want the three label values counted and any other value not a fire", tl)
	}
}
