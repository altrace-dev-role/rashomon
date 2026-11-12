package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/gopg"
)

// importCrossTree is the M2 tip's module layout in small: the root module,
// the nested pgraph module whose path follows its directory, and a testdata
// module whose path does not. The root go.mod requires nothing, so an import
// of pgraph from internal would fail the build until go.mod is edited.
var importCrossTree = map[string]string{
	"go.mod":                       "module github.com/altrace-dev-role/rashomon\n\ngo 1.25.0\n",
	"internal/hook/hook.go":        "package hook\n\nimport \"fmt\"\n\nvar _ = fmt.Sprint\n",
	"internal/graph/graph.go":      "package graph\n\nimport \"strings\"\n\nvar _ = strings.ToUpper\n",
	"cmd/rashomon/main.go":         "package main\n\nimport _ \"github.com/altrace-dev-role/rashomon/internal/hook\"\n\nfunc main() {}\n",
	"pgraph/go.mod":                "module github.com/altrace-dev-role/rashomon/pgraph\n\ngo 1.25.0\n",
	"pgraph/pgraph.go":             "package pgraph\n\nimport \"strings\"\n\nvar _ = strings.ToUpper\n",
	"pgraph/testdata/mod/go.mod":   "module example.test/mod\n\ngo 1.25.0\n",
	"pgraph/testdata/mod/mod.go":   "package mod\n",
	"pgraph/testdata/mod/user.go":  "package mod\n\nimport _ \"example.test/mod/sub\"\n",
	"pgraph/testdata/mod/sub/s.go": "package sub\n",
}

const importCrossCommit = "c-imports"

// importCrossRules are QA's two S1 rules: internal or cmd into pgraph, and
// pgraph into internal or cmd.
var importCrossRules = []Rule{
	{ID: "to-pgraph", Kind: "boundary", From: []string{"unit:go:internal", "unit:go:cmd"}, To: []string{"unit:go:pgraph"}},
	{ID: "from-pgraph", Kind: "boundary", From: []string{"unit:go:pgraph"}, To: []string{"unit:go:internal", "unit:go:cmd"}},
}

// importCrossFixture writes the tree and returns its root, the import map the
// hook would build for it, and a store holding the index's records for every
// .go file: the gopg mapping's records restamped at the commit with the
// index's tier (Extract's import edges equal the overlay's by key:
// TestExtract_ImportEdgesMatchTheOverlays).
func importCrossFixture(t *testing.T, tree map[string]string) (string, *gopg.ImportMap, *graph.Store) {
	t.Helper()
	root := t.TempDir()
	for rel, src := range tree {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := &gopg.ImportMap{Module: gopg.ReadModulePath(root), IsDir: gopg.DirUnder(root)}
	if m.Module != "github.com/altrace-dev-role/rashomon" {
		t.Fatalf("module path %q", m.Module)
	}
	s := graph.NewStore()
	rev := graph.RevCommit(importCrossCommit)
	for rel, src := range tree {
		if !strings.HasSuffix(rel, ".go") {
			continue
		}
		recs, err := gopg.Resolver{Imports: m}.Overlay(rel, []byte(src), rev, 0)
		if err != nil || len(recs) == 0 {
			t.Fatalf("%s: %d records, %v", rel, len(recs), err)
		}
		for _, r := range recs {
			switch {
			case r.Node != nil:
				n := *r.Node
				n.Rev, n.Tier, n.Conf = rev, graph.Proven, 1.0
				s.AssertNode(n)
			case r.Edge != nil:
				e := *r.Edge
				e.Rev, e.Tier, e.Conf = rev, graph.Algorithmic, 1.0
				s.AssertEdge(e)
			}
		}
	}
	return root, m, s
}

// crossOf is the boundary_cross conflicts of an overlay that writes src to rel
// at seq, through the hook's own resolver and carried set.
func crossOf(t *testing.T, s *graph.Store, m *gopg.ImportMap, rel, src string, seq int64) []Conflict {
	t.Helper()
	r := gopg.Resolver{Imports: m}
	base := graph.RevCommit(importCrossCommit)
	recs, err := r.Overlay(rel, []byte(src), base, seq)
	if err != nil || len(recs) == 0 {
		t.Fatalf("overlay %s: %d records, %v", rel, len(recs), err)
	}
	ov := graph.RevOverlay(importCrossCommit, seq)
	s.ApplyOverlay(graph.FileID(rel), []byte(src), recs, ov, r.OverlayCarried())
	return onlyKind(Conflicts(s, DiffOf(s, base, ov, []graph.ID{graph.FileID(rel)}), importCrossRules), ConflictBoundaryCross)
}

// TestConflicts_ImportCrossOnePerDirection (record line 106, clarifications
// 1, 3 and 4): on the M2 tip's layout, an import of the nested pgraph module
// written into internal gives exactly one boundary_cross, and an import of
// internal written into pgraph gives exactly one, each from the file to the
// imported package's unit, with no go.mod require in either direction. The
// unedited tree gives none, and so does an edit that leaves a standing
// import or removes one.
func TestConflicts_ImportCrossOnePerDirection(t *testing.T) {
	root, m, s := importCrossFixture(t, importCrossTree)
	if b, _ := os.ReadFile(filepath.Join(root, "go.mod")); strings.Contains(string(b), "require") {
		t.Fatal("the root go.mod must require nothing")
	}

	seq := int64(1)
	for rel, src := range importCrossTree {
		if strings.HasSuffix(rel, ".go") {
			if got := crossOf(t, s, m, rel, src, seq); len(got) != 0 {
				t.Fatalf("unedited %s: %s, want none", rel, cstr(got))
			}
			seq++
		}
	}

	in := strings.Replace(importCrossTree["internal/hook/hook.go"], "import \"fmt\"",
		"import (\n\t\"fmt\"\n\n\t\"github.com/altrace-dev-role/rashomon/pgraph\"\n)\n\nvar _ = pgraph.X", 1)
	got := crossOf(t, s, m, "internal/hook/hook.go", in, seq)
	seq++
	if len(got) != 1 || got[0].RuleID != "to-pgraph" || got[0].Subject != "file:internal/hook/hook.go" ||
		got[0].Edge.Pred != graph.Imports || got[0].Edge.Dst != "unit:go:pgraph" || got[0].Unit != "unit:go:internal/hook" {
		t.Fatalf("internal to pgraph: %s, want one boundary_cross under to-pgraph", cstr(got))
	}

	out := strings.Replace(importCrossTree["pgraph/pgraph.go"], "import \"strings\"",
		"import (\n\t\"strings\"\n\n\t\"github.com/altrace-dev-role/rashomon/internal/graph\"\n)\n\nvar _ = graph.X", 1)
	got = crossOf(t, s, m, "pgraph/pgraph.go", out, seq)
	seq++
	if len(got) != 1 || got[0].RuleID != "from-pgraph" || got[0].Subject != "file:pgraph/pgraph.go" ||
		got[0].Edge.Pred != graph.Imports || got[0].Edge.Dst != "unit:go:internal/graph" || got[0].Unit != "unit:go:pgraph" {
		t.Fatalf("pgraph to internal: %s, want one boundary_cross under from-pgraph", cstr(got))
	}
}

// TestConflicts_ImportCrossStandingNeverFires: a crossing import the index
// already holds does not fire when its file is edited elsewhere, nor when the
// edit removes it (Removed, not added).
func TestConflicts_ImportCrossStandingNeverFires(t *testing.T) {
	tree := map[string]string{}
	for k, v := range importCrossTree {
		tree[k] = v
	}
	standing := "package hook\n\nimport _ \"github.com/altrace-dev-role/rashomon/pgraph\"\n\nvar A = 1\n"
	tree["internal/hook/hook.go"] = standing
	_, m, s := importCrossFixture(t, tree)
	if got := crossOf(t, s, m, "internal/hook/hook.go", strings.Replace(standing, "A = 1", "A = 2", 1), 1); len(got) != 0 {
		t.Fatalf("an edit beside a standing import: %s, want none", cstr(got))
	}
	if got := crossOf(t, s, m, "internal/hook/hook.go", "package hook\n\nvar A = 1\n", 2); len(got) != 0 {
		t.Fatalf("an edit removing the standing import: %s, want none", cstr(got))
	}
}

// TestConflicts_ImportCrossTestFileNeverFires (D5, a known limit): the index
// holds no _test.go file (pgraph loads ./... without tests,
// TestPL1_FilesKeyedByPathAndContentHash) and the overlay emits no Imports
// edge from one (TestOverlay_TestFileEmitsNoImports), so a crossing import
// written into a test file gives no boundary_cross. The witness writes the
// same source into a non-test file of the same directory and gets one.
func TestConflicts_ImportCrossTestFileNeverFires(t *testing.T) {
	_, m, s := importCrossFixture(t, importCrossTree)
	src := "package hook\n\nimport _ \"github.com/altrace-dev-role/rashomon/pgraph\"\n"
	if got := crossOf(t, s, m, "internal/hook/hook_test.go", src, 1); len(got) != 0 {
		t.Fatalf("a crossing import in a _test.go file: %s, want none (D5)", cstr(got))
	}
	if got := crossOf(t, s, m, "internal/hook/cross.go", src, 2); len(got) != 1 || got[0].RuleID != "to-pgraph" {
		t.Fatalf("witness, the same import in a non-test file: %s, want one to-pgraph", cstr(got))
	}
}

// TestConflicts_ImportCrossBuildExcludedRefires (D6, a known limit): the
// index holds no build-tag-excluded file (pgraph loads under the sync's build
// configuration, TestPL1_FilesKeyedByPathAndContentHash), and the overlay
// cannot evaluate that configuration, so it emits such a file's imports. A
// crossing import standing in an excluded file therefore reads as added on
// every edit of the file. The witness puts the same file in the index, as a
// built file would be, and an edit beside the import gives none
// (TestConflicts_ImportCrossStandingNeverFires).
func TestConflicts_ImportCrossBuildExcludedRefires(t *testing.T) {
	rel := "internal/hook/never.go"
	excluded := "//go:build never\n\npackage hook\n\nimport _ \"github.com/altrace-dev-role/rashomon/pgraph\"\n\nvar A = 1\n"
	edits := []string{excluded, strings.Replace(excluded, "A = 1", "A = 2", 1), strings.Replace(excluded, "A = 1", "A = 3", 1)}

	_, m, s := importCrossFixture(t, importCrossTree)
	for i, src := range edits {
		got := crossOf(t, s, m, rel, src, int64(i+1))
		if len(got) != 1 || got[0].RuleID != "to-pgraph" || got[0].Subject != graph.FileID(rel) || got[0].Edge.Dst != "unit:go:pgraph" {
			t.Fatalf("edit %d of a build-excluded file beside a standing import: %s, want one to-pgraph (D6 re-fire)", i+1, cstr(got))
		}
	}

	tree := map[string]string{}
	for k, v := range importCrossTree {
		tree[k] = v
	}
	tree[rel] = excluded
	_, m, s = importCrossFixture(t, tree)
	if got := crossOf(t, s, m, rel, edits[1], 1); len(got) != 0 {
		t.Fatalf("witness, the file in the index: %s, want none", cstr(got))
	}
}

// TestConflicts_ImportCrossTestdataModuleMapsNothing (clarification 1): a
// module whose path does not follow its directory (example.test/mod under
// pgraph/testdata) maps to nothing, so an import of it, or of anything under
// it, makes no edge and no conflict even under a rule naming its directory.
func TestConflicts_ImportCrossTestdataModuleMapsNothing(t *testing.T) {
	_, m, s := importCrossFixture(t, importCrossTree)
	rules := []Rule{{ID: "td", Kind: "boundary", From: []string{"unit:go:internal"}, To: []string{"unit:go:pgraph"}}}
	src := "package hook\n\nimport _ \"example.test/mod/sub\"\n"
	r := gopg.Resolver{Imports: m}
	recs, err := r.Overlay("internal/hook/hook.go", []byte(src), graph.RevCommit(importCrossCommit), 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, rec := range recs {
		if rec.Edge != nil && rec.Edge.Pred == graph.Imports {
			t.Fatalf("an example.test import made an edge: %+v", *rec.Edge)
		}
	}
	ov := graph.RevOverlay(importCrossCommit, 1)
	s.ApplyOverlay("file:internal/hook/hook.go", []byte(src), recs, ov, r.OverlayCarried())
	if got := onlyKind(Conflicts(s, DiffOf(s, graph.RevCommit(importCrossCommit), ov, []graph.ID{"file:internal/hook/hook.go"}), rules), ConflictBoundaryCross); len(got) != 0 {
		t.Fatalf("%s, want none", cstr(got))
	}
}
