package engine

import (
	"path"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// The record claim (W-M1.24, W-M1.31, W-M1.41): a record the request
// names claims the unit its evidence lies in, over a partial claim, a
// claim on a phrase beside the record's id and any claim by word
// coverage; resolve reads it after the phrase loop. Split from roles.go
// for Law 10.

// overRecord is why the unit a named record's evidence lies in takes the
// phrase over the ladder's pick, else "": over a partial claim (W-M1.31,
// N7: "B374 again; fix the root cause" is B374's unit, not the unit that
// declares a Cause), over any claim by coverage on a phrase that holds
// the record's id, whose other words stand beside the record and are not
// the thing (W-M1.24, N8: "Close B400." is B400's, not the unit that
// declares a Close), and over every other claim by word coverage, one
// name covering the whole phrase among them (W-M1.41: the request named
// the record, and word coverage names nothing, E5). A claim by the unit's
// own name stands, and so does a qualified claim that covers its whole
// phrase. A unit the request bars takes nothing (recordUnits).
func (r Roles) overRecord(c corpus, pick UnitPick) string {
	if u := r.recordUnit(c, r.Picks); u == "" || u == pick.Path {
		return ""
	}
	switch {
	case pick.Class == ClassPartial:
		return "a partial claim on " + pick.Path
	case pick.Class != ClassUnit && holdsRecord(r.Target, r.Named):
		return "a claim on " + pick.Path + " beside its id"
	case basisOf(pick.Class) == BasisCoverage && (pick.Class != ClassQualified || !coversWhole(pick)):
		return "a claim by coverage on " + pick.Path
	}
	return ""
}

// recordUnit is the unit the named records' evidence claims on the
// ladder's picks: the one unit it lies in, else the first pick on a unit
// it lies in, so a record that links several units claims the one the
// ladder ranks first among them (W-M1.41); "" when none is. Only a unit
// the request does not bar counts (recordUnits).
func (r Roles) recordUnit(c corpus, picks []UnitPick) string {
	us := r.recordUnits(c)
	if len(us) == 1 {
		return us[0]
	}
	for _, p := range picks {
		if slices.Contains(us, p.Path) {
			return p.Path
		}
	}
	return ""
}

// recordUnits is the units the named records' evidence lies in less those
// the request bars: what the prompt leaves alone is no candidate on any
// path (barred, W-M1.34), the records' claim as the ladder's, so "B374
// again, without touching internal/spend" claims no unit when B374 applies
// in internal/spend alone, and "D176 again, without touching
// internal/spend" claims the other unit D176 applies in (W-M1.41b).
func (r Roles) recordUnits(c corpus) []string {
	bar := r.barred(c)
	return slices.DeleteFunc(slices.Clone(r.Named.KnowledgeUnits), func(u string) bool { return bar[u] })
}

// recordUnitNamed is the unit the request names by its own name among the
// named records' units less the barred ones (recordUnits), for when the
// ladder ranks none of them (W-M1.49): the one whose last segment the
// request's things hold, the test unitNamed applies ("AutoSteer" holds
// steer); "" when none or two or more are named so.
func (r Roles) recordUnitNamed(c corpus, prompt []string) string {
	units, things, got := r.recordUnits(c), r.things(prompt), ""
	for _, u := range units {
		if _, ok := holds(things, Tokens(path.Base(u))); ok {
			if got != "" {
				return ""
			}
			got = u
		}
	}
	return got
}

// RecordClaim is a claim by the records the request names (W-M1.41b):
// IDs are the named records whose evidence lies in a unit the request
// does not bar and Units the units it lies in less the barred ones
// (recordUnits), so a record whose evidence lies only in a barred unit is
// none of the claim's; Barred counts the units the request bars that the
// evidence of IDs also lies in, so the line says the count it states
// leaves them out and names none of them; when Units is more than one,
// First is the basis on which the ladder ranked the claimed unit first
// among them, the cell the brief's lines are banded in (cell), and
// RunnerUp the next of them the ladder ranked. The zero value is no
// record claim.
type RecordClaim struct {
	IDs      []graph.ID
	Units    int
	Barred   int
	First    string
	RunnerUp graph.ID
}

// recordClaim is the claim the named records make on unit u, read off the
// ladder the phrase settled: no pick on another of their units ranks above
// u's (recordUnit), so the next one on the ladder is the runner-up.
func (r Roles) recordClaim(g Reader, c corpus, ladder []UnitPick, u string) RecordClaim {
	us, bar := r.recordUnits(c), r.barred(c)
	rc := RecordClaim{Units: len(us)}
	var barred []string
	for _, k := range r.Named.Knowledge {
		eu := c.evidenceUnits(g, k)
		if !slices.ContainsFunc(eu, func(e string) bool { return slices.Contains(us, e) }) {
			continue
		}
		rc.IDs = append(rc.IDs, k)
		for _, e := range eu {
			if bar[e] && !slices.Contains(barred, e) {
				barred = append(barred, e)
			}
		}
	}
	rc.Barred = len(barred)
	at := slices.IndexFunc(ladder, func(p UnitPick) bool { return p.Path == u })
	if len(us) < 2 || at < 0 {
		return rc
	}
	rc.First = basisOf(ladder[at].Class)
	for _, p := range ladder {
		if p.Path != u && p.Refused == "" && p.ID != "" && slices.Contains(us, p.Path) {
			rc.RunnerUp = p.ID
			break
		}
	}
	return rc
}

// claimBasis is the basis word and the runner-up of the claim settled on
// pick: a record claim's own (W-M1.41b), else the pick's class's and the
// ladder's second pick.
func (r Roles) claimBasis(pick UnitPick) (string, graph.ID) {
	if r.Record.Units > 0 {
		return BasisRecord, r.Record.RunnerUp
	}
	return basisOf(pick.Class), runnerUp(r.Picks)
}

// cell is the basis whose calibration cell bands the brief's lines: the
// claim's own, and for a record claim the basis it rests on, the named
// cell when its records apply in one unit and the ladder's when they
// apply in several; no record cell is calibrated (W-M1.41b). A record
// claim the slot does not state (stated) is banded in the cell of no
// claim, which withholds its lines, so no line is banded on a claim whose
// unit line the slot refuses.
func (r Roles) cell() string {
	switch {
	case r.Basis != BasisRecord:
		return r.Basis
	case !r.Record.stated():
		return BasisNone
	case r.Record.Units > 1:
		return r.Record.First
	}
	return BasisNamed
}

// stated is whether the slot states a unit line on the claim: its record
// rules (validRecord) on the fields recordClaim sets, a record named, a
// unit counted, and a rank by the unit's name or word coverage, with any
// runner-up, only when the records apply in more than one unit; the cap
// on the count is the slot's. Every claim recordClaim makes holds them: a
// claim on one unit ranks nothing, and one on several is ranked by its
// pick on the ladder, which holds no pick by name (a named unit returns
// before the phrases are read, and the record's own pick is made after
// the ladder is taken), or, when the ladder ranks none of them, by the
// unit's name (recordUnitNamed, W-M1.49).
func (rc RecordClaim) stated() bool {
	switch {
	case len(rc.IDs) == 0 || rc.Units < 1:
		return false
	case rc.Units == 1:
		return rc.First == "" && rc.RunnerUp == ""
	}
	return rc.First == BasisUnitName || rc.First == BasisCoverage
}

// unitUnknown is the kinds the unit line names as not known, of those
// withheld (unknown): a record claim's line names no knowledge kind when
// every knowledge line withheld (out) is one of its records, which the
// line names as applying (W-M1.41b); when one is not, the slot says the
// records not known are other records.
func (r Roles) unitUnknown(unknown []string, out []graph.ID) []string {
	if r.Basis != BasisRecord || len(out) == 0 {
		return unknown
	}
	for _, k := range out {
		if !slices.Contains(r.Record.IDs, k) {
			return unknown
		}
	}
	var kept []string
	for _, k := range unknown {
		if k != RoleKnowledge {
			kept = append(kept, k)
		}
	}
	return kept
}

// ladderStands is whether the ladder's pick on a phrase stands as the
// claim when no named record outranks it (overRecord), and makes it a
// record claim when a named record applies in its unit (W-M1.41c): the
// pick stands as the ladder made it when the records' unit is another or
// none; as their claim, banded in the ladder's cell, when they apply in
// several units; and as their claim on the pick's evidence they apply to
// (held) when they apply in its unit alone. It does not stand when they
// apply to none of that evidence: their own evidence claims the unit
// (resolve), since a claim on one unit is banded in the named cell and
// no line it states may rest on the words alone.
func (r *Roles) ladderStands(g Reader, c corpus) bool {
	u := r.recordUnit(c, r.Picks)
	if u == "" || u != r.Picks[0].Path {
		return true
	}
	rc := r.recordClaim(g, c, r.Picks, u)
	if rc.Units > 1 {
		r.Record = rc
		return true
	}
	if held := c.held(g, rc, r.Picks[0]); len(held) > 0 {
		r.Picks[0].Nodes, r.Record = held, rc
		return true
	}
	return false
}

// held is the evidence of the pick the ladder settled on a unit that a
// record claim on that unit alone keeps (W-M1.41c), in the pick's order:
// all of it when one of the claim's records applies to the unit as a
// whole, else what lies in a file one of them applies to, a symbol's
// declaring file or the file itself.
func (c corpus) held(g Reader, rc RecordClaim, pick UnitPick) []graph.ID {
	files := map[graph.ID]bool{}
	for _, k := range rc.IDs {
		for _, e := range g.Out(k, c.rev, graph.AppliesTo) {
			if c.unitOf(e.Dst) != pick.Path {
				continue
			}
			if strings.HasPrefix(string(e.Dst), "unit:") {
				return pick.Nodes
			}
			files[declaring(g, c.rev, e.Dst)] = true
		}
	}
	delete(files, "")
	return slices.DeleteFunc(slices.Clone(pick.Nodes), func(n graph.ID) bool { return !files[declaring(g, c.rev, n)] })
}

// holdsRecord is whether the phrase holds a word that named a record, as
// the prompt wrote it.
func holdsRecord(target []string, n Named) bool {
	for _, w := range n.Words {
		if w.Kind != NamedKnowledge {
			continue
		}
		for _, t := range Tokens(w.Word) {
			if hasToken(target, t) {
				return true
			}
		}
	}
	return false
}
