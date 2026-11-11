package render

// The task-preflight row that carries the repository graph's brief (card
// GRAPH-SLOT1): the brief is a note on the typed channel like every other
// note, its paths and names slot values and its header a fixed line, so
// STR-8 holds for it (nothing prints that no steer line records) and the
// line rules hold for every word the model is sent. Kept apart from
// render.go so each file stays inside Law 10's 500 code lines.

import (
	"reflect"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TaskPreflightGraphBinding is the graph brief: the revision it was read
// at, how many nodes the request's words matched and how many the walk
// reached, and up to five lines, each a code, table or knowledge fact
// (slot.GraphLine), an empty one left out by the sparse template. Five
// lines and the header are the briefing cap's six; the brief steps a line
// down to fit its bytes and never raises the cap.
type TaskPreflightGraphBinding struct {
	Rev     slot.Revision  `slot:"rev"`
	Matched slot.Count     `slot:"matched"`
	Reached slot.Count     `slot:"reached"`
	Line1   slot.GraphLine `slot:"line1"`
	Line2   slot.GraphLine `slot:"line2"`
	Line3   slot.GraphLine `slot:"line3"`
	Line4   slot.GraphLine `slot:"line4"`
	Line5   slot.GraphLine `slot:"line5"`
}

// GraphLineSlots is how many lines the graph binding carries.
const GraphLineSlots = 5

// NewGraphBinding is the binding for lines, at most GraphLineSlots of them,
// the rest left empty.
func NewGraphBinding(rev slot.Revision, matched, reached slot.Count, lines []slot.GraphLine) TaskPreflightGraphBinding {
	b := TaskPreflightGraphBinding{Rev: rev, Matched: matched, Reached: reached}
	at := []*slot.GraphLine{&b.Line1, &b.Line2, &b.Line3, &b.Line4, &b.Line5}
	for i := 0; i < len(lines) && i < len(at); i++ {
		*at[i] = lines[i]
	}
	return b
}

// graphHead is the brief's header: what the lines are and where they came
// from, with the two counts that size the read.
const graphHead = "rashomon record: the code this request touches, read from the repository graph at rev {rev}: " +
	"{matched} node(s) matched, {reached} reached.\n"

// graphLines is one placeholder per line, each left out when its slot
// stands for nothing.
const graphLines = "{line1}\n{line2}\n{line3}\n{line4}\n{line5}"

// templatesGraph is the one task-preflight row the graph brief fills.
func templatesGraph() map[templateKey]template {
	return map[templateKey]template{
		{pattern.TaskPreflight, reflect.TypeOf(TaskPreflightGraphBinding{})}: {
			id:     "task-preflight-graph/v1",
			format: graphHead + graphLines,
			sparse: true,
		},
	}
}
