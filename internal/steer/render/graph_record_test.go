package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TestGS2_ARecordClaimAtItsLongestRendersUnderTheBriefingCheck (W-M1.41b):
// a record claim's unit line at the longest its clause grows to -- five
// records at the longest id, three listed and two counted, the most units
// a count holds and units the request bars, the path and the runner-up at
// the longest relative path and every kind not known -- renders under
// ValidateBriefing, its words a statement of fact; the test logs its bytes
// against the cap.
func TestGS2_ARecordClaimAtItsLongestRendersUnderTheBriefingCheck(t *testing.T) {
	long := slot.RelPath("../../../../" + strings.Repeat("abcdefghij/", 23) + "abc")
	if err := long.Validate(); err != nil {
		t.Fatalf("premise: the longest relative path is refused: %v", err)
	}
	var ids []slot.KnowledgeID
	for i := 1; i <= 5; i++ {
		id := slot.KnowledgeID(fmt.Sprintf("D%d%s", i, strings.Repeat("x", 62)))
		if err := id.Validate(); err != nil {
			t.Fatalf("premise: the longest knowledge id is refused: %v", err)
		}
		ids = append(ids, id)
	}
	unit := slot.NewUnitLine(slot.UnitFact{Path: long, Basis: slot.BasisRecord, Band: slot.BandCandidate, RunnerUp: long,
		Records: ids, Units: 1_000_000, Barred: 1_000_000, First: slot.BasisCoverage,
		Unknown: []string{slot.UnknownTarget, slot.UnknownReuse, slot.UnknownConsumer, slot.UnknownKnowledge}})
	m, err := New(pattern.TaskPreflight, NewGraphBinding("C1", 1, 1, []slot.GraphLine{unit}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Logf("%d bytes of %d:\n%s", len(m.Text()), MaxBriefingMessageBytes, m.Text())
	if err := ValidateBriefing(m.Text()); err != nil {
		t.Errorf("the rendered brief fails the briefing check: %v", err)
	}
	for _, want := range []string{string(ids[2]) + " and 2 more, which apply in 1000000 units the request does not bar; this one by word coverage; runner-up '",
		"; not known: the file to change, the code to reuse, the consumers of the change, other records that apply to the change"} {
		if !strings.Contains(m.Text(), want) {
			t.Errorf("the line lacks %q", want)
		}
	}
}
