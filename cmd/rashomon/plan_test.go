package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/knowledge"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// planFx is a repository on disk with an index built in the test: units
// internal/graph (g.go, FooBar) and internal/hookio (h.go, HookIt), plus
// internal/graph/fresh.go on disk and not in the index.
type planFx struct {
	root, rid string
	s         *graph.Store
	rev       graph.Rev
}

const planRID = "github.com/example/plan"

func newPlanFx(t *testing.T) planFx {
	t.Helper()
	root := t.TempDir()
	rev := graph.RevCommit("c1")
	s := graph.NewStore()
	for _, f := range []struct{ unit, rel, sym string }{
		{"internal/graph", "internal/graph/g.go", "FooBar"},
		{"internal/hookio", "internal/hookio/h.go", "HookIt"},
	} {
		p := filepath.Join(root, filepath.FromSlash(f.rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		fid := graph.FileID(f.rel)
		s.AssertNode(graph.Node{ID: fid, Kind: "file", Rev: rev})
		s.AssertEdge(graph.Edge{Src: graph.UnitID("go", f.unit), Pred: graph.Contains, Dst: fid, Rev: rev})
		sid := graph.SymID("go", f.unit, "func", f.sym)
		s.AssertNode(graph.Node{ID: sid, Kind: "sym", Attrs: map[string]string{"name": f.sym, "symkind": "func"}, Rev: rev})
		s.AssertEdge(graph.Edge{Src: fid, Pred: graph.Declares, Dst: sid, Rev: rev})
	}
	if err := os.WriteFile(filepath.Join(root, "internal/graph/fresh.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := graph.SaveFile(root, s, graph.Header{Repo: planRID, Rev: graph.HeaderRev{Commit: "c1"}}); err != nil {
		t.Fatal(err)
	}
	return planFx{root: root, rid: planRID, s: s, rev: rev}
}

// spec1 is a compiled spec with one source of the given trust holding rules.
func spec1(trust spec.Trust, rules ...spec.Rule) *spec.Compiled {
	sp := &spec.Compiled{Sources: []spec.Source{{Path: "spec.json", Trust: trust}}}
	for _, r := range rules {
		sp.Rules = append(sp.Rules, spec.DeclaredRule{ID: r.ID, Source: 0, Rule: r})
	}
	return sp
}

func hookioBoundary(from, to string) spec.Rule {
	return spec.Rule{ID: "b1", Kind: string(spec.Boundary), Why: "w", HardControl: "h",
		From: []string{"unit:go:" + from}, To: []string{"unit:go:" + to}}
}

// TestPlan_NamesUnitsFromThePlanText: the plan's paths and symbols become
// the diff: a path the index has and a path it lacks (on disk, claimed by
// the Go resolver) are both Files, and each one's unit and the named
// symbol's unit are Touched. The diff has no Decls, Removed or Base.
func TestPlan_NamesUnitsFromThePlanText(t *testing.T) {
	p := newPlanFx(t)
	plan := []byte("change internal/hookio/h.go and add internal/graph/fresh.go and then call FooBar")
	got, ok := planNamesOf(plan, p.root)
	if !ok {
		t.Fatal("no names with an index present")
	}
	wantFiles := []graph.ID{"file:internal/graph/fresh.go", "file:internal/hookio/h.go"}
	if !slices.Equal(got.Diff.Files, wantFiles) {
		t.Errorf("Files = %v, want %v", got.Diff.Files, wantFiles)
	}
	wantUnits := []graph.ID{"unit:go:internal/graph", "unit:go:internal/hookio"}
	if !slices.Equal(got.Diff.Touched, wantUnits) {
		t.Errorf("Touched = %v, want %v", got.Diff.Touched, wantUnits)
	}
	if got.Diff.Rev != p.rev || got.Diff.Base != (graph.Rev{}) || len(got.Diff.Decls) != 0 || len(got.Diff.Removed) != 0 {
		t.Errorf("diff = %+v, want Rev only", got.Diff)
	}
	if got.Repo != p.rid {
		t.Errorf("Repo = %q, want %q", got.Repo, p.rid)
	}

	// A symbol alone names its unit and no file.
	only, ok := planNamesOf([]byte("rework HookIt please"), p.root)
	if !ok || len(only.Diff.Files) != 0 || !slices.Equal(only.Diff.Touched, []graph.ID{"unit:go:internal/hookio"}) {
		t.Errorf("symbol alone: %+v ok=%v", only.Diff, ok)
	}
}

// TestPlan_NoIndexYieldsNothing: no index under cwd is no output; an index
// and a plan that names nothing is no output either.
func TestPlan_NoIndexYieldsNothing(t *testing.T) {
	bare := t.TempDir()
	if err := os.WriteFile(filepath.Join(bare, "a.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sp := spec1(spec.Managed, spec.Rule{ID: "p1", Kind: string(spec.ProtectPath), Why: "w", HardControl: "h", Paths: []string{"a.go"}})
	if lines, ok := PlanLines([]byte("edit a.go now"), bare, sp); ok || lines != nil {
		t.Errorf("no index: %q ok=%v, want nil false", lines, ok)
	}
	p := newPlanFx(t)
	if lines, ok := PlanLines([]byte("think about it"), p.root, sp); ok || lines != nil {
		t.Errorf("a plan naming nothing: %q ok=%v, want nil false", lines, ok)
	}
}

// TestPlan_ConflictLineAtNudge: a protect_path rule on a planned new file
// leads, a decision row's paths_untouched follows, at most two lines; with
// no path rule the boundary caution is the line, worded as a direction and
// never as a conflict found.
func TestPlan_ConflictLineAtNudge(t *testing.T) {
	p := newPlanFx(t)
	row := knowledge.Row{Schema: 1, ID: "D1", Kind: knowledge.KindDecision, Title: "leave hookio", Clause: "hookio stays as is",
		Status: knowledge.StatusApproved, Tier: knowledge.TierDeclared,
		AppliesTo:  []string{string(graph.GlobalID(p.rid, "file:internal/hookio/h.go"))},
		Predicates: []knowledge.Predicate{{Kind: spec.PredicatePathsUntouched}}}
	if _, err := knowledge.WriteFile(p.root, "d", []knowledge.Row{row}); err != nil {
		t.Fatal(err)
	}
	sp := spec1(spec.Managed,
		spec.Rule{ID: "p1", Kind: string(spec.ProtectPath), Why: "w", HardControl: "h", Paths: []string{"internal/graph/fresh.go"}},
		hookioBoundary("internal/graph", "internal/hookio"))
	plan := []byte("change internal/hookio/h.go and add internal/graph/fresh.go and call FooBar")

	lines, ok := PlanLines(plan, p.root, sp)
	if !ok || len(lines) != 2 {
		t.Fatalf("lines = %q ok=%v, want two", lines, ok)
	}
	if !strings.Contains(lines[0], "p1") || !strings.Contains(lines[0], "internal/graph/fresh.go") {
		t.Errorf("first line = %q, want the protect_path rule on the new file", lines[0])
	}
	if !strings.Contains(lines[1], "internal/hookio/h.go") || !strings.Contains(lines[1], "D1") {
		t.Errorf("second line = %q, want the decision on h.go from row D1", lines[1])
	}

	// No path rule: the caution alone.
	bnd := spec1(spec.Managed, hookioBoundary("internal/graph", "internal/hookio"))
	lines, ok = PlanLines([]byte("call FooBar and HookIt together"), p.root, bnd)
	if !ok || len(lines) != 1 {
		t.Fatalf("caution: lines = %q ok=%v, want one", lines, ok)
	}
	want := "the plan names unit:go:internal/graph and unit:go:internal/hookio; rule b1 forbids edges from unit:go:internal/graph into unit:go:internal/hookio"
	if lines[0] != want || strings.Contains(lines[0], "conflict") {
		t.Errorf("caution line = %q, want %q", lines[0], want)
	}
}
