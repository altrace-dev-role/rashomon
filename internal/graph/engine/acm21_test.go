package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// TestConflicts_NoneOnThisRepositoryAtHead: AC-M2.1, the live read. It
// reads a synced root's index under RASHOMON_ACM21=1 (RASHOMON_ACM21_DIR
// names another root) and skips otherwise, as ACM10, ACM11 and K7 do: CI
// has no pgraph sidecars. The declared boundary comes from the real import
// graph: no unit under internal/graph reaches internal/hookio. Three reads:
// no conflict on the empty diff; no boundary_cross with every file's
// records against an empty base (the tree as committed, not only what
// changed); exactly one conflict on an overlay that adds one call from a
// symbol of internal/graph/engine into internal/hookio. The fixture twin of
// the three reads is TestConflicts_BoundaryCrossOnNewEdgeOnly.
func TestConflicts_NoneOnThisRepositoryAtHead(t *testing.T) {
	if os.Getenv("RASHOMON_ACM21") != "1" {
		t.Skip("AC-M2.1 live read: set RASHOMON_ACM21=1 on a synced root")
	}
	root := os.Getenv("RASHOMON_ACM21_DIR")
	if root == "" {
		root = filepath.Join("..", "..", "..")
	}
	s, h, err := graph.LoadFile(root)
	if err != nil {
		t.Fatalf("index under %s: %v", root, err)
	}
	rev := graph.RevCommit(h.Rev.Commit)
	rules := []Rule{{ID: "acm21", Kind: "boundary", From: []string{"unit:go:internal/graph"}, To: []string{"unit:go:internal/hookio"}}}

	if got := Conflicts(s, Diff{Rev: rev}, rules); len(got) != 0 {
		t.Fatalf("empty diff at %s: %s, want none", rev, cstr(got))
	}

	var files []graph.ID
	var src, dst graph.ID
	for _, n := range s.Nodes(rev) {
		id := string(n.ID)
		switch {
		case n.Kind == "file":
			files = append(files, n.ID)
		case src == "" && strings.HasPrefix(id, "sym:go:internal/graph/engine:func:"):
			src = n.ID
		case dst == "" && strings.HasPrefix(id, "sym:go:internal/hookio:func:"):
			dst = n.ID
		}
	}
	if len(files) == 0 || src == "" || dst == "" {
		t.Fatalf("index at %s: %d files, engine func %q, hookio func %q", rev, len(files), src, dst)
	}
	all := DiffOf(s, graph.RevCommit("acm21-empty-base"), rev, files)
	if got := onlyKind(Conflicts(s, all, rules), ConflictBoundaryCross); len(got) != 0 {
		t.Fatalf("the tree at %s against an empty base: %s, want no boundary_cross", rev, cstr(got))
	}

	ov := graph.RevOverlay(rev.Commit, 1)
	s.AssertEdge(graph.Edge{Src: src, Pred: graph.Calls, Dst: dst, Rev: ov})
	var decl []graph.ID
	for _, e := range s.In(src, rev, graph.Declares) {
		decl = append(decl, e.Src)
	}
	if len(decl) != 1 {
		t.Fatalf("%s declared by %q, want one file", src, decl)
	}
	got := Conflicts(s, DiffOf(s, rev, ov, decl), rules)
	if len(got) != 1 || got[0].Kind != ConflictBoundaryCross || got[0].Subject != src || got[0].Edge.Dst != dst {
		t.Fatalf("a synthetic call %s -> %s: %s, want one boundary_cross", src, dst, cstr(got))
	}
	t.Logf("AC-M2.1 at %s: %d files, %d records against an empty base; synthetic %s -> %s in %s: one boundary_cross",
		rev, len(files), len(all.Decls), src, dst, decl[0])
}
