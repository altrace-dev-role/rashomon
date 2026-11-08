package engine

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// Gate shape 3's naming rule splits the development files as it was
// registered: by the prompt alone, classes.tsv 81 naming, 62 prose and 17
// no-unit rows and classes2.tsv 58, 84 and 18 (W-M1.37's split.py, alnum
// mode); with the four naming classes, 103, 40, 17 and 80, 62, 18.
func TestACM15_NamingSplit(t *testing.T) {
	cases := []struct {
		file          string
		prompt, kinds [3]int
	}{
		{"classes.tsv", [3]int{81, 62, 17}, [3]int{103, 40, 17}},
		{"classes2.tsv", [3]int{58, 84, 18}, [3]int{80, 62, 18}},
	}
	index := map[string]int{kindNaming: 0, kindProse: 1, kindNoUnit: 2}
	for _, c := range cases {
		var prompt, kinds [3]int
		for _, r := range readTSV(t, filepath.Join("..", "..", "..", "test", "live", "autosteer", "acm11", c.file), 9) {
			kinds[index[rowKind(r)]]++
			switch {
			case r[2] == "-":
				prompt[2]++
			case namesTruth(r):
				prompt[0]++
			default:
				prompt[1]++
			}
		}
		if prompt != c.prompt || kinds != c.kinds {
			t.Errorf("%s: prompt rule %v, want %v; kinds %v, want %v", c.file, prompt, c.prompt, kinds, c.kinds)
		}
	}
}

// A truth unit's last segment or a target file's name names the unit only
// with no letter or digit beside it; a unit's path names it as a
// substring; the prompt is case-folded.
func TestACM15_NamesTruth(t *testing.T) {
	row := func(unit, target, prompt string) []string {
		return []string{"X1", "refactor", unit, target, "*", "*", "*", "*", prompt}
	}
	cases := []struct {
		r    []string
		want bool
	}{
		{row("internal/learn", "-", "fix the flake in learn_test.go"), true},
		{row("internal/learn", "-", "relearn the weights"), false},
		{row("internal/learn", "-", "Learn: drop the cap"), true},
		{row("internal/learn", "-", "learn2 drifts"), false},
		{row("internal/graph/engine", "-", "see internal/graph/engines/x.go"), true},
		{row("internal/spend", "cmd/rashomon/budget.go", "the cost verb"), false},
		{row("internal/spend", "cmd/rashomon/budget.go", "budget.go panics"), true},
		{row("-", "-", "anything"), false},
		{row("-;internal/x", "-", "add x\\nand test it"), true},
	}
	for _, c := range cases {
		if got := namesTruth(c.r); got != c.want {
			t.Errorf("namesTruth(unit %q, target %q, %q) = %v, want %v", c.r[2], c.r[3], c.r[8], got, c.want)
		}
	}
	if bounded("anything", "") {
		t.Errorf("an empty name names something")
	}
}

// A unit line that renders its runner-up is a candidate; a runner-up the
// line does not render leaves it stated; a brief without a unit line
// claims the unit briefUnit gave it; a line past Kept was not said.
func TestACM15_ClaimOf(t *testing.T) {
	unitLine := func(f slot.UnitFact) Line {
		v, err := slot.Bind(slot.NewUnitLine(f))
		if err != nil {
			t.Fatal(err)
		}
		return Line{Role: RoleUnit, Key: "unit:go:x", Slot: v.(slot.GraphLine), Text: v.Text()}
	}
	cover := unitLine(slot.UnitFact{Path: "internal/store", Basis: slot.BasisCoverage, RunnerUp: "internal/safe"})
	byName := unitLine(slot.UnitFact{Path: "internal/hook", Basis: slot.BasisUnitName, RunnerUp: "internal/learn"})
	reuse := Line{Role: RoleReuse, Key: "file:internal/x/a.go"}
	cases := []struct {
		b    Brief
		unit string
		want briefClaim
	}{
		{Brief{Lines: []Line{cover, reuse}, Kept: 2}, "internal/store", briefClaim{Unit: "internal/store", Runner: "internal/safe", Line: true, Candidate: true}},
		{Brief{Lines: []Line{byName}, Kept: 1}, "internal/hook", briefClaim{Unit: "internal/hook", Runner: "internal/learn", Line: true}},
		{Brief{Lines: []Line{reuse}, Kept: 1}, "internal/x", briefClaim{Unit: "internal/x"}},
		{Brief{Lines: []Line{reuse, cover}, Kept: 1}, "internal/x", briefClaim{Unit: "internal/x"}},
		{Brief{}, "", briefClaim{}},
	}
	for i, c := range cases {
		if got := claimOf(c.b, c.unit); got != c.want {
			t.Errorf("case %d: claimOf = %+v, want %+v", i, got, c.want)
		}
	}
}

// A prose row is right when the brief claims no unit, states a truth or
// offers a candidate pair holding one; a naming or no-unit row keeps
// shape 2's verdict, a pair included.
func TestACM15_UnitRight3(t *testing.T) {
	want := []string{"internal/a", "internal/b"}
	pair := briefClaim{Unit: "internal/c", Runner: "internal/b", Line: true, Candidate: true}
	cases := []struct {
		kind   string
		c      briefClaim
		unitOK bool
		right  bool
	}{
		{kindProse, briefClaim{}, false, true},
		{kindProse, briefClaim{Unit: "internal/a", Line: true}, true, true},
		{kindProse, briefClaim{Unit: "internal/c", Line: true}, false, false},
		{kindProse, pair, false, true},
		{kindProse, briefClaim{Unit: "internal/c", Runner: "internal/b", Line: true}, false, false},
		{kindProse, briefClaim{Unit: "internal/c"}, false, false},
		{kindNaming, briefClaim{}, false, false},
		{kindNaming, pair, false, false},
		{kindNoUnit, briefClaim{}, true, true},
	}
	for _, c := range cases {
		if got := unitRight3(c.kind, want, c.c, c.unitOK); got != c.right {
			t.Errorf("unitRight3(%s, %v, unitOK %v) = %v, want %v", c.kind, c.c, c.unitOK, got, c.right)
		}
	}
}

// Ten rows: six naming rows stating the truth with the knowledge line
// right on four, two prose rows claiming no unit, a prose pair holding the
// truth, a prose row stating a wrong unit. The unit verdict is 9 of 10;
// knowledge 4 of 6 judged misses 7/10 in the class and over the set; the
// wrong unit's consumer line is not judged. Two rows with a role judged
// pass the class (under three judged) and miss the set.
func TestACM15_Shape3Score(t *testing.T) {
	want := []string{"internal/a"}
	yes, no := verdict{true, true}, verdict{true, false}
	stated := func(u string) briefClaim { return briefClaim{Unit: u, Line: true} }
	var s shape3Score
	for i := 0; i < 6; i++ {
		k := yes
		if i >= 4 {
			k = no
		}
		s.add(kindNaming, want, rowVerdicts{UnitOK: true, Knowledge: k, Claim: stated("internal/a")})
	}
	s.add(kindProse, want, rowVerdicts{})
	s.add(kindProse, want, rowVerdicts{})
	s.add(kindProse, want, rowVerdicts{Claim: briefClaim{Unit: "internal/b", Runner: "internal/a", Line: true, Candidate: true}})
	if right := s.add(kindProse, want, rowVerdicts{Consumers: no, Claim: stated("internal/b")}); right {
		t.Errorf("a prose row stating a wrong unit is right")
	}
	got := [...]int{s.N, s.UnitRight, s.Naming, s.NamingRight, s.Prose, s.ProseRight, s.Withheld, s.Stated, s.StatedRight, s.Candidate, s.CandidateTruth}
	if got != [...]int{10, 9, 6, 6, 4, 3, 2, 7, 6, 1, 1} {
		t.Errorf("tally %v", got)
	}
	if s.Consumers.JudgedUnit != 0 || s.Knowledge.JudgedUnit != 6 || s.Knowledge.RightUnit != 4 {
		t.Errorf("roles: consumers %+v knowledge %+v", s.Consumers, s.Knowledge)
	}
	if m := s.missed(); !reflect.DeepEqual(m, []string{"knowledge"}) {
		t.Errorf("missed = %v", m)
	}
	if m := s.pooledMissed(); !reflect.DeepEqual(m, []string{"knowledge"}) {
		t.Errorf("pooledMissed = %v", m)
	}
	var few shape3Score
	few.add(kindNaming, want, rowVerdicts{UnitOK: true, Reuse: no, Claim: stated("internal/a")})
	few.add(kindNaming, want, rowVerdicts{UnitOK: true, Reuse: no, Claim: stated("internal/a")})
	if m := few.missed(); m != nil {
		t.Errorf("two judged: missed = %v, want none", m)
	}
	if m := few.pooledMissed(); !reflect.DeepEqual(m, []string{"reuse"}) {
		t.Errorf("two judged: pooledMissed = %v", m)
	}
	var wrong shape3Score
	wrong.add(kindNaming, want, rowVerdicts{Claim: stated("internal/b")})
	wrong.add(kindNaming, want, rowVerdicts{UnitOK: true, Claim: stated("internal/a")})
	wrong.add(kindProse, want, rowVerdicts{Claim: briefClaim{Unit: "internal/b", Runner: "internal/c", Line: true, Candidate: true}})
	if m := wrong.pooledMissed(); !reflect.DeepEqual(m, []string{"stated", "candidate"}) {
		t.Errorf("stated 1 of 2, candidate 0 of 1: pooledMissed = %v", m)
	}
}
