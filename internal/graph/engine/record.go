package engine

import "slices"

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
// phrase.
func (r Roles) overRecord(pick UnitPick) string {
	if u := r.recordUnit(r.Picks); u == "" || u == pick.Path {
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
// ladder ranks first among them (W-M1.41); "" when none is.
func (r Roles) recordUnit(picks []UnitPick) string {
	us := r.Named.KnowledgeUnits
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
