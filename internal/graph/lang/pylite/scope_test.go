package pylite_test

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
)

// scopeUnit is the unit the scope tests read their files in.
const scopeUnit = "services/auth"

// scopeOverlay is the records of src read as the file rel of the scope unit.
func scopeOverlay(t *testing.T, rel, src string) []graph.Record {
	t.Helper()
	recs, err := graphtest.Resolver().Overlay(rel, []byte(src), graph.RevCommit(graphtest.Commit), 1)
	if err != nil {
		t.Fatal(err)
	}
	return recs
}

// symID is the id of the symbol of kind symkind, qualified name, in the
// scope unit.
func symID(symkind, qualified string) graph.ID {
	return graph.SymID("py", scopeUnit, symkind, qualified)
}

// hasNode reports whether recs hold a node whose id is id.
func hasNode(recs []graph.Record, id graph.ID) bool {
	for _, r := range recs {
		if r.Node != nil && r.Node.ID == id {
			return true
		}
	}
	return false
}

// hasEdge reports whether recs hold the edge src pred dst.
func hasEdge(recs []graph.Record, src graph.ID, pred graph.Pred, dst graph.ID) bool {
	for _, r := range recs {
		if r.Edge != nil && r.Edge.Src == src && r.Edge.Pred == pred && r.Edge.Dst == dst {
			return true
		}
	}
	return false
}

// defBodySrc has a def inside a def, a call after it in the outer body, and
// an import indented in that body.
const defBodySrc = `def helper():
    return 1


def wrap(f):
    return f


def outer():
    def inner():
        helper()
    import orgs
    return wrap(inner)
`

// TestLANGREG1_ADefInsideADefIsItsBody: a def nested in a def's body is no
// symbol of the module. Its lines are the outer def's body, so the calls in
// them are the outer def's, and so is a call after it; an import indented in
// that body is not the module's import. Break: test for a def before the
// body rule (inner gets an id, helper is inner's call, the call of wrap is
// lost and the import of orgs is the module's).
func TestLANGREG1_ADefInsideADefIsItsBody(t *testing.T) {
	rel := "services/auth/auth/nest.py"
	recs := scopeOverlay(t, rel, defBodySrc)
	if id := symID("func", "auth.nest.inner"); hasNode(recs, id) {
		t.Errorf("the nested def has the symbol %s; want none", id)
	}
	outer := symID("func", "auth.nest.outer")
	for _, callee := range []string{"helper", "wrap"} {
		if !hasEdge(recs, outer, graph.Calls, symID("func", "auth.nest."+callee)) {
			t.Errorf("outer has no call of %s; want the call its body makes", callee)
		}
	}
	if hasEdge(recs, graph.FileID(rel), graph.Imports, graph.SymID("py", "services/orgs", "mod", "orgs")) {
		t.Errorf("the import indented in the body of outer is an import of the module; want none")
	}
}
