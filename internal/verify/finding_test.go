package verify

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// closedFactTypeNames are the only named types a fact field may have, as a
// literal list: slots with a charset, closed vocabularies and digests. The
// list is written out rather than read off the structs under test, so
// retyping a field to string fails here instead of widening the allowlist.
var closedFactTypeNames = []string{
	"verify.SlotID", "verify.PathSlot", "verify.Program", "verify.QuoteHMAC",
	"verify.State", "verify.Reason", "verify.Verdict", "verify.ClaimClass",
	"verify.ClaimSource", "verify.IntegrityKind", "verify.MarkerID", "spec.RuleKind", "spec.Trust",
}

// TestVER54b_FactFieldsCarryNoFreeText walks the four fact structs: every
// field, behind any pointer or slice, is a number, a flag or one of the
// closed types named above; a bare string anywhere fails (VER-54).
func TestVER54b_FactFieldsCarryNoFreeText(t *testing.T) {
	allowed := map[string]bool{}
	for _, n := range closedFactTypeNames {
		allowed[n] = true
	}
	for _, fact := range []any{GateFact{}, ClaimFact{}, RuleFact{}, IntegrityFact{}} {
		rt := reflect.TypeOf(fact)
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			ft := f.Type
			for ft.Kind() == reflect.Pointer || ft.Kind() == reflect.Slice {
				ft = ft.Elem()
			}
			switch ft.Kind() {
			case reflect.Bool, reflect.Int, reflect.Int64:
				continue
			}
			if !allowed[ft.String()] {
				t.Errorf("%s.%s has type %s: a fact field must be a number, a flag or a closed type", rt.Name(), f.Name, f.Type)
			}
		}
	}
	if allowed["string"] {
		t.Fatal("the allowlist admits bare strings")
	}
}

// TestVER54c_FindingCarriesTheSpecsFields pins 2.2.9's Finding and GateFact field
// lists and that Finding.Pattern is the frozen steer id.
func TestVER54c_FindingCarriesTheSpecsFields(t *testing.T) {
	fields := func(v any) string {
		rt := reflect.TypeOf(v)
		var names []string
		for i := 0; i < rt.NumField(); i++ {
			names = append(names, rt.Field(i).Name)
		}
		return strings.Join(names, ",")
	}
	if got, want := fields(Finding{}), "Pattern,Class,Event,DedupeKey,Gate,Claim,Rule,Integrity,UserOnly"; got != want {
		t.Errorf("Finding fields %s, want %s", got, want)
	}
	if reflect.TypeOf(Finding{}).Field(0).Type != reflect.TypeOf(pattern.ID("")) {
		t.Errorf("Finding.Pattern is %s, want pattern.ID", reflect.TypeOf(Finding{}).Field(0).Type)
	}
	if got, want := fields(GateFact{}), "GateID,Program,State,Reason,LastGreenSeq,LastRunSeq,ExitCode,WritesSince,Paths,PathsOmitted,OtherAgents,PartialRuns"; got != want {
		t.Errorf("GateFact fields %s, want %s", got, want)
	}
	if got, want := fields(ClaimFact{}), "Class,Source,Strict,Verdict,Gates,GatesOmitted,QuoteHMAC,UnrecoveredFailures"; got != want {
		t.Errorf("ClaimFact fields %s, want %s", got, want)
	}
	if got, want := fields(RuleFact{}), "RuleID,Kind,Trust,Seq,Program,Path,Gate,Added,Pragma,Rewritten"; got != want {
		t.Errorf("RuleFact fields %s, want %s", got, want)
	}
	if got, want := fields(IntegrityFact{}), "Kind,Count,Paths,PathsOmitted,Seq,Marker"; got != want {
		t.Errorf("IntegrityFact fields %s, want %s", got, want)
	}
	f := Finding{Pattern: pattern.ClaimVsRecord, Class: FalseClaim, Claim: &ClaimFact{Class: ClaimTestsPass, Verdict: Verdict(Red)}}
	if !pattern.Valid(f.Pattern) {
		t.Errorf("the finding's pattern %q is not a frozen id", f.Pattern)
	}
	if MaxPaths != 5 {
		t.Errorf("MaxPaths = %d, want 5", MaxPaths)
	}
	want := []LabelClass{"false_claim", "absence", "literal", "user_labelled"}
	if !reflect.DeepEqual(LabelClasses, want) {
		t.Errorf("LabelClasses %q, want %q", LabelClasses, want)
	}
}

// TestVER54d_SlotsHoldTheSpecsCharsets pins the slot forms of 2.2.9 and the
// relative-path rule a fact's path must satisfy.
func TestVER54d_SlotsHoldTheSpecsCharsets(t *testing.T) {
	long := strings.Repeat("a", 64)
	for _, c := range []struct {
		id   SlotID
		want bool
	}{
		{"go-test", true}, {"spec:ci:1", true}, {"a.b_c-d", true}, {"0", true}, {SlotID(long), true},
		{"", false}, {"Go", false}, {"-x", false}, {"a b", false}, {SlotID(long + "a"), false},
	} {
		if got := c.id.Valid(); got != c.want {
			t.Errorf("SlotID(%q).Valid() = %v, want %v", c.id, got, c.want)
		}
	}
	longPath := strings.Repeat("p", 120)
	for _, c := range []struct {
		p    PathSlot
		want bool
	}{
		{"internal/verify/state.go", true}, {"a@b+c", true}, {".github/workflows/ci.yaml", true}, {PathSlot(longPath), true},
		{"", false}, {"/etc/passwd", false}, {"../x", false}, {"a/../b", false}, {"a/./b", false}, {"a//b", false},
		{"a b", false}, {"a\x00b", false}, {PathSlot(longPath + "p"), false},
	} {
		if got := c.p.Valid(); got != c.want {
			t.Errorf("PathSlot(%q).Valid() = %v, want %v", c.p, got, c.want)
		}
	}
	for _, c := range []struct {
		p    Program
		want bool
	}{
		{"go", true}, {"golangci-lint", true}, {"python3.12", true}, {"c++", true},
		{"", false}, {"Go", false}, {"a b", false}, {"./run", false},
	} {
		if got := c.p.Valid(); got != c.want {
			t.Errorf("Program(%q).Valid() = %v, want %v", c.p, got, c.want)
		}
	}
	hex := strings.Repeat("0f", 32)
	for _, c := range []struct {
		d    Digest
		want bool
	}{
		{Digest(hex), true}, {Digest(strings.ToUpper(hex)), false}, {Digest(hex[:63]), false}, {"", false},
	} {
		if got := c.d.Valid(); got != c.want {
			t.Errorf("Digest(%q).Valid() = %v, want %v", c.d, got, c.want)
		}
	}
	for _, c := range []struct {
		q    QuoteHMAC
		want bool
	}{
		{QuoteHMAC(QuoteHMACPrefix + hex), true},
		{QuoteHMAC(hex), false}, // no prefix
		{QuoteHMAC(QuoteHMACPrefix + strings.ToUpper(hex)), false},
		{QuoteHMAC(QuoteHMACPrefix + hex[:63]), false},
		{"", false},
	} {
		if got := c.q.Valid(); got != c.want {
			t.Errorf("QuoteHMAC(%q).Valid() = %v, want %v", c.q, got, c.want)
		}
	}
}

// TestVER54j_ProposedFactFieldsCarryTheMark: neither spec defines ClaimFact,
// RuleFact or IntegrityFact, so the gap row asks for "a doc comment per
// field citing its section" until L signs them. Every field of the three
// carries "P-07 proposal" and a section number in its own doc comment, so a
// later author can tell the proposal from spec text (verify F12).
func TestVER54j_ProposedFactFieldsCarryTheMark(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "finding.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"ClaimFact": true, "RuleFact": true, "IntegrityFact": true}
	seen := 0
	ast.Inspect(file, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || !want[ts.Name.Name] {
			return true
		}
		seen++
		for _, field := range ts.Type.(*ast.StructType).Fields.List {
			doc := ""
			if field.Doc != nil {
				doc = strings.Join(strings.Fields(field.Doc.Text()), " ")
			}
			if !strings.Contains(doc, "P-07 proposal") || !sectionRef(doc) {
				t.Errorf("%s.%s has no \"P-07 proposal\" mark with a section", ts.Name.Name, field.Names[0].Name)
			}
		}
		return false
	})
	if seen != len(want) {
		t.Errorf("found %d of the %d proposed fact structs", seen, len(want))
	}
}

// sectionRef reports a section number such as 2.2.5 or 40.4.6 in s.
func sectionRef(s string) bool {
	for i := 0; i+2 < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' && s[i+1] == '.' && s[i+2] >= '0' && s[i+2] <= '9' {
			return true
		}
	}
	return false
}
