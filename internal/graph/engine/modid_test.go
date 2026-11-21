package engine

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/graphtest"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// TestConflicts_AModuleIDAddsNoConflict: a Python file declares its module
// id, which carries no node. The readers of a file's declarations pass it
// by: it has no arity conflict and no caller conflict, and the signature
// edit of sessions.py still raises its two signature_mismatch conflicts and
// no conflict names the module id, at the commit and at the edit.
func TestConflicts_AModuleIDAddsNoConflict(t *testing.T) {
	sp := graphtest.New(t)
	c1 := graph.RevCommit(graphtest.Commit)
	if _, err := sp.Overlay(sessionsPy, oldDef, newDef, 12); err != nil {
		t.Fatal(err)
	}
	ov12 := graph.RevOverlay(graphtest.Commit, 12)
	file := graph.FileID(sessionsPy)
	mod := graph.SymID(pylite.Lang, "services/auth", "mod", "auth.sessions")
	for _, rev := range []graph.Rev{c1, ov12} {
		var declared []graph.ID
		for _, e := range sp.Store.Out(file, rev, graph.Declares) {
			declared = append(declared, e.Dst)
		}
		if !slices.Contains(declared, mod) {
			t.Fatalf("at %s %s declares %v; want %s among them", rev, file, declared, mod)
		}
		if _, ok := sp.Store.Node(mod, rev); ok {
			t.Errorf("at %s the module id has a node", rev)
		}
		if cs := append(ArityConflicts(sp.Store, mod, rev), CallerConflicts(sp.Store, mod, rev)...); len(cs) != 0 {
			t.Errorf("at %s the module id has conflicts %s", rev, describe(cs))
		}
	}
	all := Conflicts(sp.Store, DiffOf(sp.Store, c1, ov12, []graph.ID{file}), nil)
	got := onlyKind(all, ConflictSignatureMismatch)
	if len(got) != 2 || got[0].Edge.Src != handler || got[1].Edge.Src != testCreate {
		t.Errorf("signature_mismatch %s; want api.py then test_sessions.py", cstr(got))
	}
	for _, c := range all {
		if c.Subject == mod || c.Callee.ID == mod || slices.Contains(c.Evidence, mod) {
			t.Errorf("conflict %+v names the module id", c)
		}
	}
}
