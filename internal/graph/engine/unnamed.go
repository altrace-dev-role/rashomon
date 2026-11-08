package engine

import (
	"path"
	"strings"
)

// The unit rule (PLAN W-M1.38): a unit claimed on a request that names no
// unit is withheld, so the brief says the unit is not known in the unit
// line's place instead of naming one its own record states too seldom
// (bands.go). A rule reads the request and the graph's names only, never
// a truth, inside the hook's budget. E1 and E3 were registered on 11-08
// before any reading of them; E4 was proposed later that day, after a
// simulation on the development dumps, so its reading there is not a
// held-out one. E5 (W-M1.40, AC-M1.5 as re-specified from VISION) was
// chosen after reading both untuned files, so classes4.tsv is its only
// held-out read; it ships without its knowledge clause (W-M1.41).

// unitRule is whether the request names no unit, read from the corpus,
// the roles found and the request.
type unitRule func(c corpus, r Roles, prompt string) bool

// shippedUnitRule is the rule Build applies when Options sets none.
var shippedUnitRule unitRule = unitRuleE5

// noUnitRule finds every request names its unit: nothing is withheld by
// rule. The tests that read the lines of a unit the shipped rule
// withholds, Task A's among them, set it.
func noUnitRule(corpus, Roles, string) bool { return false }

// unitRuleE1 is E1: a unit was found, no unit was named and no claim is
// by a unit's own name, so the target phrase holds no unit's whole name
// and the unit stands on word coverage alone.
func unitRuleE1(_ corpus, r Roles, _ string) bool {
	if !r.Found() || r.Named.Unit != "" {
		return false
	}
	for _, p := range r.Picks {
		if p.Class == ClassNamed || p.Class == ClassUnit {
			return false
		}
	}
	return true
}

// unitRuleE3 is E3 over E1's claims: the request also holds no unit's
// path or last segment and no file's base name, case-folded and with no
// letter or digit beside it.
func unitRuleE3(c corpus, r Roles, prompt string) bool {
	return unitRuleE1(c, r, prompt) && !c.holdsAName(strings.ToLower(prompt))
}

// unitRuleE4 is E4: a unit was found by a claim that is not by name, and
// the request holds neither the claimed unit's path nor its last segment
// (namesUnit), so a unit the request names in a phrase the ladder did not
// read as the target still stands.
func unitRuleE4(_ corpus, r Roles, prompt string) bool {
	return r.Found() && r.Basis != BasisNamed && !namesUnit(strings.ToLower(prompt), r.Path)
}

// unitRuleE5 is E5 without its knowledge clause (W-M1.40): a unit stands
// only on what the request names. A named basis stands (a path, a file, a
// symbol, a knowledge id, a pasted error); so does a qualified claim that
// covers the whole target phrase (the unit's path covers part of it and
// one of its names the rest, T1's form), and a claim by the unit's own
// name in a target phrase of at most two tokens. Every other claim is
// withheld: on the two untuned files no margin on the ladder's scores
// separated a right claim from a wrong one, and the forms withheld here
// were right under half the time (PLAN 6.1).
func unitRuleE5(_ corpus, r Roles, _ string) bool {
	if !r.Found() || r.Basis == BasisNamed {
		return false
	}
	if len(r.Picks) > 0 && r.Picks[0].Class == ClassQualified && coversWhole(r.Picks[0]) {
		return false
	}
	return r.Basis != BasisUnitName || len(r.Target) > 2
}

// coversWhole is whether a pick covers its whole phrase. Covered sums the
// covered tokens' weights in sorted order and Of the phrase's in its own
// order, with the weight of words no name holds added, so two sums of the
// same weights can differ in their last bits: within a relative 1e-9 they
// are equal.
func coversWhole(p UnitPick) bool {
	return p.Of > 0 && p.Of-p.Covered <= 1e-9*p.Of
}

// holdsAName is whether the folded request p holds a unit's path or last
// segment (namesUnit), or a file's base name.
func (c corpus) holdsAName(p string) bool {
	for u := range c.unitID {
		if namesUnit(p, u) {
			return true
		}
	}
	for f := range c.files {
		if holdsWord(p, strings.ToLower(path.Base(f))) {
			return true
		}
	}
	return false
}

// namesUnit is whether the folded request p holds the unit path u's last
// segment, case-folded and with no letter or digit beside it, as it does
// wherever it holds the whole path; the root unit's "." names nothing.
func namesUnit(p, u string) bool {
	if u == "" || u == "." || u == ".." {
		return false
	}
	return holdsWord(p, path.Base(strings.ToLower(u)))
}

// holdsWord is whether w occurs in s with no ASCII letter or digit next to
// it on either side; an empty w names nothing.
func holdsWord(s, w string) bool {
	if w == "" {
		return false
	}
	for i := 0; i+len(w) <= len(s); {
		j := strings.Index(s[i:], w)
		if j < 0 {
			return false
		}
		at, end := i+j, i+j+len(w)
		if (at == 0 || !alnumByte(s[at-1])) && (end == len(s) || !alnumByte(s[end])) {
			return true
		}
		i = at + 1
	}
	return false
}

func alnumByte(b byte) bool {
	return 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || '0' <= b && b <= '9'
}
