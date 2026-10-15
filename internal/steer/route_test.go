package steer

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"reflect"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// TestContext_IsEmptyPlaceholder pins that Context is not frozen in P-07
// (the gaps table closes it before P-30 opens): zero fields today, so adding
// the real ones later is additive, not a breaking change to anything that
// depends only on the type existing.
func TestContext_IsEmptyPlaceholder(t *testing.T) {
	if n := reflect.TypeOf(Context{}).NumField(); n != 0 {
		t.Errorf("Context has %d fields, want 0 (not frozen until P-30)", n)
	}
}

// TestRoute_StubReturnsEmptyOutput pins this package's one deliberate
// behavioural gap: Route decides nothing yet (tech 4.2.4's router is P-30's),
// and the zero Output is the only honest value for "no decision was made."
func TestRoute_StubReturnsEmptyOutput(t *testing.T) {
	findings := []Finding{{Pattern: pattern.SubtaskGate, AfterSeq: 1}}
	got := Route(&Context{}, findings)
	if got != (Output{}) {
		t.Errorf("Route() = %+v, want the zero Output", got)
	}
}

func TestRoute_StubIgnoresNilInputs(t *testing.T) {
	got := Route(nil, nil)
	if got != (Output{}) {
		t.Errorf("Route(nil, nil) = %+v, want the zero Output", got)
	}
}

// TestDEL22f_RouteAdmitsOnlyCheckedFindings ties Finding.Check to Route
// structurally, so the tie does not rest on P-30 remembering it (P07N-14):
// Route's body is admit then route, with nothing between them, and admit
// keeps a Finding only when Check passes, so a message riding another
// pattern's finding, a kind outside the frozen list or evidence after
// after_seq never reaches the router.
func TestDEL22f_RouteAdmitsOnlyCheckedFindings(t *testing.T) {
	good := Finding{Pattern: pattern.SubtaskGate, Priority: 90, MaxLevel: LevelStop, StopForm: StopFormDeny, Evidence: []int64{3}, AfterSeq: 5}
	bad := []Finding{
		{Pattern: pattern.SubtaskGate, Kind: "not-a-kind", AfterSeq: 5},
		{Pattern: pattern.SubtaskGate, Evidence: []int64{9}, AfterSeq: 5},
		{Pattern: pattern.ClaimVsRecord, Kind: "masked", MaxLevel: LevelStop, StopForm: StopFormStopContinue, AfterSeq: 5},
	}
	got := admit(append([]Finding{bad[0], good, bad[1]}, bad[2]))
	if len(got) != 1 || got[0].Pattern != good.Pattern || got[0].StopForm != good.StopForm {
		t.Errorf("admit kept %+v, want only the finding that passes Check", got)
	}
	if got := admit(nil); len(got) != 0 {
		t.Errorf("admit(nil) = %v", got)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "route.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var body []ast.Stmt
	for _, d := range f.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "Route" && fn.Recv == nil {
			body = fn.Body.List
		}
	}
	want := "return route(ctx, admit(findings))"
	if len(body) != 1 || source(t, fset, body[0]) != want {
		t.Errorf("Route's body is not %q; every finding must pass Check before the router sees it", want)
	}
}

// source prints a statement as gofmt would.
func source(t *testing.T, fset *token.FileSet, n ast.Node) string {
	t.Helper()
	var b strings.Builder
	if err := printer.Fprint(&b, fset, n); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
