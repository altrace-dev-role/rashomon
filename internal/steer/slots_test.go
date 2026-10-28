package steer

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TestSlotsOf_MatchesTheSpecExample: a regression-location Message built
// from the spec example's values writes the example line's slots byte for
// byte, keys sorted (tech 4.2.3): the gate as its raw value, never the
// quoted text it renders as, numbers as numbers, and the path list as
// `paths` and `more` (card 2.2 R10, A2).
func TestSlotsOf_MatchesTheSpecExample(t *testing.T) {
	m, err := render.New(pattern.RegressionLocation, render.RegressionLocationBinding{
		Gate: "go test", Minutes: 6, Exit: 1,
		Paths: slot.NewPathList([]slot.Path{"internal/a.go", "internal/b.go"}, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := slotsOf(m)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"exit":1,"gate":"go test","minutes":6,"more":0,"paths":["internal/a.go","internal/b.go"]}`
	if string(b) != want {
		t.Errorf("slots = %s, want %s", b, want)
	}
	if got, err := slotsOf(render.Message{}); err != nil || len(got) != 0 {
		t.Errorf("the zero Message's slots = %v, %v, want none", got, err)
	}
}

// TestSlotsOf_EverySlotTypeHasARow: every type in package slot that
// implements its sealed Value (a bind method) has a sample here, and every
// sample writes valid JSON under its own name, so a slot type the table
// does not know fails this test instead of reaching a line as a silent
// string. Two slots that would write one key refuse the set.
func TestSlotsOf_EverySlotTypeHasARow(t *testing.T) {
	samples := map[string]slot.Value{
		"Program":   slot.Program("aws"),
		"Gate":      slot.Gate("go test"),
		"Mechanism": slot.Mechanism("AWS_PROFILE"),
		"Approved":  slot.Approved("use the staging profile"),
		"Count":     slot.Count(3),
		"Exit":      slot.Exit(-2),
		"Minutes":   slot.Minutes(6),
		"TaskID":    slot.TaskID("7"),
		"AgentType": slot.AgentType("reviewer"),
		"RuleID":    slot.RuleID("no-shell-true"),
		"Model":     slot.Model("claude-sonnet-5[1m]"),
		"USD":       slot.USD(1.5),
		"Date":      slot.Date("2026-10-27"),
		"Path":      slot.Path("internal/a.go"),
		"PathList":  slot.NewPathList([]slot.Path{"a.go"}, 2),
		"Enum":      slot.NewEnum(slot.EnumGateKinds, "test, build or lint"),
		"FactValue": slot.FactValue{Kind: slot.FactAWSRegion, Value: "us-east-1"},
	}
	want := map[string]string{
		"Program": `"aws"`, "Gate": `"go test"`, "Mechanism": `"AWS_PROFILE"`, "Approved": `"use the staging profile"`,
		"Count": `3`, "Exit": `-2`, "Minutes": `6`, "TaskID": `"7"`, "AgentType": `"reviewer"`, "RuleID": `"no-shell-true"`,
		"Model": `"claude-sonnet-5[1m]"`, "USD": `1.50`, "Date": `"2026-10-27"`, "Path": `"internal/a.go"`,
		"PathList": `["a.go"]`, "Enum": `"test, build or lint"`, "FactValue": `"us-east-1"`,
	}
	for _, name := range slotTypes(t) {
		v, ok := samples[name]
		if !ok {
			t.Errorf("slot.%s implements Value and has no row in slotsOf's test: add its sample and its JSON form", name)
			continue
		}
		got, err := slotsFrom([]render.Slot{{Name: "s", Value: v}})
		if err != nil {
			t.Errorf("slot.%s: %v", name, err)
			continue
		}
		if string(got["s"]) != want[name] || !json.Valid(got["s"]) {
			t.Errorf("slot.%s writes %s, want %s", name, got["s"], want[name])
		}
		if name == "PathList" && string(got["more"]) != "2" {
			t.Errorf("a path list writes more = %s, want 2", got["more"])
		}
	}
	if _, err := slotsFrom([]render.Slot{{Name: "more", Value: slot.Count(1)}, {Name: "paths", Value: slot.NewPathList([]slot.Path{"a.go"}, 0)}}); err == nil {
		t.Error("a slot named more beside a path list was written over")
	}
	if _, err := slotsFrom([]render.Slot{{Name: "s", Value: nil}}); err == nil {
		t.Error("a nil slot value was written")
	}
}

// slotTypes is every type package slot gives a bind method: the sealed
// Value's implementations.
func slotTypes(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join("render", "slot")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	fset := token.NewFileSet()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "bind" {
				continue
			}
			if id, ok := fn.Recv.List[0].Type.(*ast.Ident); ok {
				seen[id.Name] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) < 17 {
		t.Fatalf("found %d slot types, want at least the 17 this test knows: %v", len(names), names)
	}
	return names
}
