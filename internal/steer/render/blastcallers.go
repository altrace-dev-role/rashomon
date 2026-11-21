package render

// The caller note: at the post of an edit to a function in a language the
// repository graph indexes, the callers the graph names for that function.
// The stale rows add that the graph does not hold the present content of
// the edited file. The record rows hold counts only, so the steer line
// carries no name. Kept apart from render.go so that file stays inside
// Law 10's 500 code lines.

import (
	"reflect"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// BlastCallersBinding is the caller note: the callers the graph names, and
// the function the call edited.
type BlastCallersBinding struct {
	Callers  slot.IdentList `slot:"callers"`
	Function slot.Ident     `slot:"function"`
}

// BlastCallersStaleBinding is BlastCallersBinding where the graph does not
// hold the edited file's present content.
type BlastCallersStaleBinding struct {
	Callers  slot.IdentList `slot:"callers"`
	Function slot.Ident     `slot:"function"`
}

// BlastCallersRecordBinding is the note's record form: the count of names
// listed and the count the list leaves out.
type BlastCallersRecordBinding struct {
	Listed slot.Count `slot:"listed"`
	More   slot.Count `slot:"more"`
}

// BlastCallersStaleRecordBinding is BlastCallersRecordBinding for the stale
// note.
type BlastCallersStaleRecordBinding struct {
	Listed slot.Count `slot:"listed"`
	More   slot.Count `slot:"more"`
}

// blastCallersNote is the note's text up to its last period: the callers
// the graph names, as calling the function the call edited.
const blastCallersNote = "rashomon record: the repository graph names {callers} as calling {function}, " +
	"which this call edited"

// blastCallersCounts is the record's text up to its last period: the two
// counts, and no name.
const blastCallersCounts = "rashomon record: the repository graph names callers of the function this call edited: " +
	"{listed} listed, {more} more"

// blastCallersStale ends a stale row in place of the period: the graph does
// not hold the edited file's present content.
const blastCallersStale = "; the graph does not hold the present content of the edited file."

// templatesBlastCallers is the caller note's rows. Each states what the
// graph holds, never an order (STR-14).
func templatesBlastCallers() map[templateKey]template {
	return map[templateKey]template{
		{pattern.BlastRadius, reflect.TypeOf(BlastCallersBinding{})}: {
			id:     "blast-callers/v1",
			format: blastCallersNote + ".",
		},
		{pattern.BlastRadius, reflect.TypeOf(BlastCallersStaleBinding{})}: {
			id:     "blast-callers-stale/v1",
			format: blastCallersNote + blastCallersStale,
		},
		{pattern.BlastRadius, reflect.TypeOf(BlastCallersRecordBinding{})}: {
			id:     "blast-callers-record/v1",
			format: blastCallersCounts + ".",
		},
		{pattern.BlastRadius, reflect.TypeOf(BlastCallersStaleRecordBinding{})}: {
			id:     "blast-callers-stale-record/v1",
			format: blastCallersCounts + blastCallersStale,
		},
	}
}
