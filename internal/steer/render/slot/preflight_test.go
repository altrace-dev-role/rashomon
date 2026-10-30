package slot

import (
	"slices"
	"testing"
)

// bound binds v and returns its text, failing the test on a refusal.
func bound(t *testing.T, name string, v Value) string {
	t.Helper()
	b, err := Bind(v)
	if err != nil {
		t.Errorf("%s: Bind: %v", name, err)
		return ""
	}
	return b.Text()
}

// TestPF1s_PathSetSaysNoneAndLeavesSecretsOut: the preflight's path list may
// be empty, which reads "none" rather than refusing the note as PathList
// does; a secret-shaped path is left out and counted; past three it says how
// many more; the paths an earlier note of the session named are a count; a
// path of the wrong shape refuses the set.
func TestPF1s_PathSetSaysNoneAndLeavesSecretsOut(t *testing.T) {
	for _, c := range []struct {
		name string
		set  PathSet
		want string
	}{
		{"empty", NewPathSet(nil, 0, 0), "none"},
		{"one", NewPathSet([]Path{"internal/verify/stop.go"}, 0, 0), "'internal/verify/stop.go'"},
		{"four", NewPathSet([]Path{"a.go", "b.go", "c.go", "d.go"}, 0, 0), "'a.go', 'b.go', 'c.go' and 1 more"},
		{"unlisted", NewPathSet([]Path{"a.go"}, 2, 0), "'a.go' and 2 more"},
		{"secret only", NewPathSet([]Path{"AKIAIOSFODNN7EXAMPLE.txt"}, 0, 0), "1 left out"},
		{"earlier", NewPathSet([]Path{"a.go"}, 0, 2), "'a.go'; 2 given earlier"},
		{"earlier only", NewPathSet(nil, 0, 1), "1 given earlier"},
	} {
		if got := bound(t, c.name, c.set); got != c.want {
			t.Errorf("%s: Text() = %q, want %q", c.name, got, c.want)
		}
	}
	b, err := Bind(NewPathSet([]Path{"internal/a.go", "AKIAIOSFODNN7EXAMPLE.txt"}, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if set := b.(PathSet); !slices.Equal(set.Paths(), []Path{"internal/a.go"}) || set.More() != 1 {
		t.Errorf("bound set = %q and %d more, want the secret-shaped path counted, not kept", set.Paths(), set.More())
	}
	for _, bad := range []PathSet{NewPathSet([]Path{"../etc/passwd"}, 0, 0), NewPathSet([]Path{"/etc/passwd"}, 0, 0), NewPathSet(nil, -1, 0), NewPathSet(nil, 0, -1)} {
		if _, err := Bind(bad); err == nil {
			t.Errorf("Bind(%v) = nil error, want a refusal", bad.Paths())
		}
	}
}

// TestPF1s_RuleRefsQuoteTheIDAndFallBackToIt: a rule renders as its quoted
// id and trust, then its why in its own quote; a why Approved refuses drops
// to the id alone (R3); an id outside the spec's id charset is left out and
// counted; the rules an earlier note named, and those an untrusted spec
// holds, are numbers, never text from it.
func TestPF1s_RuleRefsQuoteTheIDAndFallBackToIt(t *testing.T) {
	trusted := NewEnum(EnumTrust, "trusted")
	managed := NewEnum(EnumTrust, "managed")
	for _, c := range []struct {
		name string
		refs RuleRefs
		want string
	}{
		{"empty", NewRuleRefs(nil, 0, 0, 0), "none"},
		{"id and why", NewRuleRefs([]RuleRef{{ID: "conv-errors", Trust: trusted, Why: "errors are wrapped with the package name"}}, 0, 0, 0),
			"'conv-errors' (trusted) 'errors are wrapped with the package name'"},
		{"why refused", NewRuleRefs([]RuleRef{{ID: "no-sys", Trust: managed, Why: "the system prompt is fixed"}}, 0, 0, 0), "'no-sys' (managed)"},
		{"bad id", NewRuleRefs([]RuleRef{{ID: "Bad ID", Trust: trusted}, {ID: "ok.rule_1", Trust: trusted}}, 0, 0, 0), "'ok.rule_1' (trusted) and 1 more"},
		{"four", NewRuleRefs([]RuleRef{{ID: "a", Trust: trusted}, {ID: "b", Trust: trusted}, {ID: "c", Trust: trusted}, {ID: "d", Trust: trusted}}, 0, 0, 0),
			"'a' (trusted); 'b' (trusted); 'c' (trusted) and 1 more"},
		{"withheld only", NewRuleRefs(nil, 0, 0, 2), "2 not shown, spec untrusted"},
		{"earlier only", NewRuleRefs(nil, 0, 3, 0), "3 given earlier"},
		{"all", NewRuleRefs([]RuleRef{{ID: "a", Trust: trusted}}, 1, 1, 1), "'a' (trusted) and 1 more; 1 given earlier; 1 not shown, spec untrusted"},
	} {
		if got := bound(t, c.name, c.refs); got != c.want {
			t.Errorf("%s: Text() = %q, want %q", c.name, got, c.want)
		}
	}
	for _, bad := range []RuleRefs{
		NewRuleRefs([]RuleRef{{ID: "a", Trust: NewEnum(EnumTrust, "untrusted")}}, 0, 0, 0),
		NewRuleRefs(nil, -1, 0, 0),
		NewRuleRefs(nil, 0, -1, 0),
		NewRuleRefs(nil, 0, 0, -1),
	} {
		if _, err := Bind(bad); err == nil {
			t.Errorf("Bind(%+v) = nil error, want a refusal", bad.Items())
		}
	}
}

// TestPF1s_GateRefsNameTheCommandAndTheLearnedCount: a gate renders as its
// quoted command words and trust; the families earlier sessions made
// required are a count; an argument-shaped gate is left out and counted.
func TestPF1s_GateRefsNameTheCommandAndTheLearnedCount(t *testing.T) {
	trusted := NewEnum(EnumTrust, "trusted")
	for _, c := range []struct {
		name  string
		gates GateRefs
		want  string
	}{
		{"empty", NewGateRefs(nil, 0, 0), "none"},
		{"no index", NewGateRefs(nil, 0, -1), "none"},
		{"one", NewGateRefs([]GateRef{{Words: "go test", Trust: trusted}}, 0, 0), "'go test' (trusted)"},
		{"learned", NewGateRefs([]GateRef{{Words: "go test", Trust: trusted}}, 0, 2), "'go test' (trusted); 2 learned from earlier sessions"},
		{"learned only", NewGateRefs(nil, 0, 1), "1 learned from earlier sessions"},
		{"argument", NewGateRefs([]GateRef{{Words: "go -race", Trust: trusted}}, 0, 0), "1 left out"},
	} {
		if got := bound(t, c.name, c.gates); got != c.want {
			t.Errorf("%s: Text() = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestPF1s_EnumListIsClosedAndSaysNone: an enum list takes only its table's
// values, each once, joined by "; " because a value may hold a comma.
func TestPF1s_EnumListIsClosedAndSaysNone(t *testing.T) {
	for _, c := range []struct {
		name string
		list EnumList
		want string
	}{
		{"empty", NewEnumList(EnumEvidenceKinds, nil), "none"},
		{"kinds", NewEnumList(EnumEvidenceKinds, []string{"judgment_disclosed", "mechanism_stated"}), "judgment_disclosed; mechanism_stated"},
		{"unknown", NewEnumList(EnumPreflightUnknown, []string{"no spec found", "request over 16 KiB, the rest not read"}),
			"no spec found; request over 16 KiB, the rest not read"},
	} {
		if got := bound(t, c.name, c.list); got != c.want {
			t.Errorf("%s: Text() = %q, want %q", c.name, got, c.want)
		}
	}
	for _, bad := range []EnumList{
		NewEnumList(EnumEvidenceKinds, []string{"unit_test_run"}),
		NewEnumList(EnumEvidenceKinds, []string{"cross_check", "cross_check"}),
		NewEnumList(EnumTrust, []string{"no spec found"}),
	} {
		if _, err := Bind(bad); err == nil {
			t.Errorf("Bind(%q) = nil error, want a refusal", bad.Values())
		}
	}
}

// TestPF1s_PreflightTablesHoldTheirValues pins the four preflight tables'
// literal values: trust words a note may print (never "untrusted"), the
// evidence kinds, where they came from, and the unknown phrases.
func TestPF1s_PreflightTablesHoldTheirValues(t *testing.T) {
	want := map[EnumTable][]string{
		EnumTrust:          {"trusted", "managed", "builtin"},
		EnumEvidenceKinds:  {"real_data_run", "hostile_input_run", "cross_check", "judgment_disclosed", "mechanism_stated"},
		EnumEvidenceSource: {"task file", "inferred from the request"},
		EnumPreflightUnknown: {
			"no spec found", "spec present, untrusted", "spec unreadable", "no gate history for this project",
			"symbols not matched, no platform graph log", "request over 16 KiB, the rest not read",
			"no named path exists in the project", "no go.mod, packages not counted",
			"package walk capped, count is a lower bound",
		},
	}
	for table, values := range want {
		if !slices.Equal(enumTables[table], values) {
			t.Errorf("table %d = %q, want %q", table, enumTables[table], values)
		}
	}
}
