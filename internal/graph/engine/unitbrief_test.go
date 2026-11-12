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

// edgeAt asserts one edge of the test graph at the unit revision.
func edgeAt(g *graph.Store, src graph.ID, pred graph.Pred, dst graph.ID) {
	g.AssertEdge(graph.Edge{Src: src, Pred: pred, Dst: dst, Tier: graph.Algorithmic, Conf: 1, Rev: unitRev, Prov: graph.Prov{Extractor: "synth", Version: "1"}})
}

// manyConsumers is a store unit with one function called from six other
// units, one file each, the callers' names ordering them c1 to c6.
func manyConsumers(t *testing.T) *graph.Store {
	t.Helper()
	files := []synFile{{"store", "store/loader.py", []synSym{fn("load_rows")}}}
	var calls [][2]string
	for _, n := range []string{"c1", "c2", "c3", "c4", "c5", "c6"} {
		files = append(files, synFile{n, n + "/" + n + ".py", []synSym{fn(n)}})
		calls = append(calls, [2]string{n + "/" + n + ".py#" + n, "store/loader.py#load_rows"})
	}
	return synth(t, files, calls, nil)
}

// TestUnitBrief_ManyConsumersLeaveThreeForKnowledge: consumers ahead of the
// rules must not starve them; with six consumer files and three records the
// brief is the top two consumers then the three records. Control: with no
// record the five lines are all consumers.
func TestUnitBrief_ManyConsumersLeaveThreeForKnowledge(t *testing.T) {
	g := manyConsumers(t)
	record(t, g, "dec:D1", "decision", "store/loader.py")
	record(t, g, "req:R1", "requirement", "store/loader.py")
	record(t, g, "les:L1", "lesson", "store/loader.py")
	b := UnitBrief(g, unitRev, storeUnit, Brief{}, Options{})
	want := []graph.ID{"file:c1/c1.py", "file:c2/c2.py", "dec:D1", "req:R1", "les:L1"}
	if got := keysOf(b.Lines); !slices.Equal(got, want) {
		t.Errorf("lines %v, want %v", got, want)
	}
	ctl := UnitBrief(manyConsumers(t), unitRev, storeUnit, Brief{}, Options{})
	wantCtl := []graph.ID{"file:c1/c1.py", "file:c2/c2.py", "file:c3/c3.py", "file:c4/c4.py", "file:c5/c5.py"}
	if got := keysOf(ctl.Lines); !slices.Equal(got, wantCtl) {
		t.Errorf("control lines %v, want %v", got, wantCtl)
	}
}

// consumerGraph is a store unit holding one symbol declared by the unit
// node itself (as gopg declares an interface's methods), called from the
// file of another unit; withDeclares says whether the unit declares it.
func consumerGraph(t *testing.T, withDeclares bool) *graph.Store {
	t.Helper()
	g := synth(t, []synFile{
		{"store", "store/loader.py", []synSym{fn("load_rows")}},
		{"brief", "brief/brief.py", []synSym{fn("brief")}},
	}, nil, nil)
	sym := graph.SymID(synLang, "store", "method", "Reader.Read")
	g.AssertNode(graph.Node{ID: sym, Kind: "sym", Tier: graph.Algorithmic, Conf: 1, Rev: unitRev,
		Attrs: map[string]string{"symkind": "method"}})
	if withDeclares {
		edgeAt(g, storeUnit, graph.Declares, sym)
	}
	edgeAt(g, graph.SymID(synLang, "brief", "func", "brief.brief"), graph.Calls, sym)
	return g
}

// TestUnitBrief_ConsumerThroughUnitDeclaredSymbol: a symbol the unit node
// itself declares is a target, so the caller's file is a consumer. Control:
// the same graph without the unit's Declares edge lists none.
func TestUnitBrief_ConsumerThroughUnitDeclaredSymbol(t *testing.T) {
	b := UnitBrief(consumerGraph(t, true), unitRev, storeUnit, Brief{}, Options{})
	if got, want := keysOf(b.Lines), []graph.ID{"file:brief/brief.py"}; !slices.Equal(got, want) {
		t.Errorf("lines %v, want %v", got, want)
	}
	ctl := UnitBrief(consumerGraph(t, false), unitRev, storeUnit, Brief{}, Options{})
	if len(ctl.Lines) != 0 {
		t.Errorf("control lines %v, want none", keysOf(ctl.Lines))
	}
}

// TestUnitBrief_ConsumerByReference: a References edge into the unit's
// symbol makes the referencing file a consumer, as M1's consumers count it.
// Control: the same graph without the edge lists none.
func TestUnitBrief_ConsumerByReference(t *testing.T) {
	build := func(ref bool) *graph.Store {
		g := synth(t, []synFile{
			{"store", "store/loader.py", []synSym{fn("load_rows")}},
			{"brief", "brief/brief.py", []synSym{fn("brief")}},
		}, nil, nil)
		if ref {
			edgeAt(g, graph.SymID(synLang, "brief", "func", "brief.brief"), graph.References, graph.SymID(synLang, "store", "func", "loader.load_rows"))
		}
		return g
	}
	b := UnitBrief(build(true), unitRev, storeUnit, Brief{}, Options{})
	if got, want := keysOf(b.Lines), []graph.ID{"file:brief/brief.py"}; !slices.Equal(got, want) {
		t.Errorf("lines %v, want %v", got, want)
	}
	if ctl := UnitBrief(build(false), unitRev, storeUnit, Brief{}, Options{}); len(ctl.Lines) != 0 {
		t.Errorf("control lines %v, want none", keysOf(ctl.Lines))
	}
}

// TestDeltas_KeylessLinesNeverDedup: a line with no key says nothing about
// another, so a keyless prev line seeds nothing and a keyless cur line is
// always kept; a keyed line still dedups.
func TestDeltas_KeylessLinesNeverDedup(t *testing.T) {
	prev := Brief{Lines: []Line{{Role: RoleConsumer, Basis: "p"}, {Key: "K"}}}
	cur := Brief{Lines: []Line{{Role: RoleConsumer, Basis: "a"}, {Key: "K"}, {Role: RoleConsumer, Basis: "b"}}}
	got := Deltas(prev, cur)
	if len(got) != 2 || got[0].Basis != "a" || got[1].Basis != "b" || got[0].Key != "" || got[1].Key != "" {
		t.Errorf("Deltas = %+v, want the two keyless lines a, b in order", got)
	}
}
