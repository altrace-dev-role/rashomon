// Package learn is the lesson ledger (card P6; PRD PF-HD2, PF-HD5): a lesson
// is a row with a tier and a tally, moved by a bound a person can recompute,
// never a model. It is its own component with its own store under the store
// root's learn/ directory (PF-HD16): hook and store import nothing of it,
// and it reaches the agent only through active.json, which a later card's
// delivery reads (LEARN-DELIVER1).
package learn

import (
	"math"

	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// Tier is where a lesson stands. Proposed is a candidate no person has
// approved; it is never delivered and moves only by approval. The other three
// follow the tally alone (Next).
type Tier string

const (
	TierProposed  Tier = "proposed"
	TierProbation Tier = "probation"
	TierProven    Tier = "proven"
	TierDemoted   Tier = "demoted"
)

// Threshold is the bound every tier move is read against, and PF-HD5's
// consistency floor for a candidate: one number governs both. MinProven is
// the fewest scored labels a proven lesson stands on.
const (
	Threshold = 0.75
	MinProven = 30
)

// The candidate gate's floors (PF-HD5): episodes, distinct sessions and
// distinct days.
const (
	MinEpisodes = 3
	MinSessions = 3
	MinDays     = 2
)

// z95 is the standard normal quantile for a two-sided 95% interval.
const z95 = 1.959963984540054

// Tally is a lesson's labels: Fires counts every label row, and only
// Confirmed and Refuted are scored. Unresolved is counted and never in n,
// so a fire nobody resolved neither helps nor hurts a lesson.
type Tally struct {
	Fires      int `json:"fires"`
	Confirmed  int `json:"confirmed"`
	Refuted    int `json:"refuted"`
	Unresolved int `json:"unresolved"`
}

// add counts one label; a value outside steer.AllLabels is not a fire.
func (t *Tally) add(l steer.Label) {
	switch l {
	case steer.LabelConfirmed:
		t.Confirmed++
	case steer.LabelRefuted:
		t.Refuted++
	case steer.LabelUnresolved:
		t.Unresolved++
	default:
		return
	}
	t.Fires++
}

// Bound is the Wilson interval over the scored labels; ok is false with none.
func (t Tally) Bound() (lo, hi float64, ok bool) {
	return Wilson95(t.Confirmed, t.Confirmed+t.Refuted)
}

// Next is the tier tally moves t to. Proposed stays proposed: only a person
// moves it. Otherwise the tally decides alone, so a demoted lesson whose
// upper bound comes back to Threshold returns to probation (PF-HD2's soft,
// auto-reactivating retirement; every lesson here is a failure lesson).
func (t Tier) Next(tally Tally) Tier {
	if t == TierProposed {
		return TierProposed
	}
	lo, hi, ok := tally.Bound()
	switch {
	case !ok:
		return TierProbation
	case tally.Confirmed+tally.Refuted >= MinProven && lo >= Threshold:
		return TierProven
	case hi < Threshold:
		return TierDemoted
	}
	return TierProbation
}

// Evidence is what episode mining (PF-HD3) reports of a candidate.
type Evidence struct {
	Episodes, Sessions, Days, Contradictions int
	Consistency                              float64
}

// Candidate reports whether e may enter the ledger at proposed (PF-HD5).
func Candidate(e Evidence) bool {
	return e.Episodes >= MinEpisodes && e.Sessions >= MinSessions && e.Days >= MinDays &&
		e.Consistency >= Threshold && e.Contradictions == 0
}

// Wilson95 is the Wilson score interval for k true of n, at 95%. ok is false
// for n <= 0 or k outside 0..n, where there is no interval to give. Wilson
// and not the normal approximation because it stays inside [0, 1] and is
// honest at small n and at p near 1.
func Wilson95(k, n int) (lo, hi float64, ok bool) {
	if n <= 0 || k < 0 || k > n {
		return 0, 0, false
	}
	nf, p := float64(n), float64(k)/float64(n)
	z2 := z95 * z95
	den := 1 + z2/nf
	centre := (p + z2/(2*nf)) / den
	half := z95 * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf)) / den
	return math.Max(0, centre-half), math.Min(1, centre+half), true
}
