package render

// The unit brief's task-preflight rows (W-M2.3 hook half, ruling 04:58 B):
// the lines a unit's brief keeps, said at the post of an edit in the unit,
// under a head that names the unit and says the note comes once a session,
// where the prompt's graph brief names the request and its counts. The
// lines are the graph row's (slot.GraphLine), so the line rules and STR-8
// hold for them as for the graph brief. The record row is the head with no
// unit: at record scope v4 the steer line holds no path, so the unit is a
// slot of the plain row only (its steer key names it). Kept apart from
// graph.go so each file stays inside Law 10's 500 code lines.

import (
	"reflect"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TaskPreflightUnitBriefBinding is a unit's brief as the post says it: the
// unit's directory relative to the hook's working directory ("." when it
// is that directory), the revision the brief was read at, and up to five
// lines, an empty one left out by the sparse template.
type TaskPreflightUnitBriefBinding struct {
	Unit  slot.RelPath   `slot:"unit"`
	Rev   slot.Revision  `slot:"rev"`
	Line1 slot.GraphLine `slot:"line1"`
	Line2 slot.GraphLine `slot:"line2"`
	Line3 slot.GraphLine `slot:"line3"`
	Line4 slot.GraphLine `slot:"line4"`
	Line5 slot.GraphLine `slot:"line5"`
}

// TaskPreflightUnitBriefRecordBinding is the unit brief's record form: the
// revision and the lines in their record form (slot.GraphLine.Record), no
// unit path.
type TaskPreflightUnitBriefRecordBinding struct {
	Rev   slot.Revision  `slot:"rev"`
	Line1 slot.GraphLine `slot:"line1"`
	Line2 slot.GraphLine `slot:"line2"`
	Line3 slot.GraphLine `slot:"line3"`
	Line4 slot.GraphLine `slot:"line4"`
	Line5 slot.GraphLine `slot:"line5"`
}

// NewUnitBriefBinding is the binding for lines, at most GraphLineSlots of
// them, the rest left empty.
func NewUnitBriefBinding(unit slot.RelPath, rev slot.Revision, lines []slot.GraphLine) TaskPreflightUnitBriefBinding {
	b := TaskPreflightUnitBriefBinding{Unit: unit, Rev: rev}
	setGraphLines(lines, &b.Line1, &b.Line2, &b.Line3, &b.Line4, &b.Line5)
	return b
}

// NewUnitBriefRecordBinding is the record binding for lines, at most
// GraphLineSlots of them, the rest left empty.
func NewUnitBriefRecordBinding(rev slot.Revision, lines []slot.GraphLine) TaskPreflightUnitBriefRecordBinding {
	b := TaskPreflightUnitBriefRecordBinding{Rev: rev}
	setGraphLines(lines, &b.Line1, &b.Line2, &b.Line3, &b.Line4, &b.Line5)
	return b
}

// setGraphLines sets each slot of at to the line of lines at its index.
func setGraphLines(lines []slot.GraphLine, at ...*slot.GraphLine) {
	for i := 0; i < len(lines) && i < len(at); i++ {
		*at[i] = lines[i]
	}
}

// unitBriefHead and unitBriefRecordHead open the unit brief. "Once a
// session" and not "the first edit": a brief withheld at the unit's first
// edit (the prompt's three-note budget spent, steering off) is said at a
// later one, and the head is a statement of fact at every edit it opens.
const (
	unitBriefHead       = "rashomon record: an edit in unit {unit}, noted once a session; the repository graph at rev {rev} records for it:\n"
	unitBriefRecordHead = "rashomon record: an edit in a unit on record, noted once a session; the repository graph at rev {rev} records for it:\n"
)

// templatesUnitBrief is the unit brief's two task-preflight rows.
func templatesUnitBrief() map[templateKey]template {
	return map[templateKey]template{
		{pattern.TaskPreflight, reflect.TypeOf(TaskPreflightUnitBriefBinding{})}: {
			id:     "task-preflight-unitbrief/v1",
			format: unitBriefHead + graphLines,
			sparse: true,
		},
		{pattern.TaskPreflight, reflect.TypeOf(TaskPreflightUnitBriefRecordBinding{})}: {
			id:     "task-preflight-unitbrief-record/v1",
			format: unitBriefRecordHead + graphLines,
			sparse: true,
		},
	}
}
