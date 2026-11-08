package engine

import (
	"reflect"
	"testing"
)

// The AC-M1.5 judge's rules, fixed at pre-registration (classes.tsv header):
// a reuse truth is met when any kept reuse line's path is one of its
// alternatives, or by no reuse line when '-' is one of them; a consumer or knowledge truth is met when every kept line
// of the role is in the derived set and at least one is kept; '-' is met
// by no line of the role; '*' is not judged.
func TestACM15_JudgeAny(t *testing.T) {
	cases := []struct {
		got   []string
		truth string
		want  verdict
	}{
		{nil, "*", verdict{}},
		{[]string{"a.go"}, "*", verdict{}},
		{nil, "-", verdict{Judged: true, Right: true}},
		{[]string{"a.go"}, "-", verdict{Judged: true, Right: false}},
		{[]string{"c.go", "b.go"}, "a.go;b.go", verdict{Judged: true, Right: true}},
		{nil, "a.go", verdict{Judged: true, Right: false}},
		{[]string{"c.go"}, "a.go", verdict{Judged: true, Right: false}},
		{nil, "-;a.go", verdict{Judged: true, Right: true}},
		{[]string{"a.go"}, "-;a.go", verdict{Judged: true, Right: true}},
		{[]string{"b.go"}, "-;a.go", verdict{Judged: true, Right: false}},
	}
	for _, c := range cases {
		if got := judgeAny(c.got, c.truth); got != c.want {
			t.Errorf("judgeAny(%v, %q) = %+v, want %+v", c.got, c.truth, got, c.want)
		}
	}
}

func TestACM15_JudgeAll(t *testing.T) {
	cases := []struct {
		got   []string
		truth string
		want  verdict
	}{
		{nil, "*", verdict{}},
		{nil, "-", verdict{Judged: true, Right: true}},
		{[]string{"a.go"}, "-", verdict{Judged: true, Right: false}},
		{[]string{"a.go"}, "a.go;b.go", verdict{Judged: true, Right: true}},
		{[]string{"a.go", "c.go"}, "a.go;b.go", verdict{Judged: true, Right: false}},
		{nil, "a.go;b.go", verdict{Judged: true, Right: false}},
	}
	for _, c := range cases {
		if got := judgeAll(c.got, c.truth); got != c.want {
			t.Errorf("judgeAll(%v, %q) = %+v, want %+v", c.got, c.truth, got, c.want)
		}
	}
}

// rolePaths reads the kept lines only: a reuse line cut by the budget is
// not something the brief said.
func TestACM15_RolePaths(t *testing.T) {
	b := Brief{Lines: []Line{
		{Role: RoleUnit, Key: "unit:go:internal/x"},
		{Role: RoleReuse, Key: "file:internal/x/a.go"},
		{Role: RoleConsumer, Key: "file:internal/y/b.go"},
		{Role: RoleKnowledge, Key: "les:B1"},
		{Role: RoleReuse, Key: "file:internal/x/cut.go"},
	}, Kept: 4}
	if got := rolePaths(b, RoleReuse); !reflect.DeepEqual(got, []string{"internal/x/a.go"}) {
		t.Errorf("reuse = %v", got)
	}
	if got := rolePaths(b, RoleConsumer); !reflect.DeepEqual(got, []string{"internal/y/b.go"}) {
		t.Errorf("consumers = %v", got)
	}
	if got := rolePaths(b, RoleKnowledge); !reflect.DeepEqual(got, []string{"les:B1"}) {
		t.Errorf("knowledge = %v", got)
	}
	if got := rolePaths(b, RoleTarget); got != nil {
		t.Errorf("target = %v, want none", got)
	}
}

func TestACM15_CeilShare(t *testing.T) {
	cases := [][4]int{{10, 4, 5, 8}, {9, 4, 5, 8}, {7, 3, 5, 5}, {5, 3, 5, 3}, {0, 4, 5, 0}, {1, 4, 5, 1}}
	for _, c := range cases {
		if got := ceilShare(c[0], c[1], c[2]); got != c[3] {
			t.Errorf("ceilShare(%d,%d,%d) = %d, want %d", c[0], c[1], c[2], got, c[3])
		}
	}
}

// A class of ten with the unit right in eight, the reuse line right in
// three of five judged, consumers right in seven of ten and knowledge
// unjudged misses the consumer gate alone; the given-unit counts only
// grow on rows whose unit was right.
func TestACM15_ClassScore(t *testing.T) {
	var c classScore
	yes, no, un := verdict{true, true}, verdict{true, false}, verdict{}
	for i := 0; i < 10; i++ {
		unitOK := i < 8
		reuse := un
		if i < 5 {
			reuse = yes
			if i >= 3 {
				reuse = no
			}
		}
		cons := yes
		if i >= 7 {
			cons = no
		}
		c.add(unitOK, reuse, cons, un)
	}
	if c.N != 10 || c.UnitRight != 8 {
		t.Fatalf("n %d unit %d", c.N, c.UnitRight)
	}
	if c.Reuse != (roleCount{Judged: 5, Right: 3, JudgedUnit: 5, RightUnit: 3}) {
		t.Errorf("reuse %+v", c.Reuse)
	}
	if c.Consumers != (roleCount{Judged: 10, Right: 7, JudgedUnit: 8, RightUnit: 7}) {
		t.Errorf("consumers %+v", c.Consumers)
	}
	if c.Knowledge != (roleCount{}) {
		t.Errorf("knowledge %+v", c.Knowledge)
	}
	if got := c.missed(); !reflect.DeepEqual(got, []string{"consumers"}) {
		t.Errorf("missed = %v", got)
	}
	if row := c.row("refactor"); row == "" {
		t.Errorf("row is empty")
	}
}

// Rows alternate halves in file order within their class: the first,
// third, fifth, seventh and ninth are the development half, the rest the
// acceptance half the gates read (declared before any engine change on
// the set, 2026-11-07).
func TestACM15_HalfOf(t *testing.T) {
	for i, want := range []string{"development", "acceptance", "development", "acceptance"} {
		if got := halfOf(i); got != want {
			t.Errorf("halfOf(%d) = %q, want %q", i, got, want)
		}
	}
}

// TestACM15_HalfWanted: RASHOMON_ACM15_HALF unset or "development" builds
// the development rows only (engine work never reads the acceptance
// half); "acceptance" builds the acceptance rows only and reads the gates;
// "all" builds both for information.
func TestACM15_HalfWanted(t *testing.T) {
	cases := []struct {
		env, half string
		want      bool
	}{
		{"", "development", true},
		{"", "acceptance", false},
		{"development", "development", true},
		{"development", "acceptance", false},
		{"acceptance", "acceptance", true},
		{"acceptance", "development", false},
		{"all", "development", true},
		{"all", "acceptance", true},
		{"dev", "development", false},
		{"dev", "acceptance", false},
	}
	for _, c := range cases {
		if got := halfWanted(c.env, c.half); got != c.want {
			t.Errorf("halfWanted(%q, %q) = %v, want %v", c.env, c.half, got, c.want)
		}
	}
}
