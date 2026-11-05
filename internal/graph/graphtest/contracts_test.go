package graphtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/knowledge"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
)

// agentKeys are the Claude Code payload names no core package may read:
// an adapter's words (internal/hookio) that a core read would bind the
// engine to one agent (R12, AD9).
var agentKeys = []string{"tool_input", "transcript_path", "hook_event_name"}

// corePackages are the trees the tripwire covers, relative to the module
// root.
var corePackages = []string{"internal/graph", "internal/verify", "internal/learn"}

// TestCore_NoAgentKeys (W-M1.7, R12, AC-X.2): no non-test Go file under the
// core trees imports internal/hookio or holds an agent key in a string
// literal. The baseline at 8593061d was zero on all three trees, so the
// test is strict, not a ratchet.
func TestCore_NoAgentKeys(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	fset := token.NewFileSet()
	for _, pkg := range corePackages {
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(pkg)), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" || d.Name() == "specimen" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return nil
			}
			af, err := parser.ParseFile(fset, p, nil, 0)
			if err != nil {
				return err
			}
			checked++
			rel, _ := filepath.Rel(root, p)
			for _, im := range af.Imports {
				if path, _ := strconv.Unquote(im.Path.Value); strings.HasSuffix(path, "/internal/hookio") {
					t.Errorf("%s imports the agent adapter %s", rel, path)
				}
			}
			ast.Inspect(af, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				s, err := strconv.Unquote(lit.Value)
				if err != nil {
					s = lit.Value
				}
				for _, k := range agentKeys {
					if strings.Contains(s, k) {
						t.Errorf("%s:%d holds the agent key %q", rel, fset.Position(lit.Pos()).Line, k)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked < 30 {
		t.Fatalf("%d core files checked; want the three trees", checked)
	}
}

// TestPredicates_EveryStoreEdgeListed (W-M1.7, AC-X.5): every edge each
// producer writes carries a predicate of the closed vocabulary: the
// specimen (pylite), the shapes fixture (gopg) and a knowledge row's
// records.
func TestPredicates_EveryStoreEdgeListed(t *testing.T) {
	sp := New(t)
	edges := sp.Store.Edges(graph.RevCommit(Commit))
	shapes, err := filepath.Abs(filepath.Join("..", "lang", "gopg", "testdata", "shapes"))
	if err != nil {
		t.Fatal(err)
	}
	recs, err := gopg.Resolver{}.Extract(shapes, nil, graph.RevCommit("c1"))
	if err != nil {
		t.Fatal(err)
	}
	row := knowledge.Row{Schema: knowledge.Schema, ID: "B1", Kind: knowledge.KindLesson, Title: "t", Status: knowledge.StatusCandidate,
		Tier: knowledge.TierInferred, AppliesTo: []string{string(graph.GlobalID("r", "file:a.py"))}}
	recs = append(recs, knowledge.Records([]knowledge.Row{row}, "r", graph.RevCommit("c1"))...)
	for _, r := range recs {
		if r.Edge != nil {
			edges = append(edges, *r.Edge)
		}
	}
	if len(edges) < 20 {
		t.Fatalf("%d edges; want the three producers' edges", len(edges))
	}
	seen := map[graph.Pred]int{}
	for _, e := range edges {
		seen[e.Pred]++
		if !graph.IsPredicate(e.Pred) {
			t.Errorf("edge %s -%s-> %s: predicate outside graph.Predicates", e.Src, e.Pred, e.Dst)
		}
	}
	for _, p := range []graph.Pred{graph.Calls, graph.MayCall, graph.Reads, graph.AppliesTo} {
		if seen[p] == 0 {
			t.Errorf("no %s edge among the producers' records", p)
		}
	}
}
