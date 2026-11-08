package engine

import (
	"slices"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// Abstention by evidence (PLAN W-M1.38): a line is stated only as far as
// the engine's own record on lines like it allows. A unit line of a basis,
// or a role line of a kind under a unit of a basis, is stated when that
// cell's delivered precision is at least StatedFloor, a candidate from
// CandidateFloor, and withheld below it: the line is not delivered, and
// the unit line names the kinds withheld.
const (
	StatedFloor    = 0.8
	CandidateFloor = 0.5
)

// The bands: the two a line is delivered in are the slot's.
const (
	BandStated    = slot.BandStated
	BandCandidate = slot.BandCandidate
	BandWithheld  = "withheld"
)

// tally is the lines of a cell judged right, of those judged.
type tally struct{ right, judged int }

// band is the band a tally's precision falls in; a cell with nothing
// judged states nothing.
func (t tally) band() string {
	if t.judged == 0 {
		return BandWithheld
	}
	switch p := float64(t.right) / float64(t.judged); {
	case p >= StatedFloor:
		return BandStated
	case p >= CandidateFloor:
		return BandCandidate
	}
	return BandWithheld
}

// calibration is delivered precision by line kind and unit basis, read at
// engine 55fcef02 (the engine of 28ca31b9) over the kept lines of every row
// of classes.tsv and classes2.tsv (320 rows; plain tree, Options{IDF:
// true}), with the kinds and their right rules registered before the
// reading ran. A unit line's cell is its own basis; a role line's is its
// kind under the basis of the unit line it hangs off, the granularity PLAN
// W-M1.38 rules after the reading showed a kind's pooled precision mixes
// lines under right units with lines under wrong ones (target 36 of 37
// under a named unit, 94 of 195 pooled).
var calibration = map[[2]string]tally{
	{RoleUnit, BasisNamed}:         {71, 75},
	{RoleUnit, BasisUnitName}:      {57, 81},
	{RoleUnit, BasisCoverage}:      {56, 105},
	{RoleTarget, BasisNamed}:       {36, 37},
	{RoleTarget, BasisUnitName}:    {21, 64},
	{RoleTarget, BasisCoverage}:    {37, 94},
	{RoleReuse, BasisNamed}:        {0, 6},
	{RoleReuse, BasisUnitName}:     {1, 8},
	{RoleReuse, BasisCoverage}:     {2, 17},
	{RoleConsumer, BasisNamed}:     {69, 73},
	{RoleConsumer, BasisUnitName}:  {62, 76},
	{RoleConsumer, BasisCoverage}:  {60, 103},
	{RoleKnowledge, BasisNamed}:    {105, 111},
	{RoleKnowledge, BasisUnitName}: {57, 92},
	{RoleKnowledge, BasisCoverage}: {75, 156},
}

// bandFunc is the band of a cell: a line of kind under a unit claimed on
// basis.
type bandFunc func(kind, basis string) string

// calibrated is calibration's band for a cell; a cell it lacks states
// nothing.
func calibrated(kind, basis string) string {
	return calibration[[2]string{kind, basis}].band()
}

// lower is the less certain of two bands.
func lower(a, b string) string {
	rank := map[string]int{BandWithheld: 0, BandCandidate: 1, BandStated: 2}
	if rank[a] < rank[b] {
		return a
	}
	return b
}

// withhold bands the merged lines and takes out the withheld ones. The
// unit line's band is its basis's cell; a role line's is its kind's cell
// under the unit's basis, capped at the unit line's band, since a role
// line is right only when its unit is. A line taken out leaves its slot
// empty, so no related line moves up into it; the unit line names the
// kinds taken out (Unknown), and a line found through a file taken out is
// anchored to the unit instead, so a record names the unit where it
// applies within it and that file only where the record itself applies to
// it first (knowledgeFact). A decision on a line taken out says so; the
// rest are renumbered to the lines kept. A unit withheld takes every role
// line with it and leaves the related lines, with no unit line to name
// what is not known (the calibration withholds no unit basis).
func withhold(lines []Line, decisions []Decision, r Roles, band bandFunc) ([]Line, []Decision) {
	unit := band(RoleUnit, r.Basis)
	kept := make([]Line, 0, len(lines))
	at := make([]int, len(lines)+1)
	why := make([]string, len(lines)+1)
	gone := map[graph.ID]bool{}
	var unknown []string
	for i, l := range lines {
		switch l.Role {
		case "":
		case RoleUnit:
			l.Band = unit
		default:
			l.Band = lower(band(l.Role, r.Basis), unit)
		}
		if l.Band == BandWithheld {
			gone[l.Key] = true
			why[i+1] = "withheld: role " + l.Role
			if l.Role != RoleUnit && !slices.Contains(unknown, l.Role) {
				unknown = append(unknown, l.Role)
			}
			continue
		}
		kept = append(kept, l)
		at[i+1] = len(kept)
	}
	anchor := r.Unit
	if gone[r.Unit] {
		anchor = ""
	}
	for i := range kept {
		if kept[i].Role == RoleUnit {
			kept[i].Unknown = unknown
		}
		if kept[i].Anchor != "" && gone[kept[i].Anchor] {
			kept[i].Anchor = anchor
		}
	}
	for i, d := range decisions {
		switch {
		case d.Line <= 0 || d.Line >= len(at):
		case why[d.Line] != "":
			decisions[i] = Decision{ID: d.ID, Why: why[d.Line]}
		default:
			decisions[i].Line = at[d.Line]
		}
	}
	return kept, decisions
}
