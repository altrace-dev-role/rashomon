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
	c = c.without(r.barred(c))
	picks := Units(g, c, r.Target, r.outside(prompt), weights)
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

// barred is the units an exclusion names by their own name ("without
// touching the store"): what the prompt leaves alone is no candidate on
// any path, a filter before the pick and not a lighter weight, and the
// ladder refuses rather than settle among what is left when only a
// barred unit covered the head. A kept thing ("keep the store tests
// green") and a property ("the spec file format") bar nothing
// (W-M1.34).
func (r Roles) barred(c corpus) map[string]bool {
	out := map[string]bool{}
	for _, p := range r.Frame.Constraints {
		if !p.Excluded || p.property() {
			continue
		}
		if u, _, ok := c.unitNamed(p.Tokens); ok {
			out[u] = true
		}
	}
	return out
}

// without is the corpus with the barred units no claim or name may pick.
func (c corpus) without(barred map[string]bool) corpus {
	c.barred = barred
	return c
}

// leftAlone is why a claim refuses when the one unit that would have
// taken it is what the prompt leaves alone: the one place it forbids is
// never a guess.
func leftAlone(u string) string {
	return "'" + u + "' is named only as what the change leaves alone"
}

// outside is the prompt's tokens less the target's, the constraints' and
// the aside: a constraint names what the change leaves alone and the
// aside carries it, so their words say where the work is not and weigh
// nothing in which unit the target is (W-M1.33, O7).
func (r Roles) outside(prompt []string) []string {
	drop := append([]string{}, r.Target...)
	for _, p := range r.Frame.Constraints {
		drop = append(drop, p.Tokens...)
	}
	drop = append(drop, r.Frame.Aside...)
	return except(prompt, drop)
}

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

// unitNamed is the unit whose own name the phrase holds: its last path
// segment as the phrase's words, or as two adjacent words run together
// ("gate output" names gateout, "budgets gen" budgetsgen). Among several,
// the one named by more of the phrase's words, then the one whose other
// segments the phrase also names. Two still tied where one is the
// phrase's head and the other the word before it are a compound of unit
// names ("the spend report", W-M1.31 K9): the modifier claims and the
// head is the kind of thing, as a lone unit name claims as a modifier,
// and no third unit takes the phrase for the word beside them. Any other
// tie is the ladder's to settle. A word spelled from a unit's name and
// more names the unit when the unit also declares the head
// (compoundNamed, W-M1.33 E5).
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
		if !ok || c.barred[u] {
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
		return c.compoundNamed(target)
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
	n := 1
	for n < len(hits) && len(hits[n].toks) == len(hits[0].toks) && hits[n].more == hits[0].more {
		n++
	}
	if n == 1 {
		return hits[0].unit, hits[0].toks, true
	}
	if len(target) < 2 {
		return "", nil, false
	}
	head, before := target[len(target)-1], target[len(target)-2]
	var kind, modifier []int
	for i := 0; i < n; i++ {
		switch {
		case hasToken(hits[i].toks, head):
			kind = append(kind, i)
		case hasToken(hits[i].toks, before):
			modifier = append(modifier, i)
		}
	}
	if len(kind) != 1 || len(modifier) != 1 {
		return "", nil, false
	}
	return hits[modifier[0]].unit, hits[modifier[0]].toks, true
}

// compoundNamed is the unit a word of the phrase is spelled from, the
// unit's name and three letters or more ("hostname" from host), when
// the unit also declares the phrase's head: the name accounts for the
// modifier and the declaration for the head, so the unit is named by
// the whole phrase (W-M1.33, E5). The spelling alone names nothing
// ("the performance counter" is not perf's when perf declares no
// counter), and two such units name nothing.
func (c corpus) compoundNamed(target []string) (string, []string, bool) {
	if len(target) < 2 {
		return "", nil, false
	}
	head := target[len(target)-1]
	declaring := c.unitsDeclaring(head)
	unit, word := "", ""
	for u := range c.unitID {
		seg := Tokens(path.Base(u))
		if c.barred[u] || len(seg) != 1 || len(seg[0]) < 4 || !declaring[u] {
			continue
		}
		for _, t := range target[:len(target)-1] {
			if len(t) < len(seg[0])+3 || !strings.HasPrefix(t, seg[0]) {
				continue
			}
			if unit != "" && unit != u {
				return "", nil, false
			}
			unit, word = u, t
		}
	}
	if unit == "" {
		return "", nil, false
	}
	return unit, []string{word, head}, true
}

// unitsDeclaring is the units with a name that holds the token.
func (c corpus) unitsDeclaring(t string) map[string]bool {
	out := map[string]bool{}
	for i := range c.nodes {
		if hasToken(c.fields[i].Name, t) {
			out[c.units[i]] = true
		}
	}
	return out
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
