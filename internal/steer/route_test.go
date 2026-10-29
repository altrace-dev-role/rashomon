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

// TestContext_FieldsAreTheRouterInputs pins Context to the router's inputs
// (card 2.2 R2), in order: every one a value the caller already holds or a
// constant until its reader lands, and none a counter, which the router
// derives from the steer log under its lock. A field added without its
// ruling fails here.
func TestContext_FieldsAreTheRouterInputs(t *testing.T) {
	want := []string{
		"Event Event", "Client Client", "Store *store.Store", "SessionID string", "PromptID *string", "AgentID string",
		"Project string", "InvocationID string", "HarnessID string", "Surface string", "NowUnixMS int64",
		"NewID func() string", "Consent bool", "SteerOff bool", "Arm Arm", "Attendance Attendance",
		"StopHookActive bool",
		"Holdout bool", // the hook's draw (card 2.5, D109)
	}
	typ := reflect.TypeOf(Context{})
	var got []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		got = append(got, f.Name+" "+strings.ReplaceAll(f.Type.String(), "steer.", ""))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Context fields = %q, want %q", got, want)
	}
}

// TestRoute_StubReturnsEmptyOutput (the name kept from the stub it
// replaced): with nowhere to write a line -- a Context with no store --
// Route prints nothing, because nothing may print that no line records
// (card 2.2 R1; STR-8).
func TestRoute_StubReturnsEmptyOutput(t *testing.T) {
	findings := []Finding{{Pattern: pattern.SubtaskGate, Priority: 90, MaxLevel: LevelNudge, StopForm: StopFormNone, AfterSeq: 1}}
	got := Route(&Context{Event: EventPostToolUse, SessionID: "s1", Consent: true}, findings)
	if got != (Output{}) {
		t.Errorf("Route() with no store = %+v, want the zero Output", got)
	}
}

// TestRoute_StubIgnoresNilInputs: a nil Context or no finding says nothing.
func TestRoute_StubIgnoresNilInputs(t *testing.T) {
	if got := Route(nil, nil); got != (Output{}) {
		t.Errorf("Route(nil, nil) = %+v, want the zero Output", got)
	}
	if got := Route(&Context{}, nil); got != (Output{}) {
		t.Errorf("Route(&Context{}, nil) = %+v, want the zero Output", got)
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

// TestAdmitCountsWhatItDrops: admit says nothing to the router about a
// finding it refuses, but a producer's bug should not be invisible either
// (P07N-14's own comment defers it to P-30's telemetry). DroppedFindings
// lets P-30 wire that in without touching Route's pinned body.
func TestAdmitCountsWhatItDrops(t *testing.T) {
	good := Finding{Pattern: pattern.SubtaskGate, Priority: 90, MaxLevel: LevelStop, StopForm: StopFormDeny, Evidence: []int64{3}, AfterSeq: 5}
	bad := Finding{Pattern: pattern.SubtaskGate, Kind: "not-a-kind", AfterSeq: 5}

	before := DroppedFindings()
	if got := admit([]Finding{good, bad, bad}); len(got) != 1 {
		t.Fatalf("admit kept %d findings, want 1", len(got))
	}
	if got := DroppedFindings() - before; got != 2 {
		t.Errorf("DroppedFindings() increased by %d, want 2", got)
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
