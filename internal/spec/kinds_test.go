package spec

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestRuleKind_ClosedListGrowsByThree pins the one list of rule kinds:
// RuleKinds lists every RuleKind constant once and nothing else, Parse
// accepts exactly the listed kinds and refuses any other as before, and the
// three kinds the knowledge graph compiles to (boundary, decision, reuse)
// parse with the fields they carry.
func TestRuleKind_ClosedListGrowsByThree(t *testing.T) {
	consts := ruleKindConsts(t)
	for i, k := range RuleKinds {
		if _, declared := consts[k]; !declared {
			t.Errorf("%q listed but not declared", k)
		}
		if slices.Index(RuleKinds, k) != i {
			t.Errorf("%q listed twice", k)
		}
	}
	for k, name := range consts {
		if !slices.Contains(RuleKinds, k) {
			t.Errorf("%s = %q declared but not listed", name, k)
		}
	}
	for _, k := range []RuleKind{Boundary, Decision, Reuse} {
		if !slices.Contains(RuleKinds, k) {
			t.Errorf("%q not listed", k)
		}
	}
	if len(RuleKinds) != 8 {
		t.Errorf("%d rule kinds, want 8: the table's five and the three of the knowledge graph", len(RuleKinds))
	}
	if want := []string{"paths_untouched", "symbol_present"}; !slices.Equal(RulePredicates, want) {
		t.Errorf("RulePredicates %q, want %q", RulePredicates, want)
	}
	for _, k := range UnenforcedRuleKinds {
		if !slices.Contains(RuleKinds, k) {
			t.Errorf("unenforced kind %q is not a listed kind", k)
		}
	}

	var rows []row
	for _, k := range RuleKinds {
		rows = append(rows, ok("rule kind "+string(k), withRule(kv{"kind", q(string(k))})))
	}
	runRows(t, append(rows,
		bad("rule kind unknown", withRule(kv{"kind", `"forbid_file"`}), ReasonEnum, "/rules/0/kind"),
		bad("rule kind in the wrong case", withRule(kv{"kind", `"Boundary"`}), ReasonEnum, "/rules/0/kind"),
	))

	b := rule(kv{"id", `"b1"`}, kv{"kind", `"boundary"`}, kv{"from", `["unit:go:internal/graph"]`},
		kv{"to", `["unit:go:internal/hookio"]`}, kv{"source", `"K-12"`})
	d := rule(kv{"id", `"d1"`}, kv{"kind", `"decision"`}, kv{"predicate", `"paths_untouched"`},
		kv{"applies_to", `["file:go:internal/spec/spec.go"]`})
	f, _, err := Parse([]byte(top(`"rules": [` + b + `, ` + d + `]`)))
	if err != nil {
		t.Fatalf("boundary and decision rules refused: %v", err)
	}
	if got := f.Rules[0]; !slices.Equal(got.From, []string{"unit:go:internal/graph"}) ||
		!slices.Equal(got.To, []string{"unit:go:internal/hookio"}) || got.Source != "K-12" {
		t.Errorf("boundary rule read as %+v", got)
	}
	if got := f.Rules[1]; got.Predicate != "paths_untouched" ||
		!slices.Equal(got.AppliesTo, []string{"file:go:internal/spec/spec.go"}) {
		t.Errorf("decision rule read as %+v", got)
	}
}

// ruleKindConsts is every RuleKind constant the package's non-test files
// declare, value to name, read from the source so the list and the
// declarations cannot drift (TestPredicates_Closed's method).
func ruleKindConsts(t *testing.T) map[RuleKind]string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	out := map[RuleKind]string{}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range af.Decls {
			gd, isGen := decl.(*ast.GenDecl)
			if !isGen || gd.Tok != token.CONST {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				id, named := vs.Type.(*ast.Ident)
				if !named || id.Name != "RuleKind" || len(vs.Values) != 1 {
					continue
				}
				lit, isLit := vs.Values[0].(*ast.BasicLit)
				if !isLit {
					continue
				}
				v, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatal(err)
				}
				if prev, dup := out[RuleKind(v)]; dup {
					t.Errorf("%s and %s both spell %q", prev, vs.Names[0].Name, v)
				}
				out[RuleKind(v)] = vs.Names[0].Name
			}
		}
	}
	return out
}
