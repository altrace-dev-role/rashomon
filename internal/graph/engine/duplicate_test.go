package engine

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

var dupRev = graph.RevCommit("c1")

func dupSym(unit, kind, qual string) graph.ID { return graph.SymID("go", unit, kind, qual) }

func dupNode(id graph.ID, symkind, doc string) graph.Node {
	return graph.Node{ID: id, Kind: "sym", Attrs: map[string]string{"symkind": symkind, "doc": doc},
		Tier: graph.Proven, Conf: 1, Rev: dupRev}
}

type dupCase struct {
	name    string
	nodes   []graph.Node
	called  []graph.ID // nodes with one incoming Calls edge
	decls   []graph.Node
	removed []graph.Node
	want    []Conflict
}

func dupRecords(ns []graph.Node) []graph.Record {
	var out []graph.Record
	for i := range ns {
		out = append(out, graph.Record{Node: &ns[i]})
	}
	return out
}

func dupStore(c dupCase) *graph.Store {
	s := graph.NewStore()
	for _, n := range slices.Concat(c.nodes, c.decls) {
		s.AssertNode(n)
	}
	caller := dupSym("x/callers", "func", "Main")
	s.AssertNode(dupNode(caller, "func", ""))
	for _, id := range c.called {
		s.AssertEdge(graph.Edge{Src: caller, Pred: graph.Calls, Dst: id, Tier: graph.Proven, Conf: 1, Rev: dupRev})
	}
	return s
}

// dupOnlyReader hides the store's Nodes.
type dupOnlyReader struct{ r Reader }

func (o dupOnlyReader) Node(id graph.ID, at graph.Rev) (graph.Node, bool) { return o.r.Node(id, at) }
func (o dupOnlyReader) Out(id graph.ID, at graph.Rev, p ...graph.Pred) []graph.Edge {
	return o.r.Out(id, at, p...)
}
func (o dupOnlyReader) In(id graph.ID, at graph.Rev, p ...graph.Pred) []graph.Edge {
	return o.r.In(id, at, p...)
}

var (
	dupAdded = dupSym("a/new", "func", "pkg.ParseUserConfigFile")
	dupOld   = dupSym("b/old", "func", "pkg.ReadConfigFileUser")
	dupOld2  = dupSym("c/old", "func", "pkg.LoadFileUserConfig")
)

const dupDoc = "parse user config file"

func dupWant(added, existing graph.ID, unit graph.ID) Conflict {
	return Conflict{Kind: ConflictDuplicateAbstraction, Unit: unit, Subject: added, Evidence: []graph.ID{added, existing}, Rev: dupRev}
}

func TestConflicts_DuplicateAbstraction(t *testing.T) {
	addedN := dupNode(dupAdded, "func", dupDoc)
	oldN := dupNode(dupOld, "func", "")
	unitA := graph.UnitID("go", "a/new")
	cases := []dupCase{
		{name: "a fires with every field", nodes: []graph.Node{oldN}, called: []graph.ID{dupOld}, decls: []graph.Node{addedN},
			want: []Conflict{dupWant(dupAdded, dupOld, unitA)}},
		{name: "b existing has no callers", nodes: []graph.Node{oldN}, decls: []graph.Node{addedN}},
		{name: "c same unit", nodes: []graph.Node{dupNode(dupSym("a/new", "func", "pkg.ReadConfigFileUser"), "func", "")},
			called: []graph.ID{dupSym("a/new", "func", "pkg.ReadConfigFileUser")}, decls: []graph.Node{addedN}},
		{name: "d k stems fire", nodes: []graph.Node{dupNode(dupSym("b/old", "func", "pkg.ReadUserConfigFile"), "func", "")},
			called: []graph.ID{dupSym("b/old", "func", "pkg.ReadUserConfigFile")},
			decls:  []graph.Node{dupNode(dupSym("a/new", "func", "pkg.ParseUserConfigFile"), "func", "")},
			want:   []Conflict{dupWant(dupSym("a/new", "func", "pkg.ParseUserConfigFile"), dupSym("b/old", "func", "pkg.ReadUserConfigFile"), unitA)}},
		{name: "d k-1 stems do not", nodes: []graph.Node{dupNode(dupSym("b/old", "func", "pkg.ReadUserConfig"), "func", "")},
			called: []graph.ID{dupSym("b/old", "func", "pkg.ReadUserConfig")},
			decls:  []graph.Node{dupNode(dupSym("a/new", "func", "pkg.ParseUserConfig"), "func", "")}},
		{name: "e changed in place", nodes: []graph.Node{oldN}, called: []graph.ID{dupOld}, decls: []graph.Node{addedN}, removed: []graph.Node{addedN}},
		{name: "f added type", nodes: []graph.Node{oldN}, called: []graph.ID{dupOld},
			decls: []graph.Node{dupNode(dupSym("a/new", "type", "pkg.ParseUserConfigFile"), "type", dupDoc)}},
		{name: "f added const", nodes: []graph.Node{oldN}, called: []graph.ID{dupOld},
			decls: []graph.Node{dupNode(dupSym("a/new", "const", "pkg.ParseUserConfigFile"), "const", dupDoc)}},
		{name: "h best match wins", nodes: []graph.Node{oldN, dupNode(dupOld2, "func", "parse")}, called: []graph.ID{dupOld, dupOld2},
			decls: []graph.Node{addedN}, want: []Conflict{dupWant(dupAdded, dupOld2, unitA)}},
		{name: "h tie goes to the smaller id", nodes: []graph.Node{oldN, dupNode(dupOld2, "func", "")}, called: []graph.ID{dupOld, dupOld2},
			decls: []graph.Node{addedN}, want: []Conflict{dupWant(dupAdded, dupOld, unitA)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := dupStore(c)
			d := Diff{Rev: dupRev, Decls: dupRecords(c.decls), Removed: dupRecords(c.removed)}
			if got := duplicateConflicts(s, d); !dupEqual(got, c.want) {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
		})
	}
	t.Run("g a Reader without Nodes yields nil", func(t *testing.T) {
		c := cases[0]
		d := Diff{Rev: dupRev, Decls: dupRecords(c.decls)}
		if got := duplicateConflicts(dupOnlyReader{dupStore(c)}, d); got != nil {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("i order of Decls does not matter", func(t *testing.T) {
		a2 := dupNode(dupSym("d/new", "func", "pkg.OpenUserConfigFile"), "func", dupDoc)
		c := dupCase{nodes: []graph.Node{oldN}, called: []graph.ID{dupOld}, decls: []graph.Node{addedN, a2}}
		s := dupStore(c)
		fwd := duplicateConflicts(s, Diff{Rev: dupRev, Decls: dupRecords(c.decls)})
		rev := duplicateConflicts(s, Diff{Rev: dupRev, Decls: dupRecords([]graph.Node{a2, addedN})})
		if len(fwd) != 2 || !dupEqual(fwd, rev) || fwd[0].Subject > fwd[1].Subject {
			t.Errorf("fwd %+v rev %+v", fwd, rev)
		}
	})
}

// dupAt is n at rev with the attrs given as key, value pairs added.
func dupAt(n graph.Node, rev graph.Rev, kv ...string) graph.Node {
	a := map[string]string{}
	for k, v := range n.Attrs {
		a[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		a[kv[i]] = kv[i+1]
	}
	n.Attrs, n.Rev = a, rev
	return n
}

// TestConflicts_DuplicateSkipsCalledCandidate_CommitRev: P2 ruling 7(b).
// An added callable is not compared against a candidate it calls: a Calls
// or MayCall edge to it at d.Rev, or its calls attr naming the candidate's
// short name. The exclusion is per candidate: the added callable still
// fires against another candidate it does not call.
func TestConflicts_DuplicateSkipsCalledCandidate_CommitRev(t *testing.T) {
	addedN, oldN := dupNode(dupAdded, "func", dupDoc), dupNode(dupOld, "func", "")
	unitA := graph.UnitID("go", "a/new")
	run := func(name string, added graph.Node, edge graph.Pred, want []Conflict) {
		t.Helper()
		c := dupCase{nodes: []graph.Node{oldN, dupNode(dupOld2, "func", "")}, called: []graph.ID{dupOld, dupOld2}, decls: []graph.Node{added}}
		s := dupStore(c)
		if edge != "" {
			s.AssertEdge(graph.Edge{Src: dupAdded, Pred: edge, Dst: dupOld, Tier: graph.Proven, Conf: 1, Rev: dupRev})
		}
		if got := duplicateConflicts(s, Diff{Rev: dupRev, Decls: dupRecords(c.decls)}); !dupEqual(got, want) {
			t.Errorf("%s: got %+v, want %+v", name, got, want)
		}
	}
	run("control, calls nothing: the smaller id of the tie", addedN, "", []Conflict{dupWant(dupAdded, dupOld, unitA)})
	run("a Calls edge to the old one", addedN, graph.Calls, []Conflict{dupWant(dupAdded, dupOld2, unitA)})
	run("a MayCall edge to the old one", addedN, graph.MayCall, []Conflict{dupWant(dupAdded, dupOld2, unitA)})
	run("its calls attr names the old one", dupAt(addedN, dupRev, "calls", "Close,ReadConfigFileUser"), "",
		[]Conflict{dupWant(dupAdded, dupOld2, unitA)})
	run("it calls both", dupAt(addedN, dupRev, "calls", "LoadFileUserConfig"), graph.Calls, nil)
}

// TestConflicts_DuplicateSkipsCalledCandidate_OverlayRev: P2 ruling 7(b)
// at an overlay revision, where an added callable has no out-edges (the
// Go overlay carries Declares alone) and its calls attr is the only
// witness. A same-named function elsewhere is excluded too: a miss, the
// safe side.
func TestConflicts_DuplicateSkipsCalledCandidate_OverlayRev(t *testing.T) {
	ov := graph.RevOverlay("c1", 1)
	unitA := graph.UnitID("go", "a/new")
	oldN := dupNode(dupOld, "func", "")
	for _, tc := range []struct {
		name, calls string
		want        []Conflict
	}{
		{"control, calls another name", "Close", []Conflict{{Kind: ConflictDuplicateAbstraction, Unit: unitA, Subject: dupAdded,
			Evidence: []graph.ID{dupAdded, dupOld}, Rev: ov}}},
		{"calls names the old one", "Close,ReadConfigFileUser", nil},
		{"calls names it, meaning a function elsewhere", "ReadConfigFileUser", nil},
	} {
		added := dupAt(dupNode(dupAdded, "func", dupDoc), ov, "calls", tc.calls)
		c := dupCase{nodes: []graph.Node{oldN}, called: []graph.ID{dupOld}, decls: []graph.Node{added}}
		if tc.name == "calls names it, meaning a function elsewhere" {
			other := dupSym("z/other", "func", "pkg.ReadConfigFileUser")
			c.nodes, c.called = append(c.nodes, dupNode(other, "func", "")), append(c.called, other)
		}
		s := dupStore(c)
		if got := duplicateConflicts(s, Diff{Rev: ov, Decls: dupRecords(c.decls)}); !dupEqual(got, tc.want) {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

// TestConflicts_DuplicateFiresOnNameAndDocAtOverlayRev: P2 ruling 7. At an
// overlay revision the added callable's stems are its overlay record's name
// and doc: a two-word name reaches k shared stems only with its doc. The
// control is the same record with no doc.
func TestConflicts_DuplicateFiresOnNameAndDocAtOverlayRev(t *testing.T) {
	ov := graph.RevOverlay("c1", 1)
	walk := dupSym("a/new", "func", "pkg.WalkConfig")
	for _, tc := range []struct {
		doc  string
		want []Conflict
	}{
		{"walk the user config file", []Conflict{{Kind: ConflictDuplicateAbstraction, Unit: graph.UnitID("go", "a/new"), Subject: walk,
			Evidence: []graph.ID{walk, dupOld}, Rev: ov}}},
		{"", nil},
	} {
		added := dupAt(dupNode(walk, "func", tc.doc), ov, "name", "WalkConfig")
		c := dupCase{nodes: []graph.Node{dupNode(dupOld, "func", "")}, called: []graph.ID{dupOld}, decls: []graph.Node{added}}
		if got := duplicateConflicts(dupStore(c), Diff{Rev: ov, Decls: dupRecords(c.decls)}); !dupEqual(got, tc.want) {
			t.Errorf("doc %q: got %+v, want %+v", tc.doc, got, tc.want)
		}
	}
}

func dupEqual(a, b []Conflict) bool {
	return slices.EqualFunc(a, b, func(x, y Conflict) bool {
		return x.Kind == y.Kind && x.Unit == y.Unit && x.RuleID == y.RuleID && x.Subject == y.Subject &&
			slices.Equal(x.Evidence, y.Evidence) && x.Rev == y.Rev && x.Got == y.Got && x.Want == y.Want &&
			x.Edge.Key() == y.Edge.Key() && x.Callee.ID == y.Callee.ID
	})
}
