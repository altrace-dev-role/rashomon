package steer

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TestGS3_ARecordClaimRecordsItsRecordsInBothForms (W-M1.41b): a unit
// line claimed by the records the request names records them as a
// knowledge line records its id, listed to the cap and the rest counted,
// in the text form and the record form alike, beside the lessons that
// apply to the unit, which stay a list of their own; the record form keeps
// no path, and its text no runner-up.
func TestGS3_ARecordClaimRecordsItsRecordsInBothForms(t *testing.T) {
	unit := slot.NewUnitLine(slot.UnitFact{Path: "internal/report", Basis: slot.BasisRecord, Band: slot.BandCandidate,
		RunnerUp: "internal/spend", Records: []slot.KnowledgeID{"dec:D176", "dec:D109", "dec:D1", "dec:D2"}, Units: 3,
		First: slot.BasisCoverage, Lessons: []slot.KnowledgeID{"les:B9"}, Unknown: []string{slot.UnknownTarget}})
	digest := func(string) string { return strings.Repeat("0", 64) }
	for _, form := range []struct {
		name, path, text string
		line             slot.GraphLine
	}{
		{"text", "internal/report",
			"unit 'internal/report' (a candidate: the request names D176, D109, D1 and 1 more, which apply in 3 units; this one by word coverage; runner-up 'internal/spend') B9 applies; not known: the file to change",
			boundGraph(t, unit)},
		{"record", "", "a unit on record B9 applies; not known: the file to change", boundGraph(t, unit).Record(digest)},
	} {
		m, err := render.New(pattern.TaskPreflight, render.NewGraphBinding("C1", 1, 2, []slot.GraphLine{form.line}))
		if err != nil {
			t.Fatalf("%s form: %v", form.name, err)
		}
		got, err := slotsOf(m)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s form: %s\nslot: %s", form.name, m.Text(), got["line1"])
		var l1 struct {
			Kind, Path string
			Records    []string
			Lessons    []string
			Counts     map[string]int
		}
		if err := json.Unmarshal(got["line1"], &l1); err != nil {
			t.Fatal(err)
		}
		if l1.Kind != "unit" || l1.Path != form.path || !slices.Equal(l1.Records, []string{"dec:D176", "dec:D109", "dec:D1"}) ||
			l1.Counts["records"] != 1 || !slices.Equal(l1.Lessons, []string{"les:B9"}) || l1.Counts["lessons"] != 0 {
			t.Errorf("%s form slot = %+v", form.name, l1)
		}
		if lines := strings.Split(m.Text(), "\n"); len(lines) != 2 || lines[1] != form.text {
			t.Errorf("%s form text\n got %q\nwant %q", form.name, lines, form.text)
		}
	}
}
