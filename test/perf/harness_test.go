package perf

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/budget"
)

// The harness itself (rss_test.go) is behind the perf tag, so no untagged
// test runs it; what it decides with is in runs.go and tested here, and the
// tests below read its source to pin that it calls those decisions where a
// measurement depends on them, as ci_test.go pins the workflow by reading it.

// harnessSource parses rss_test.go; the parser ignores build constraints.
func harnessSource(t *testing.T) *ast.File {
	t.Helper()
	path := filepath.Join(moduleRoot(t), "test", "perf", "rss_test.go")
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// harnessFunc is the function or method named name in the harness.
func harnessFunc(t *testing.T, f *ast.File, name string) *ast.FuncDecl {
	t.Helper()
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == name && fd.Body != nil {
			return fd
		}
	}
	t.Fatalf("the harness has no func %s", name)
	return nil
}

// callsIn is every call under n, written out.
func callsIn(n ast.Node) []string {
	var out []string
	ast.Inspect(n, func(x ast.Node) bool {
		if c, ok := x.(*ast.CallExpr); ok {
			out = append(out, types.ExprString(c))
		}
		return true
	})
	return out
}

// firstStmt is the index of the first statement of list with a call
// starting with prefix, or -1.
func firstStmt(list []ast.Stmt, prefix string) int {
	for i, s := range list {
		for _, c := range callsIn(s) {
			if strings.HasPrefix(c, prefix) {
				return i
			}
		}
	}
	return -1
}

// failsOnErr reports whether s is `if err != nil { ... }` whose first
// statement fails: a return of a nonzero literal code, or t.Fatal(f).
func failsOnErr(s ast.Stmt) bool {
	is, ok := s.(*ast.IfStmt)
	if !ok || types.ExprString(is.Cond) != "err != nil" || len(is.Body.List) == 0 {
		return false
	}
	switch b := is.Body.List[0].(type) {
	case *ast.ReturnStmt:
		if len(b.Results) == 0 {
			return false
		}
		lit, ok := b.Results[0].(*ast.BasicLit)
		return ok && lit.Kind == token.INT && lit.Value != "0"
	case *ast.ExprStmt:
		c := types.ExprString(b.X)
		return strings.HasPrefix(c, "t.Fatal(") || strings.HasPrefix(c, "t.Fatalf(")
	}
	return false
}

// guardedBlock finds the block under n with a statement that calls prefix
// and fails on its error, as `if err := prefix(...); err != nil` or as the
// call followed by `if err != nil`; it returns the block and the index.
func guardedBlock(n ast.Node, prefix string) ([]ast.Stmt, int) {
	var list []ast.Stmt
	pos := -1
	ast.Inspect(n, func(x ast.Node) bool {
		b, ok := x.(*ast.BlockStmt)
		if !ok || pos >= 0 {
			return pos < 0
		}
		for i, s := range b.List {
			is, isIf := s.(*ast.IfStmt)
			inInit := isIf && is.Init != nil && firstStmt([]ast.Stmt{is.Init}, prefix) == 0 && failsOnErr(s)
			before := !isIf && firstStmt([]ast.Stmt{s}, prefix) == 0 && i+1 < len(b.List) && failsOnErr(b.List[i+1])
			if inInit || before {
				list, pos = b.List, i
				return false
			}
		}
		return true
	})
	return list, pos
}

// TestRES36i_PreflightRefusesAForeignToolchain: the harness's preflight
// reads budgets.json and refuses a run under any toolchain but the one its
// rows were measured under, naming both, before anything is built; an
// unreadable or malformed file fails rather than yielding empty rows that
// gate nothing.
func TestRES36i_PreflightRefusesAForeignToolchain(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	ok := write("ok.json", `{"go_version": "go1.27.2", "entries": [{"id": "hook.bash_short"}]}`)
	b, err := Preflight(ok, "go1.27.2")
	if err != nil || b.GoVersion != "go1.27.2" || len(b.Entries) != 1 {
		t.Fatalf("matching toolchain: %+v, %v", b, err)
	}
	if _, err := Preflight(ok, "go1.25.8"); err == nil || !strings.Contains(err.Error(), "go1.25.8") || !strings.Contains(err.Error(), "go1.27.2") {
		t.Errorf("foreign toolchain: %v", err)
	}
	if _, err := Preflight(write("none.json", `{"entries": []}`), "go1.27.2"); err == nil || !strings.Contains(err.Error(), "go_version") {
		t.Errorf("a file naming no toolchain: %v", err)
	}
	if _, err := Preflight(write("bad.json", `{"go_version": "go1.27.2",`), "go1.27.2"); err == nil {
		t.Error("a malformed budgets.json passed the preflight")
	}
	if _, err := Preflight(filepath.Join(dir, "absent.json"), "go1.27.2"); err == nil {
		t.Error("a missing budgets.json passed the preflight")
	}
}

// TestRES36j_HarnessPreflightsBeforeItBuilds: TestMain runs buildAndRun, and
// buildAndRun calls Preflight with runtime.Version() and returns a nonzero
// code on its error before it builds the binary or runs a test, so a perf
// run under go1.25.8 against go1.27.2 rows fails with the reason instead of
// comparing (the CI failure on #59).
func TestRES36j_HarnessPreflightsBeforeItBuilds(t *testing.T) {
	f := harnessSource(t)
	if firstStmt(harnessFunc(t, f, "TestMain").Body.List, "buildAndRun(m)") < 0 {
		t.Error("TestMain does not run buildAndRun(m)")
	}
	body := harnessFunc(t, f, "buildAndRun").Body.List
	pre := -1
	for i, s := range body {
		as, ok := s.(*ast.AssignStmt)
		if !ok || len(as.Rhs) != 1 {
			continue
		}
		if c, ok := as.Rhs[0].(*ast.CallExpr); ok && types.ExprString(c.Fun) == "Preflight" && len(c.Args) == 2 && types.ExprString(c.Args[1]) == "runtime.Version()" {
			pre = i
			break
		}
	}
	if pre < 0 {
		t.Fatal("buildAndRun does not call Preflight(budgets.json, runtime.Version())")
	}
	if pre+1 >= len(body) || !failsOnErr(body[pre+1]) {
		t.Error("buildAndRun goes on after Preflight's error instead of returning a nonzero code")
	}
	for _, later := range []string{`exec.Command("go", "build"`, "m.Run()"} {
		if at := firstStmt(body, later); at <= pre {
			t.Errorf("%s at statement %d does not follow Preflight at %d", later, at, pre)
		}
	}
}

// TestRES28n_SettleFailsARunThatSkippedAnOwedGate: a required run (CI sets
// RASHOMON_PERF_REQUIRE_ALL=1) that passed fails for every owed gate it did
// not run, naming them; a failed run keeps its own code; an unrequired run
// is not judged on what it skipped.
func TestRES28n_SettleFailsARunThatSkippedAnOwedGate(t *testing.T) {
	b := budget.Budgets{Entries: []budget.Entry{{ID: "hook.a"}}}
	all := map[string]bool{EntryGate("hook.a"): true, InitGate: true}
	if code, err := Settle(0, true, b, all); code != 0 || err != nil {
		t.Errorf("whole required run: %d, %v", code, err)
	}
	code, err := Settle(0, true, b, map[string]bool{InitGate: true})
	if code == 0 || err == nil || !strings.Contains(err.Error(), "entry hook.a") || !strings.Contains(err.Error(), RequireAllEnv) {
		t.Errorf("required run skipped entry hook.a: %d, %v", code, err)
	}
	if code, err := Settle(1, true, b, nil); code != 1 || err != nil {
		t.Errorf("a failed run keeps its code: %d, %v", code, err)
	}
	if code, err := Settle(0, false, b, nil); code != 0 || err != nil {
		t.Errorf("unrequired run: %d, %v", code, err)
	}
	if RequireAllEnv != "RASHOMON_PERF_REQUIRE_ALL" {
		t.Errorf("RequireAllEnv = %q, the name ci.yaml sets", RequireAllEnv)
	}
}

// TestRES28o_HarnessChecksEveryRunItMeasures: the harness's measurements go
// through runs.go's decisions: medianRSS fails a run WorkDone refuses before
// keeping its figure, a seeded probe start is checked the same way, every
// entry row is measured on SeedFor's store, and the run's exit code is
// Settle's, required when RASHOMON_PERF_REQUIRE_ALL is 1.
func TestRES28o_HarnessChecksEveryRunItMeasures(t *testing.T) {
	f := harnessSource(t)
	list, pos := guardedBlock(harnessFunc(t, f, "medianRSS"), "WorkDone(command, ")
	if pos < 0 {
		t.Error("medianRSS keeps a run without failing it on WorkDone")
	} else if keep := firstStmt(list, "append(xs, rss)"); keep <= pos {
		t.Errorf("medianRSS keeps the figure (statement %d) before WorkDone judges it (statement %d)", keep, pos)
	}
	if _, pos := guardedBlock(harnessFunc(t, f, "seed"), "WorkDone(start, "); pos < 0 {
		t.Error("seed does not fail a probe start that did no work")
	}
	list, pos = guardedBlock(harnessFunc(t, f, "TestRES28b_EveryEntryStaysWithinItsRow"), "SeedFor(e)")
	if pos < 0 {
		t.Fatal("the entry gate does not take each row's store from SeedFor, failing on its error")
	}
	seeded, measured := firstStmt(list, "r.seed(t, sd)"), firstStmt(list, "r.medianRSS(t, e.Command, ")
	if seeded <= pos || measured <= seeded {
		t.Errorf("the entry gate measures (statement %d) without seeding SeedFor's store first (SeedFor %d, seed %d)", measured, pos, seeded)
	}
	settled := false
	ast.Inspect(harnessFunc(t, f, "buildAndRun"), func(x ast.Node) bool {
		if r, ok := x.(*ast.ReturnStmt); ok && len(r.Results) == 1 {
			settled = settled || types.ExprString(r.Results[0]) == `Settle(m.Run(), os.Getenv(RequireAllEnv) == "1", b, ran)`
		}
		return true
	})
	if !settled {
		t.Error(`buildAndRun does not return Settle(m.Run(), os.Getenv(RequireAllEnv) == "1", b, ran)`)
	}
}
