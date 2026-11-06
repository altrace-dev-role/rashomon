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

// TestGS3_AUnitLineAndARoleRecordAsTheGraphKinds (W-M1.12): the role
// words a code line carries and the unit line's kind are steer's graph
// kinds, pinned here since slot cannot import steer; a unit line records
// its path, names, consumer files and lessons, and a roled code line its
// role, in the same flat form, the record form counting the consumers.
func TestGS3_AUnitLineAndARoleRecordAsTheGraphKinds(t *testing.T) {
	if slot.RoleReuse != string(KindReuse) || slot.RoleConsumer != string(KindConsumer) {
		t.Fatalf("slot roles %q %q are not steer's kinds %q %q", slot.RoleReuse, slot.RoleConsumer, KindReuse, KindConsumer)
	}
	for _, k := range []Kind{KindUnit, KindReuse, KindConsumer} {
		if !slices.Contains(GraphKinds(), k) {
			t.Errorf("%q is not a graph kind", k)
		}
	}
	unit := slot.NewUnitLine(slot.UnitFact{Path: "internal/platform", Names: []slot.Ident{"Read", "Callers"},
		Consumers: []slot.RelPath{"cmd/rashomon/report_platform.go", "cmd/rashomon/blast.go", "cmd/rashomon/laststop.go"},
		Lessons:   []slot.KnowledgeID{"dec:D145"}})
	code := slot.NewCodeLine(slot.CodeFact{Path: "cmd/rashomon/report_platform.go", Role: slot.RoleConsumer,
		Names: []slot.Ident{"platformReadWithin"}, Calls: []slot.Ident{"Read"}})
	m, err := render.New(pattern.TaskPreflight, render.NewGraphBinding("C1", 1, 2, []slot.GraphLine{boundGraph(t, unit), boundGraph(t, code)}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := slotsOf(m)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("unit line: %s\nslot: %s", strings.Split(m.Text(), "\n")[1], got["line1"])
	if string(unit.Kind()) != string(KindUnit) {
		t.Errorf("unit line kind %q", unit.Kind())
	}
	var l1 struct {
		Kind, Path string
		Names      []string
		Consumers  []string
		Lessons    []string
		Counts     map[string]int
	}
	if err := json.Unmarshal(got["line1"], &l1); err != nil {
		t.Fatal(err)
	}
	if l1.Kind != "unit" || l1.Path != "internal/platform" || len(l1.Names) != 2 || len(l1.Consumers) != 2 || l1.Counts["consumers"] != 1 || len(l1.Lessons) != 1 {
		t.Errorf("unit slot = %+v", l1)
	}
	if !strings.Contains(string(got["line2"]), `"role":"consumer"`) {
		t.Errorf("code slot lacks the role: %s", got["line2"])
	}
	rec, err := render.New(pattern.TaskPreflight, render.NewGraphBinding("C1", 1, 2, []slot.GraphLine{
		boundGraph(t, unit).Record(func(string) string { return strings.Repeat("0", 64) })}))
	if err != nil {
		t.Fatal(err)
	}
	recSlots, err := slotsOf(rec)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("record form: %s", recSlots["line1"])
	if !strings.Contains(string(recSlots["line1"]), `"consumers":[]`) || !strings.Contains(string(recSlots["line1"]), `"consumers":3`) || strings.Contains(string(recSlots["line1"]), "platform") {
		t.Errorf("record form = %s", recSlots["line1"])
	}
}
