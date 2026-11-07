package engine

import (
	"path"
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// claim is one target phrase's claim on a unit, with why: the unit whose
// own name the phrase holds (W-M1.14, the strongest lexical signal: "the
// recap decision" is recap's, whatever declares a decision); else the
// ladder's claim, a tie among units settled by a record the prompt named
// when its evidence lies in one of them. An owner phrase ("the seed
// counter's ...") and the whole-prompt fallback claim by a unit's name
// alone: the ladder's reading of their words is a guess.
func (r *Roles) claim(g Reader, c corpus, p Phrase, prompt []string, weights map[string]float64) ([]UnitPick, string) {
	picks := Units(g, c, r.Target, except(prompt, r.Target), weights)
	if u, toks, ok := c.unitNamed(r.Target); ok && (r.Fallback || nameClaims(picks, u, toks, r.Target)) {
		picks = unitFirst(c, picks, u, toks)
		return picks, picks[0].Class
	}
	if r.Fallback {
		return nil, "no unit is named"
	}
	switch {
	case len(picks) == 0:
		if p.Owner {
			return refused("the owner '" + strings.Join(r.Target, " ") + "' names no unit"), "refused: the owner '" + strings.Join(r.Target, " ") + "' names no unit"
		}
		return nil, "no unit names the target"
	case picks[0].Refused != "":
		if settled, why, ok := knowledgeSettle(g, c, picks, r.Named.Knowledge); ok {
			return settled, why
		}
		return picks, "refused: " + picks[0].Refused
	}
	return picks, picks[0].Class
}

func refused(why string) []UnitPick { return []UnitPick{{Class: ClassPartial, Refused: why}} }

// nameClaims is whether the unit a phrase names by its own segment takes
// the phrase over the ladder's reading. A directory name is one word of
// evidence. As the whole phrase ("peak rss", "budgets generator") it is
// the claim; as the phrase's head ("the integrity report", "the
// continuity briefing") it is the kind of thing, and it yields to a
// unit that declares the thing itself: a name wholly in the phrase
// (verify's integrity for the integrity report) or one that completes
// it, never to a unit whose names merely touch the words ("the test
// harness" stays harness's over a testBending elsewhere); as a modifier
// ("the recap decision" is recap's, whatever declares a decision) it
// claims, unless one declared name elsewhere is the whole phrase
// (platformObligations in verify over the platform unit), which
// outweighs a directory that is one word of it.
func nameClaims(picks []UnitPick, u string, toks, target []string) bool {
	if len(toks) == len(target) {
		return true
	}
	for _, p := range picks {
		if p.Path == u {
			if p.Refused == "" && (p.Class == ClassUnit || p.Class == ClassQualified) {
				return true
			}
			break
		}
	}
	if len(picks) == 0 || picks[0].Refused != "" {
		return true
	}
	if hasToken(toks, target[len(target)-1]) {
		return classRank(picks[0].Class) > classRank(ClassSingle) && picks[0].Precision < 1
	}
	return classRank(picks[0].Class) > classRank(ClassSingle)
}

// unitNamed is the one unit whose own name the phrase holds: its last
// path segment as the phrase's words, or as two adjacent words run
// together ("gate output" names gateout, "budgets gen" budgetsgen).
// Among several, the one named by more of the phrase's words, then the
// one whose other segments the phrase also names; a tie is the ladder's
// to settle.
func (c corpus) unitNamed(target []string) (string, []string, bool) {
	type hit struct {
		unit string
		toks []string
		more int
	}
	var hits []hit
	for u := range c.unitID {
		seg := Tokens(path.Base(u))
		toks, ok := holds(target, seg)
		if !ok {
			continue
		}
		more := 0
		for _, t := range Tokens(path.Dir(u)) {
			if hasToken(target, t) {
				more++
			}
		}
		hits = append(hits, hit{u, toks, more})
	}
	if len(hits) == 0 {
		return "", nil, false
	}
	sort.Slice(hits, func(i, j int) bool {
		if len(hits[i].toks) != len(hits[j].toks) {
			return len(hits[i].toks) > len(hits[j].toks)
		}
		if hits[i].more != hits[j].more {
			return hits[i].more > hits[j].more
		}
		return hits[i].unit < hits[j].unit
	})
	if len(hits) > 1 && len(hits[0].toks) == len(hits[1].toks) && hits[0].more == hits[1].more {
		return "", nil, false
	}
	return hits[0].unit, hits[0].toks, true
}

// holds is the words of target that are seg: seg's tokens in a row, or
// for a one-token seg two adjacent words run together (with or without a
// plural s between), either a prefix of the other at five letters or
// more and at least half its length.
func holds(target, seg []string) ([]string, bool) {
	if len(seg) == 0 || len(target) < len(seg) {
		return nil, false
	}
	for i := 0; i+len(seg) <= len(target); i++ {
		if strings.Join(target[i:i+len(seg)], " ") == strings.Join(seg, " ") {
			return seg, true
		}
	}
	if len(seg) != 1 || len(seg[0]) < 5 {
		return nil, false
	}
	s := seg[0]
	for i := 0; i+1 < len(target); i++ {
		a, b := target[i], target[i+1]
		for _, ab := range []string{a + b, a + "s" + b} {
			if len(ab) < 5 || 2*min(len(ab), len(s)) < max(len(ab), len(s)) {
				continue
			}
			if strings.HasPrefix(ab, s) || strings.HasPrefix(s, ab) {
				return []string{a, b}, true
			}
		}
	}
	return nil, false
}

// unitFirst is picks with u's claim first and whole: the ladder's own
// claim when it already read the name (unit or qualified, first and
// unrefused), else the ladder's nodes for u under a whole claim, else a
// bare claim on the unit.
func unitFirst(c corpus, picks []UnitPick, u string, toks []string) []UnitPick {
	for i, p := range picks {
		if p.Path != u {
			continue
		}
		if i == 0 && p.Refused == "" && (p.Class == ClassUnit || p.Class == ClassQualified) {
			return picks
		}
		p.Class, p.Refused, p.Tied, p.Covered = ClassUnit, "", 0, p.Of
		for _, t := range toks {
			if !hasToken(p.Tokens, t) {
				p.Tokens = append(p.Tokens, t)
			}
		}
		if i > 0 {
			picks[0].Refused, picks[0].Tied = "", 0
		}
		return append([]UnitPick{p}, append(picks[:i:i], picks[i+1:]...)...)
	}
	pick := UnitPick{ID: c.unitID[u], Path: u, Class: ClassUnit, Covered: 1, Of: 1, Callable: true, Precision: 1, Tokens: toks}
	if len(picks) > 0 {
		picks[0].Refused, picks[0].Tied = "", 0
	}
	return append([]UnitPick{pick}, picks...)
}

// knowledgeSettle breaks a tie among the leading picks by a record the
// prompt named: the one tied unit its evidence lies in.
func knowledgeSettle(g Reader, c corpus, picks []UnitPick, ids []graph.ID) ([]UnitPick, string, bool) {
	n := picks[0].Tied
	if n < 2 {
		return nil, "", false
	}
	for _, id := range ids {
		units := map[string]bool{}
		for _, e := range g.Out(id, c.rev, graph.AppliesTo) {
			if u := c.unitOf(e.Dst); u != "" {
				units[u] = true
			}
		}
		at := -1
		for i := 0; i < n; i++ {
			if units[picks[i].Path] {
				if at >= 0 {
					at = -1
					break
				}
				at = i
			}
		}
		if at >= 0 {
			picks[0], picks[at] = picks[at], picks[0]
			picks[0].Refused, picks[0].Tied = "", 0
			picks[at].Refused, picks[at].Tied = "", 0
			return picks, picks[0].Class + "; tie settled by " + short(id), true
		}
	}
	return nil, "", false
}
