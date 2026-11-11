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

func dupEqual(a, b []Conflict) bool {
	return slices.EqualFunc(a, b, func(x, y Conflict) bool {
		return x.Kind == y.Kind && x.Unit == y.Unit && x.RuleID == y.RuleID && x.Subject == y.Subject &&
			slices.Equal(x.Evidence, y.Evidence) && x.Rev == y.Rev && x.Got == y.Got && x.Want == y.Want &&
			x.Edge.Key() == y.Edge.Key() && x.Callee.ID == y.Callee.ID
	})
}
