package engine

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

// AC-M1.5 (PLAN 7.7, W-M1.19): over an indexed tree, for every task class
// M1 claims, the brief's four lines are judged against pre-registered
// truths and the per-class table is the claim. RASHOMON_ACM11_DIR is the
// indexed root and RASHOMON_ACM15_CLASSES the truth set
// (test/live/autosteer/acm11/classes.tsv: id, class, unit, target, reuse,
// consumers, knowledge, allow, prompt); the test skips without them. The
// unit line is judged as AC-M1.1 does (a unit of 'a;b' is any of them, '-'
// is right when the brief holds no unit line). The reuse line is right
// when a kept reuse line's path is one of the truth's, and no reuse line
// is right when '-' is one of them; the consumer and
// knowledge lines are right when every kept line of the role is in the
// graph-derived set and at least one is kept, or when the set is '-' and
// none is kept; '*' leaves a cell unjudged. When the brief holds no unit
// line and '-' is an accepted unit, no consumer or knowledge line is
// expected (the role lines hang off the unit). Every row of a class
// counts, so a brief on the wrong unit fails its role lines, and the rate
// given the unit right is printed beside it. Gates per class, fixed at
// pre-registration: unit 4 of 5, reuse 3 of 5 of the rows judged,
// consumers 4 of 5, knowledge 4 of 5, each rounded up. The target column
// is informational (W-M1.20 reads it). The rows of a class alternate
// halves in file order: the first, third, fifth, seventh and ninth are
// the development half, read while the engine is worked on; the second,
// fourth, sixth, eighth and tenth are the acceptance half the gates read
// (declared 2026-11-07 after the set's first run and before any engine
// change on it; the full-ten table is printed beside it for information
// and claims nothing). Zero model cost; the prompts stay fixed;
// RASHOMON_ACM11_DUMP prints each brief. RASHOMON_ACM15_HALF picks the
// half built: unset or "development" builds and prints the development
// rows only, so engine work never reads the acceptance half;
// "acceptance" builds the acceptance rows and reads the gates; "all"
// builds both, for information.
func TestACM15_ClassesTable(t *testing.T) {
	root, path := os.Getenv("RASHOMON_ACM11_DIR"), os.Getenv("RASHOMON_ACM15_CLASSES")
	if root == "" || path == "" {
		t.Skip("RASHOMON_ACM11_DIR and RASHOMON_ACM15_CLASSES not set")
	}
	g, rev := loadACM11(t, root)
	rows := readTSV(t, path, 9)
	scores := map[string]*classScore{}
	halves := map[string]map[string]*classScore{"development": {}, "acceptance": {}}
	seen := map[string]int{}
	var order []string
	var all classScore
	wantHalf := os.Getenv("RASHOMON_ACM15_HALF")
	for _, r := range rows {
		id, cls, want, reuse, consumers, knowledge := r[0], r[1], strings.Split(r[2], ";"), r[4], r[5], r[6]
		half := halfOf(seen[cls])
		seen[cls]++
		if !halfWanted(wantHalf, half) {
			continue
		}
		prompt := strings.ReplaceAll(r[8], "\\n", "\n")
		b := Build(g, rev, prompt, Options{IDF: true})
		got := briefUnit(g, rev, b)
		basis, _, has := unitDisclosure(b)
		unitOK := slices.Contains(want, got) && got != "" || slices.Contains(want, "-") && !has
		if !has && slices.Contains(want, "-") {
			consumers, knowledge = "-", "-"
		}
		gr, gc, gk := rolePaths(b, RoleReuse), rolePaths(b, RoleConsumer), rolePaths(b, RoleKnowledge)
		vr, vc, vk := judgeAny(gr, reuse), judgeAll(gc, consumers), judgeAll(gk, knowledge)
		if scores[cls] == nil {
			scores[cls] = &classScore{}
			order = append(order, cls)
		}
		if halves[half][cls] == nil {
			halves[half][cls] = &classScore{}
		}
		scores[cls].add(unitOK, vr, vc, vk)
		halves[half][cls].add(unitOK, vr, vc, vk)
		all.add(unitOK, vr, vc, vk)
		t.Logf("%s %s (%s half): unit %q want %q %s (basis %q); reuse %v want %q %s; consumers %v (set of %d) %s; knowledge %v (set of %d) %s",
			id, cls, half, got, r[2], mark(verdict{true, unitOK}), basis, gr, reuse, mark(vr), gc, setSize(consumers), mark(vc), gk, setSize(knowledge), mark(vk))
		if os.Getenv("RASHOMON_ACM11_DUMP") != "" {
			t.Logf("%s\n%s", b.Text(), b.Explain())
		}
	}
	var sb strings.Builder
	for _, half := range []string{"acceptance", "development"} {
		if !halfWanted(wantHalf, half) {
			continue
		}
		fmt.Fprintf(&sb, "%s half:\n%-14s %3s  %-11s  %-19s  %-19s  %-19s  missed\n", half, "class", "n", "unit", "reuse (judged)", "consumers (judged)", "knowledge (judged)")
		var sum classScore
		for _, cls := range order {
			c := halves[half][cls]
			if c == nil {
				continue
			}
			m := c.missed()
			fmt.Fprintf(&sb, "%s  %s\n", c.row(cls), strings.Join(m, ","))
			sum.merge(*c)
			if half == "acceptance" && len(m) > 0 {
				t.Errorf("AC-M1.5 not met on %s (acceptance half): gate missed on %s", cls, strings.Join(m, ", "))
			}
		}
		fmt.Fprintf(&sb, "%s\n\n", sum.row("all"))
	}
	if halfWanted(wantHalf, "development") && halfWanted(wantHalf, "acceptance") {
		fmt.Fprintf(&sb, "all rows (information):\n")
		for _, cls := range order {
			fmt.Fprintf(&sb, "%s  %s\n", scores[cls].row(cls), strings.Join(scores[cls].missed(), ","))
		}
		fmt.Fprintf(&sb, "%s\n", all.row("all"))
	}
	t.Logf("AC-M1.5 per-class table (right/n, then right/judged with the rate given the unit right in brackets; gates unit 4/5, reuse 3/5, consumers 4/5, knowledge 4/5 rounded up, read on the acceptance half):\n%s", sb.String())
}

// halfWanted is whether RASHOMON_ACM15_HALF (env) builds the rows of half:
// unset or "development" the development rows only, "acceptance" the
// acceptance rows only, "all" both.
func halfWanted(env, half string) bool {
	switch env {
	case "", "development":
		return half == "development"
	case "acceptance":
		return half == "acceptance"
	}
	return true
}

// halfOf is the half of the i-th row of its class in file order:
// development for the first, third, fifth, seventh and ninth, acceptance
// for the rest.
func halfOf(i int) string {
	if i%2 == 0 {
		return "development"
	}
	return "acceptance"
}

// verdict is one role's judgement on one row: whether the truth was
// judged (not '*') and whether the brief met it.
type verdict struct {
	Judged, Right bool
}

// judgeAny is the reuse rule: '*' unjudged; else met when any of got is
// one of the truth's alternatives, a '-' among them met by no line ('-'
// alone only by no line, '-;a.go' by no line or by a.go: the reuse
// re-read's R2 gains keep '-' beside the readers' file).
func judgeAny(got []string, truth string) verdict {
	if truth == "*" {
		return verdict{}
	}
	set := strings.Split(truth, ";")
	if len(got) == 0 {
		return verdict{Judged: true, Right: slices.Contains(set, "-")}
	}
	for _, p := range got {
		if slices.Contains(set, p) {
			return verdict{Judged: true, Right: true}
		}
	}
	return verdict{Judged: true}
}

// judgeAll is the consumer and knowledge rule: '*' unjudged; '-' met by
// no line; else met when at least one line is kept and every kept line is
// in the set.
func judgeAll(got []string, truth string) verdict {
	switch truth {
	case "*":
		return verdict{}
	case "-":
		return verdict{Judged: true, Right: len(got) == 0}
	}
	set := strings.Split(truth, ";")
	for _, p := range got {
		if !slices.Contains(set, p) {
			return verdict{Judged: true}
		}
	}
	return verdict{Judged: true, Right: len(got) > 0}
}

// rolePaths is what the kept lines of a role name: the knowledge id for a
// knowledge line, the file path for a reuse or consumer line.
func rolePaths(b Brief, role string) []string {
	var out []string
	for _, l := range b.Lines[:b.Kept] {
		if l.Role != role {
			continue
		}
		if role == RoleKnowledge {
			out = append(out, string(l.Key))
			continue
		}
		out = append(out, strings.TrimPrefix(string(l.Key), "file:"))
	}
	return out
}

// ceilShare is num/den of n, rounded up: the gate for a class of n rows.
func ceilShare(n, num, den int) int {
	return (n*num + den - 1) / den
}

// roleCount tallies one role over a class: rows judged and right, and the
// same over the rows whose unit line was right.
type roleCount struct {
	Judged, Right, JudgedUnit, RightUnit int
}

func (c *roleCount) add(unitOK bool, v verdict) {
	if !v.Judged {
		return
	}
	c.Judged++
	if v.Right {
		c.Right++
	}
	if unitOK {
		c.JudgedUnit++
		if v.Right {
			c.RightUnit++
		}
	}
}

func (c roleCount) String() string {
	if c.Judged == 0 {
		return "unjudged"
	}
	return fmt.Sprintf("%d/%d [%d/%d]", c.Right, c.Judged, c.RightUnit, c.JudgedUnit)
}

// classScore is one class's tally over its rows.
type classScore struct {
	N, UnitRight                int
	Reuse, Consumers, Knowledge roleCount
}

// merge adds another tally into this one.
func (c *classScore) merge(o classScore) {
	c.N += o.N
	c.UnitRight += o.UnitRight
	for _, p := range [][2]*roleCount{{&c.Reuse, &o.Reuse}, {&c.Consumers, &o.Consumers}, {&c.Knowledge, &o.Knowledge}} {
		p[0].Judged += p[1].Judged
		p[0].Right += p[1].Right
		p[0].JudgedUnit += p[1].JudgedUnit
		p[0].RightUnit += p[1].RightUnit
	}
}

func (c *classScore) add(unitOK bool, reuse, consumers, knowledge verdict) {
	c.N++
	if unitOK {
		c.UnitRight++
	}
	c.Reuse.add(unitOK, reuse)
	c.Consumers.add(unitOK, consumers)
	c.Knowledge.add(unitOK, knowledge)
}

// missed names the gates the class fails, in table order.
func (c classScore) missed() []string {
	var out []string
	if c.UnitRight < ceilShare(c.N, 4, 5) {
		out = append(out, "unit")
	}
	if c.Reuse.Right < ceilShare(c.Reuse.Judged, 3, 5) {
		out = append(out, "reuse")
	}
	if c.Consumers.Right < ceilShare(c.Consumers.Judged, 4, 5) {
		out = append(out, "consumers")
	}
	if c.Knowledge.Right < ceilShare(c.Knowledge.Judged, 4, 5) {
		out = append(out, "knowledge")
	}
	return out
}

func (c classScore) row(name string) string {
	return fmt.Sprintf("%-14s %3d  %-11s  %-19s  %-19s  %-19s", name, c.N, fmt.Sprintf("%d/%d", c.UnitRight, c.N), c.Reuse, c.Consumers, c.Knowledge)
}

func mark(v verdict) string {
	switch {
	case !v.Judged:
		return "unjudged"
	case v.Right:
		return "right"
	}
	return "WRONG"
}

func setSize(truth string) int {
	if truth == "-" || truth == "*" {
		return 0
	}
	return len(strings.Split(truth, ";"))
}
