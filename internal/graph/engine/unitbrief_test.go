package engine

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// unitRepo is a store unit with one function, two other units that call it
// (brief twice over, audit once) and a third that only imports its file,
// a decision, a requirement and a lesson on the store file.
func unitRepo(t *testing.T) *graph.Store {
	t.Helper()
	g := synth(t, []synFile{
		{"store", "store/loader.py", []synSym{fn("load_rows"), fn("close_rows")}},
		{"brief", "brief/brief.py", []synSym{fn("brief"), fn("brief_all")}},
		{"audit", "audit/audit.py", []synSym{fn("audit")}},
	}, [][2]string{
		{"brief/brief.py#brief", "store/loader.py#load_rows"},
		{"brief/brief.py#brief_all", "store/loader.py#close_rows"},
		{"audit/audit.py#audit", "store/loader.py#load_rows"},
	}, nil)
	record(t, g, "dec:D1", "decision", "store/loader.py")
	record(t, g, "req:R1", "requirement", "store/loader.py")
	record(t, g, "les:L1", "lesson", "store/loader.py")
	return g
}

var (
	unitRev   = graph.RevCommit(graphtest.Commit)
	storeUnit = graph.UnitID(synLang, "store")
)

func keysOf(ls []Line) []graph.ID {
	var out []graph.ID
	for _, l := range ls {
		out = append(out, l.Key)
	}
	return out
}

// TestUnitBrief_ConsumersLeadThenRulesThenLessons: the unit's consumers
// come first, the file with more calling edges before the one with fewer,
// then the rules and decisions that apply to the unit, then its lessons;
// one finding per line, the kinds the steer pattern kinds name.
func TestUnitBrief_ConsumersLeadThenRulesThenLessons(t *testing.T) {
	g := unitRepo(t)
	b := UnitBrief(g, unitRev, storeUnit, Brief{}, Options{})
	t.Logf("\n%s", b.Text())
	want := []graph.ID{"file:brief/brief.py", "file:audit/audit.py", "dec:D1", "req:R1", "les:L1"}
	if got := keysOf(b.Lines[:b.Kept]); !slices.Equal(got, want) {
		t.Fatalf("lines %v, want %v", got, want)
	}
	for i, role := range []string{RoleConsumer, RoleConsumer} {
		if b.Lines[i].Role != role {
			t.Errorf("line %d role %q, want %q", i, b.Lines[i].Role, role)
		}
	}
	text := b.Text()
	for _, w := range []string{"consumer 'brief/brief.py'", "consumer 'audit/audit.py'", "D1", "R1", "L1"} {
		if !strings.Contains(text, w) {
			t.Errorf("message lacks %q:\n%s", w, text)
		}
	}
	if b.Prompt != "" || len(b.Tokens) != 0 {
		t.Errorf("the unit brief holds prompt words: %q %v", b.Prompt, b.Tokens)
	}
}

// TestUnitBrief_DeltasAgainstThePromptBrief: a line the prompt brief
// already delivered is not in the unit brief's lines, and Deltas is the
// set difference on line keys.
func TestUnitBrief_DeltasAgainstThePromptBrief(t *testing.T) {
	g := unitRepo(t)
	prev := Brief{Lines: []Line{{Key: "file:brief/brief.py"}, {Key: "dec:D1"}}}
	b := UnitBrief(g, unitRev, storeUnit, prev, Options{})
	want := []graph.ID{"file:audit/audit.py", "req:R1", "les:L1"}
	if got := keysOf(b.Lines[:b.Kept]); !slices.Equal(got, want) {
		t.Fatalf("lines %v, want %v", got, want)
	}
	cur := Brief{Lines: []Line{{Key: "a"}, {Key: "b"}, {Key: "c"}}}
	if got := keysOf(Deltas(Brief{Lines: []Line{{Key: "b"}, {Key: "z"}}}, cur)); !slices.Equal(got, []graph.ID{"a", "c"}) {
		t.Errorf("Deltas = %v, want [a c]", got)
	}
	if got := Deltas(cur, cur); len(got) != 0 {
		t.Errorf("Deltas(x, x) = %v, want none", got)
	}
}

// TestUnitBrief_ReuseLineOfThePromptBriefClosesTheBrief: the prompt
// brief's reuse line for this unit rides last; one for another unit does not.
func TestUnitBrief_ReuseLineOfThePromptBriefClosesTheBrief(t *testing.T) {
	g := unitRepo(t)
	prompt := Brief{Lines: []Line{{Key: "file:brief/brief.py", Role: RoleReuse}, {Key: "file:store/loader.py", Role: RoleReuse, Members: []graph.ID{"file:store/loader.py"}}}}
	b := UnitBrief(g, unitRev, storeUnit, prompt, Options{})
	keys := keysOf(b.Lines)
	if n := len(keys); n == 0 || keys[n-1] != "file:store/loader.py" || b.Lines[n-1].Role != RoleReuse {
		t.Fatalf("lines %v, want the store reuse line last", keys)
	}
	if len(b.Lines) > MaxLines {
		t.Errorf("%d lines, over %d", len(b.Lines), MaxLines)
	}
	if slices.Contains(keys[:len(keys)-1], "file:store/loader.py") {
		t.Errorf("reuse line repeated: %v", keys)
	}
}

// TestUnitBrief_NothingToSayIsEmpty: a unit nobody consumes and no record
// reaches gets an empty brief.
func TestUnitBrief_NothingToSayIsEmpty(t *testing.T) {
	g := unitRepo(t)
	b := UnitBrief(g, unitRev, graph.UnitID(synLang, "audit"), Brief{}, Options{})
	if !b.Empty() || len(b.Lines) != 0 {
		t.Errorf("brief %v, want empty", keysOf(b.Lines))
	}
	if b := UnitBrief(g, unitRev, graph.UnitID(synLang, "nope"), Brief{}, Options{}); !b.Empty() {
		t.Errorf("unknown unit gave a brief")
	}
}

// TestUnitBrief_CapAtMaxLines: more findings than MaxLines are cut in order.
func TestUnitBrief_CapAtMaxLines(t *testing.T) {
	g := unitRepo(t)
	for _, id := range []string{"les:L2", "les:L3", "les:L4"} {
		record(t, g, id, "lesson", "store/loader.py")
	}
	b := UnitBrief(g, unitRev, storeUnit, Brief{}, Options{})
	want := []graph.ID{"file:brief/brief.py", "file:audit/audit.py", "dec:D1", "req:R1", "les:L1"}
	if got := keysOf(b.Lines); !slices.Equal(got, want) {
		t.Errorf("lines %v, want %v", got, want)
	}
}

// TestUnitBrief_Deterministic: two calls give the same message.
func TestUnitBrief_Deterministic(t *testing.T) {
	g := unitRepo(t)
	a, b := UnitBrief(g, unitRev, storeUnit, Brief{}, Options{}), UnitBrief(g, unitRev, storeUnit, Brief{}, Options{})
	if a.Text() != b.Text() || a.Text() == "" {
		t.Errorf("not deterministic or empty:\n%s\n%s", a.Text(), b.Text())
	}
}
