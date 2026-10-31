package measure

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestMS1w_PairCountsEqualImpactOf pins the pair's steer-side counts to the
// recap's own reading of the same lines (recap.ImpactOf, REPORT1a), so the
// two reports of one session never say different numbers: the notes given
// are ImpactOf's delivered notes, and the patterns the pair raised findings
// for are the patterns ImpactOf's findings name.
func TestMS1w_PairCountsEqualImpactOf(t *testing.T) {
	f := twoArms(t)
	// A stale-evidence note is a finding to recap.ImpactOf; the pair must
	// read it as one too.
	f.lines(sSteer, delivered("t5", pattern.StaleEvidence, "go", 5))
	p := pairOf(t, f)
	lines, _, err := f.st.ReadSteer(sSteer)
	if err != nil {
		t.Fatal(err)
	}
	im := recap.ImpactOf(verify.StopView{}, lines, nil)
	on := p.Sides[1]
	if on.Arm != string(steer.ArmSteer) {
		t.Fatalf("premise: side 1 is arm %q, want steer", on.Arm)
	}
	if on.NotesGiven != len(im.Delivered) || len(im.Delivered) == 0 {
		t.Errorf("the pair gave %d notes, ImpactOf %d delivered", on.NotesGiven, len(im.Delivered))
	}
	var pairPatterns, impactPatterns []string
	for _, fd := range on.Findings {
		pairPatterns = append(pairPatterns, fd.Pattern)
	}
	for _, fd := range im.Findings {
		if !slices.Contains(impactPatterns, fd.Pattern) {
			impactPatterns = append(impactPatterns, fd.Pattern)
		}
	}
	slices.Sort(pairPatterns)
	slices.Sort(impactPatterns)
	if !slices.Equal(pairPatterns, impactPatterns) || len(impactPatterns) == 0 {
		t.Errorf("the pair's finding patterns %v, ImpactOf's %v", pairPatterns, impactPatterns)
	}
}
