package engine

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// AC-M1.5 gate shape 3 (PLAN 7.7, registered 2026-11-08 before any
// classes3.tsv row existed; shape 2's read on classes2.tsv stands as not
// met). M1 claims the unit where the request names it, and claims only
// that it states nothing false where the request describes behavior in
// prose. A row is a naming row when its class names its unit by
// construction or its prompt names a truth unit or target file
// (namesTruth), a no-unit row when its unit truth is '-', and a prose row
// otherwise. A naming or no-unit row's unit verdict is shape 2's; a prose
// row's is right when the brief claims no unit, or its claim holds a
// truth. The role lines are judged on the rows whose brief unit is a truth
// (roleCount's given-unit counts: a wrong or withheld unit's role lines
// are that unit's error, counted once). Gates per class: the unit verdict
// in 7 of 10 rows, and each role in 7 of 10 of the rows judged, rounded
// up, where at least three are judged. Gates over the set: each role in 7
// of 10 of the rows judged; stated unit lines right in 8 of 10 of those
// stated; candidate lines holding a truth in 5 of 10 of those delivered
// (W-M1.38's floors).

// The kinds of truth row gate shape 3 reads.
const (
	kindNaming = "naming"
	kindProse  = "prose"
	kindNoUnit = "no-unit"
)

// namingClasses are the claimed classes whose requests name their unit by
// construction: by a declared name, a path, a pasted error or a knowledge
// id.
var namingClasses = []string{"named-name", "named-path", "pasted-error", "knowledge-id"}

// rowKind is a truth row's kind: no-unit when its unit truth is '-',
// naming when its class is one of namingClasses or namesTruth holds, prose
// otherwise.
func rowKind(r []string) string {
	switch {
	case r[2] == "-":
		return kindNoUnit
	case slices.Contains(namingClasses, r[1]) || namesTruth(r):
		return kindNaming
	}
	return kindProse
}

// namesTruth is whether a row's prompt, case-folded, holds a truth unit's
// path as a substring, or a truth unit's last path segment or a target
// file's name with no letter or digit next to it on either side (W-M1.37's
// split, split.py's alnum mode: learn_test.go names learn, relearn does
// not).
func namesTruth(r []string) bool {
	return promptNames(strings.ReplaceAll(r[8], "\\n", "\n"), strings.Split(r[2], ";"), strings.Split(r[3], ";"))
}

// promptNames is namesTruth over a prompt, its unit truths and its target
// files ('-', '*' and empty entries name nothing), for the files whose
// rows are not classes rows (AC-M1.1's, which carry no target file).
func promptNames(prompt string, units, files []string) bool {
	p := strings.ToLower(prompt)
	var keys []string
	for _, u := range units {
		if u != "-" && u != "*" {
			u = strings.ToLower(u)
			keys = append(keys, u, lastSegment(u))
		}
	}
	for _, f := range files {
		if f != "-" && f != "*" && f != "" {
			keys = append(keys, lastSegment(strings.ToLower(f)))
		}
	}
	for _, k := range keys {
		if strings.Contains(k, "/") && strings.Contains(p, k) || !strings.Contains(k, "/") && bounded(p, k) {
			return true
		}
	}
	return false
}

// lastSegment is a slash path's last segment.
func lastSegment(p string) string {
	return p[strings.LastIndex(p, "/")+1:]
}

// bounded is whether k occurs in p with no ASCII letter or digit next to
// it on either side; an empty k names nothing.
func bounded(p, k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i+len(k) <= len(p); {
		j := strings.Index(p[i:], k)
		if j < 0 {
			return false
		}
		s, e := i+j, i+j+len(k)
		if (s == 0 || !isAlnum(p[s-1])) && (e == len(p) || !isAlnum(p[e])) {
			return true
		}
		i = s + 1
	}
	return false
}

func isAlnum(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}

// briefClaim is what a brief claims about the unit: the unit it names
// (briefUnit's: its unit line's, else the unit of its first kept line, so
// a brief that withholds its unit line and keeps a role line still claims
// that line's unit), whether it holds a unit line, and whether that line
// is in the candidate band (W-M1.38), so the brief offers the unit and its
// runner-up as a pair, or the unit alone when it names none.
type briefClaim struct {
	Unit, Runner    string
	Line, Candidate bool
}

// claimOf is brief b's unit claim, unit being briefUnit's answer.
func claimOf(b Brief, unit string) briefClaim {
	c := briefClaim{Unit: unit}
	for _, l := range b.Lines[:b.Kept] {
		if u, ok := l.Slot.Unit(); ok {
			c.Line, c.Runner = true, string(u.RunnerUp)
			c.Candidate = u.Band == slot.BandCandidate
			break
		}
	}
	return c
}

// holds is whether the claim names one of want: its unit, or for a
// candidate its runner-up when it names one.
func (c briefClaim) holds(want []string) bool {
	return c.Unit != "" && slices.Contains(want, c.Unit) || c.Candidate && c.Runner != "" && slices.Contains(want, c.Runner)
}

func (c briefClaim) String() string {
	switch {
	case c.Unit == "":
		return "claims no unit"
	case c.Candidate && c.Runner != "":
		return fmt.Sprintf("candidate %q or %q", c.Unit, c.Runner)
	case c.Candidate:
		return fmt.Sprintf("candidate %q", c.Unit)
	case c.Line:
		return fmt.Sprintf("states %q", c.Unit)
	}
	return fmt.Sprintf("no unit line, first line in %q", c.Unit)
}

// unitRight3 is gate shape 3's unit verdict: shape 2's (unitOK) on a
// naming or no-unit row; on a prose row, right when the brief states no
// wrong unit: it claims none, or its claim holds a truth.
func unitRight3(kind string, want []string, c briefClaim, unitOK bool) bool {
	if kind != kindProse {
		return unitOK
	}
	return c.Unit == "" || c.holds(want)
}

// shape3Score is a class's tally under gate shape 3, or the set's.
type shape3Score struct {
	N, UnitRight                int
	Naming, NamingRight         int
	Prose, ProseRight, Withheld int
	NoUnit, NoUnitRight         int
	Stated, StatedRight         int
	Candidate, CandidateTruth   int
	Reuse, Consumers, Knowledge roleCount
}

// add tallies one row of kind with unit truth want, and returns its unit
// verdict.
func (s *shape3Score) add(kind string, want []string, v rowVerdicts) bool {
	c := v.Claim
	right := unitRight3(kind, want, c, v.UnitOK)
	s.N++
	s.UnitRight += btoi(right)
	switch kind {
	case kindNaming:
		s.Naming++
		s.NamingRight += btoi(right)
	case kindProse:
		s.Prose++
		s.ProseRight += btoi(right)
		s.Withheld += btoi(c.Unit == "")
	default:
		s.NoUnit++
		s.NoUnitRight += btoi(right)
	}
	switch {
	case c.Candidate:
		s.Candidate++
		s.CandidateTruth += btoi(c.holds(want))
	case c.Line:
		s.Stated++
		s.StatedRight += btoi(slices.Contains(want, c.Unit))
	}
	s.Reuse.add(v.UnitOK, v.Reuse)
	s.Consumers.add(v.UnitOK, v.Consumers)
	s.Knowledge.add(v.UnitOK, v.Knowledge)
	return right
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}

type namedRole struct {
	name string
	c    roleCount
}

func (s shape3Score) roles() []namedRole {
	return []namedRole{{"reuse", s.Reuse}, {"consumers", s.Consumers}, {"knowledge", s.Knowledge}}
}

// missed names the class gates s misses: the unit verdict in 7 of 10 rows,
// each role in 7 of 10 of the rows judged on a truth unit where at least
// three are, rounded up.
func (s shape3Score) missed() []string {
	var out []string
	if s.UnitRight < ceilShare(s.N, 7, 10) {
		out = append(out, "unit")
	}
	for _, r := range s.roles() {
		if r.c.JudgedUnit >= 3 && r.c.RightUnit < ceilShare(r.c.JudgedUnit, 7, 10) {
			out = append(out, r.name)
		}
	}
	return out
}

// pooledMissed names the set gates s misses: each role in 7 of 10 of the
// rows judged on a truth unit, stated unit lines right in 8 of 10 of
// those stated, candidate lines holding a truth in 5 of 10 of those
// delivered, rounded up.
func (s shape3Score) pooledMissed() []string {
	var out []string
	for _, r := range s.roles() {
		if r.c.RightUnit < ceilShare(r.c.JudgedUnit, 7, 10) {
			out = append(out, r.name)
		}
	}
	if s.StatedRight < ceilShare(s.Stated, 8, 10) {
		out = append(out, "stated")
	}
	if s.CandidateTruth < ceilShare(s.Candidate, 5, 10) {
		out = append(out, "candidate")
	}
	return out
}

func frac(n, d int) string {
	if d == 0 {
		return "-"
	}
	return fmt.Sprintf("%d/%d", n, d)
}

func (s shape3Score) row(name string) string {
	g := func(c roleCount) string { return frac(c.RightUnit, c.JudgedUnit) }
	return fmt.Sprintf("%-14s %3d  %3d %3d %2d  %-6s  %-6s  %-6s  %-9s  %-9s  %-9s  %-6s",
		name, s.N, s.Naming, s.Prose, s.NoUnit, frac(s.UnitRight, s.N), frac(s.NamingRight, s.Naming), frac(s.ProseRight, s.Prose),
		g(s.Reuse), g(s.Consumers), g(s.Knowledge), frac(s.Withheld, s.Prose))
}

// shape3Table prints the gate shape 3 table of the claimed classes and the
// set, failing t on every gate missed.
func shape3Table(t *testing.T, scores map[string]*shape3Score, all shape3Score) string {
	t.Helper()
	var sb strings.Builder
	fmt.Fprintf(&sb, "%-14s %3s  %-10s  %-6s  %-6s  %-6s  %-9s  %-9s  %-9s  %-6s  missed\n",
		"class", "n", "nam pro no", "unit", "naming", "prose", "reuse", "consumers", "knowledge", "prose withheld")
	for _, cls := range claimedClasses {
		s := scores[cls]
		m := s.missed()
		fmt.Fprintf(&sb, "%s  %s\n", s.row(cls), strings.Join(m, ","))
		if len(m) > 0 {
			t.Errorf("AC-M1.5 shape 3 not met on %s (fresh set): gate missed on %s", cls, strings.Join(m, ", "))
		}
	}
	m := all.pooledMissed()
	fmt.Fprintf(&sb, "%s  %s\n", all.row("all"), strings.Join(m, ","))
	fmt.Fprintf(&sb, "stated unit lines right %s; candidate lines holding a truth %s; naming rows unit right %s; no-unit rows right %s\n",
		frac(all.StatedRight, all.Stated), frac(all.CandidateTruth, all.Candidate), frac(all.NamingRight, all.Naming), frac(all.NoUnitRight, all.NoUnit))
	if len(m) > 0 {
		t.Errorf("AC-M1.5 shape 3 not met over the fresh set: gate missed on %s", strings.Join(m, ", "))
	}
	return sb.String()
}
