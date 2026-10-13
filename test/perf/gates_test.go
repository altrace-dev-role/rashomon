package perf

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/budget"
	"github.com/altrace-dev-role/rashomon/internal/install"
)

// moduleRoot is the repository root, two levels up from this package.
func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// loadBudgets reads test/perf/budgets.json.
func loadBudgets(t *testing.T) budget.Budgets {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(moduleRoot(t), "test", "perf", "budgets.json"))
	if err != nil {
		t.Fatal(err)
	}
	var b budget.Budgets
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatal(err)
	}
	return b
}

// TestDEL7_EveryEntryHasABudgetRow: every event the installer subscribes to
// is served by an entry row in budgets.json, and every row names an event
// the installer knows, so an entry cannot gain work without a budget (DEL-7).
func TestDEL7_EveryEntryHasABudgetRow(t *testing.T) {
	b := loadBudgets(t)
	covered := map[string]string{}
	for _, e := range b.Entries {
		if e.ID == "" || len(e.Command) == 0 || e.PayloadClass == "" || e.MaxRSSMiBP50 <= 0 || e.MaxRSSMiBP95 < e.MaxRSSMiBP50 {
			t.Errorf("entry %+v is not a complete budget row", e)
		}
		for _, ev := range e.Events {
			covered[ev] = e.ID
		}
	}
	for _, ev := range install.Events {
		if covered[ev] == "" {
			t.Errorf("install event %s has no budget row", ev)
		}
	}
	// The floor row of an entry, its smallest payload class, is the one every
	// later class is read against; hook and post must keep theirs.
	floor := map[string]bool{}
	for _, e := range b.Entries {
		floor[e.Command[0]+"."+e.PayloadClass] = true
	}
	for _, want := range []string{"hook.bash_short", "post.bash_short"} {
		if !floor[want] {
			t.Errorf("the floor row %s is missing", want)
		}
	}
	known := map[string]bool{}
	for _, ev := range install.Events {
		known[ev] = true
	}
	for ev, id := range covered {
		if !known[ev] {
			t.Errorf("entry %s names an event the installer does not install: %s", id, ev)
		}
	}
	for _, c := range b.RSSCases {
		if want := round4(budget.RES28BudgetMiB(c.PayloadBytes, c.SlopeMiBPerMiB)); c.BudgetMiB != want {
			t.Errorf("rss case %s: budget %v, its line gives %v", c.ID, c.BudgetMiB, want)
		}
		if c.SlopeMiBPerMiB != 0 && c.Basis == "" {
			t.Errorf("rss case %s: slope %v off RES-28's line with no basis", c.ID, c.SlopeMiBPerMiB)
		}
		if c.GatesFrom != "P-08" && c.GatesFrom != "P-10" {
			t.Errorf("rss case %s: gates_from %q", c.ID, c.GatesFrom)
		}
	}
}

func round4(x float64) float64 { return float64(int64(x*10000+0.5)) / 10000 }

// TestRES36d_RowsCarryNoHiddenMargin: every benchmark row's max equals its
// measurement, so the gate's ten percent is the only margin (RES-36).
func TestRES36d_RowsCarryNoHiddenMargin(t *testing.T) {
	b := loadBudgets(t)
	for _, r := range b.Benchmarks {
		if r.MeasuredBytesPerOp <= 0 || r.MeasuredAllocsPerOp <= 0 {
			t.Errorf("%s: no measurement recorded", r.Name)
		}
		if r.BytesPerOpMax != r.MeasuredBytesPerOp || r.AllocsPerOpMax != r.MeasuredAllocsPerOp {
			t.Errorf("%s: max %d/%d differs from the measurement %d/%d: a margin hides in the row", r.Name, r.BytesPerOpMax, r.AllocsPerOpMax, r.MeasuredBytesPerOp, r.MeasuredAllocsPerOp)
		}
	}
	if !strings.Contains(b.BenchmarkRows, "only margin") {
		t.Error("budgets.json does not state that the gate's ten percent is the only margin")
	}
}

// TestRES36_GeneratedBudgetsMatch: the table compiled into the binary is the
// JSON file (RES-36).
func TestRES36_GeneratedBudgetsMatch(t *testing.T) {
	want := loadBudgets(t)
	got, err := budget.Table()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("budgets_gen.go differs from budgets.json: run `go generate ./internal/budget`\n got %+v\nwant %+v", got, want)
	}
}

// TestRES36_BudgetsFileCoversEveryBenchmark: every benchmark in the module
// has a budget row and every row names a benchmark that exists, in both
// directions, so a benchmark cannot be added or dropped without its budget
// (RES-36). Benchmarks are found by parsing the test files: a top-level
// `func BenchmarkX(b *testing.B)` and, inside it, `b.Run("name", ...)` with
// a literal name; a b.Run whose name is not a literal fails the test, because
// the gate could not name what it would be gating.
func TestRES36_BudgetsFileCoversEveryBenchmark(t *testing.T) {
	b := loadBudgets(t)
	found := map[string]bool{}
	root := moduleRoot(t)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "docs" || d.Name() == "skills") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		for _, name := range benchmarksIn(t, path) {
			found[name] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	budgeted := map[string]bool{}
	for _, row := range b.Benchmarks {
		budgeted[row.Name] = true
		if !found[row.Name] {
			t.Errorf("budgets.json names %s, which no test file defines", row.Name)
		}
	}
	for name := range found {
		if !budgeted[name] {
			t.Errorf("benchmark %s has no row in budgets.json", name)
		}
	}
}

// benchmarksIn lists the benchmark names a test file defines, sub-benchmarks
// included as Parent/child. A sub-benchmark's name is read statically: a
// string literal, or, for a table-driven loop, the literal values the loop
// ranges over (a field of each struct element when the name is a selector
// on the loop variable, or the integer elements when the name is
// strconv.Itoa of it). Any other form fails the test: the gate must be able
// to name what it gates without running it.
func benchmarksIn(t *testing.T, path string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	var names []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Benchmark") || fn.Body == nil {
			continue
		}
		subs, ok := subBenchmarkNames(fn.Body)
		if !ok {
			t.Errorf("%s: %s calls Run with a name this test cannot read statically (%s)", path, fn.Name.Name, fset.Position(fn.Pos()))
			continue
		}
		if len(subs) == 0 {
			names = append(names, fn.Name.Name)
		}
		for _, s := range subs {
			names = append(names, fn.Name.Name+"/"+s)
		}
	}
	sort.Strings(names)
	return names
}

// subBenchmarkNames resolves every b.Run name in body; ok is false when one
// cannot be resolved statically.
func subBenchmarkNames(body *ast.BlockStmt) (names []string, ok bool) {
	ok = true
	ast.Inspect(body, func(n ast.Node) bool {
		rng, isRange := n.(*ast.RangeStmt)
		if isRange {
			// b.Run inside a range: its name comes from the ranged literal.
			resolved, found, good := rangeRunNames(rng)
			if found {
				if !good {
					ok = false
				}
				names = append(names, resolved...)
				return false
			}
			return true
		}
		call, isCall := n.(*ast.CallExpr)
		if !isCall || !isRunCall(call) {
			return true
		}
		lit, isLit := call.Args[0].(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			ok = false
			return true
		}
		names = append(names, strings.Trim(lit.Value, "`\""))
		return true
	})
	return names, ok
}

func isRunCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Run" && len(call.Args) >= 1
}

// rangeRunNames resolves b.Run names inside a range over a composite literal:
// `for _, c := range []struct{name string ...}{{name: "x"}, ...}` with
// `b.Run(c.name, ...)`, or `for _, n := range []int{10, 100}` with
// `b.Run(strconv.Itoa(n), ...)`. found reports whether the range body calls
// Run at all; good reports whether every such call was resolved.
func rangeRunNames(rng *ast.RangeStmt) (names []string, found, good bool) {
	good = true
	loopVar, _ := rng.Value.(*ast.Ident)
	lit, isLit := rng.X.(*ast.CompositeLit)
	ast.Inspect(rng.Body, func(n ast.Node) bool {
		call, isCall := n.(*ast.CallExpr)
		if !isCall || !isRunCall(call) {
			return true
		}
		found = true
		if loopVar == nil || !isLit {
			good = false
			return true
		}
		switch arg := call.Args[0].(type) {
		case *ast.SelectorExpr: // c.name
			id, isIdent := arg.X.(*ast.Ident)
			if !isIdent || id.Name != loopVar.Name {
				good = false
				return true
			}
			for _, elem := range lit.Elts {
				v, okv := structFieldLiteral(elem, arg.Sel.Name)
				if !okv {
					good = false
					return true
				}
				names = append(names, v)
			}
		case *ast.CallExpr: // strconv.Itoa(n)
			fn, isSel := arg.Fun.(*ast.SelectorExpr)
			id, isIdent := fn.X.(*ast.Ident)
			argID, isArgIdent := arg.Args[0].(*ast.Ident)
			if !isSel || !isIdent || id.Name != "strconv" || fn.Sel.Name != "Itoa" || !isArgIdent || argID.Name != loopVar.Name {
				good = false
				return true
			}
			for _, elem := range lit.Elts {
				v, isV := elem.(*ast.BasicLit)
				if !isV || v.Kind != token.INT {
					good = false
					return true
				}
				names = append(names, v.Value)
			}
		default:
			good = false
		}
		return true
	})
	return names, found, good
}

// structFieldLiteral reads the string literal of field in one composite
// element, keyed or positional (positional: field must be the first field,
// which is how every table here is written).
func structFieldLiteral(elem ast.Expr, field string) (string, bool) {
	el, ok := elem.(*ast.CompositeLit)
	if !ok || len(el.Elts) == 0 {
		return "", false
	}
	var value ast.Expr
	for _, e := range el.Elts {
		if kv, isKV := e.(*ast.KeyValueExpr); isKV {
			if k, isK := kv.Key.(*ast.Ident); isK && k.Name == field {
				value = kv.Value
			}
		}
	}
	if value == nil {
		if _, isKV := el.Elts[0].(*ast.KeyValueExpr); isKV {
			return "", false
		}
		value = el.Elts[0]
	}
	lit, ok := value.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	return strings.Trim(lit.Value, "`\""), true
}

// TestRES2_NoPackageLevelRegexpCompile: no package linked into the binary
// compiles a regexp while the process initialises: not in a var initialiser
// or an init function, and not in anything they call (RES-2); a regexp is
// compiled on first use or not at all. The scan is RegexpCompilesAtInit's,
// over every non-test file of every module package `go list -deps` names,
// other platforms' files included.
func TestRES2_NoPackageLevelRegexpCompile(t *testing.T) {
	root := moduleRoot(t)
	cmd := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}} {{.Dir}}{{end}}", "./cmd/rashomon")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 5 {
		t.Fatalf("go list named %d module packages; the binary links more than that", len(lines))
	}
	fset := token.NewFileSet()
	var pkgs []SrcPackage
	for _, ln := range lines {
		importPath, dir, ok := strings.Cut(ln, " ")
		if !ok {
			t.Fatalf("go list line %q", ln)
		}
		pkgs = append(pkgs, SrcPackage{Path: importPath, Files: parseNonTest(t, fset, dir)})
	}
	for _, c := range RegexpCompilesAtInit(pkgs) {
		t.Errorf("regexp compiled at init at %s (%s)", fset.Position(c.Pos), c.Via)
	}
}

// parseNonTest parses every non-test Go file in dir.
func parseNonTest(t *testing.T, fset *token.FileSet, dir string) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		t.Fatalf("%s: no Go files", dir)
	}
	return files
}

// TestRES28h_EveryLineHoldsOnTheReferenceMachine: every rss case records the
// median of nine measured on the reference machine when its line was set,
// and that figure sits at or under the line. A line that holds only through
// the platform tolerance is not within RES-28; it is restated from the
// measurement instead (budgets.json carries both).
func TestRES28h_EveryLineHoldsOnTheReferenceMachine(t *testing.T) {
	b := loadBudgets(t)
	for _, c := range b.RSSCases {
		if err := LineHolds(c.ID, c.MeasuredMiB, c.BudgetMiB); err != nil {
			t.Error(err)
		}
	}
}

// TestRES28h_LineHoldsTakesNoTolerance: the comparison pads nothing and
// refuses a case with no measurement. Break: pad the line with the
// tolerance, or let a zero measurement through, and this fails.
func TestRES28h_LineHoldsTakesNoTolerance(t *testing.T) {
	for _, c := range []struct {
		measured, line float64
		ok             bool
	}{{9.9, 9.9, true}, {5.97, 6.5008, true}, {9.91, 9.9, false}, {10.5, 9.9, false}, {0, 9.9, false}} {
		err := LineHolds("hook.1MiB", c.measured, c.line)
		if (err == nil) != c.ok {
			t.Errorf("LineHolds(%v, %v) = %v, want ok=%v", c.measured, c.line, err, c.ok)
		}
	}
}
