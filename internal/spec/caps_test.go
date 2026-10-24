package spec

import (
	"fmt"
	"regexp/syntax"
	"strings"
	"testing"
)

// capCase is one rule of the limits table, with a document the rule accepts
// and one it refuses by exactly one. The refusing document's first error is
// asserted: its pointer, its reason and the limit it names.
type capCase struct {
	name, ok, bad, pointer string
	reason                 Reason
	limit                  int
}

func words(n int) string {
	return `[` + strings.Join(gen(n, func(i int) string { return fmt.Sprintf(`"w%d"`, i) }), ",") + `]`
}

// letters is n distinct all-letter words: A, AA, AAA, ...
func letters(n int) string {
	return `[` + strings.Join(gen(n, func(i int) string { return `"` + strings.Repeat("A", i+1) + `"` }), ",") + `]`
}

func gateIDs(n int) []string {
	return gen(n, func(i int) string { return fmt.Sprintf(`{"id":"g%d","kind":"test","argv":["go","test"]}`, i) })
}

func ciCmds(n int) string {
	return strings.Join(gen(n, func(int) string { return `["go","vet"]` }), ",")
}

func ruleIDs(n int) []string {
	return gen(n, func(i int) string {
		return fmt.Sprintf(`{"id":"r%d","kind":"protect_path","paths":["a"],"why":"w","hard_control":"h"}`, i)
	})
}

func factIDs(n int) []string {
	return gen(n, func(i int) string { return strings.Replace(validFact, `"id":"f"`, fmt.Sprintf(`"id":"f%d"`, i), 1) })
}

// sized pads a minimal document with trailing spaces to exactly n bytes.
func sized(n int) string {
	d := `{"rashomon_spec":1}`
	return d + strings.Repeat(" ", n-len(d))
}

func TestSP2_OverLimitRefused(t *testing.T) {
	gate := func(extra string) string { return withGates(gateWith(extra)) }
	rule := func(kind, extra string) string { return withRules(ruleWith(kind, extra)) }
	why := func(n int) string { return strings.Repeat("é", n) } // two bytes a rune: the cap counts runes
	cases := []capCase{
		{"file bytes", sized(MaxBytes), sized(MaxBytes + 1), "", ReasonTooLarge, MaxBytes},
		{"gates", withGates(gateIDs(MaxGates)...), withGates(gateIDs(MaxGates + 1)...), "/gates", ReasonTooMany, MaxGates},
		// ci_commands count against the same 32.
		{"gates plus ci_commands", doc(`"gates":[`+strings.Join(gateIDs(MaxGates-1), ",")+`]`, `"ci_commands":[`+ciCmds(1)+`]`),
			doc(`"gates":[`+strings.Join(gateIDs(MaxGates), ",")+`]`, `"ci_commands":[`+ciCmds(1)+`]`), "/ci_commands", ReasonTooMany, MaxGates},
		{"ci_commands alone", doc(`"ci_commands":[` + ciCmds(MaxGates) + `]`), doc(`"ci_commands":[` + ciCmds(MaxGates+1) + `]`), "/ci_commands", ReasonTooMany, MaxGates},
		{"rules", withRules(ruleIDs(MaxRules)...), withRules(ruleIDs(MaxRules + 1)...), "/rules", ReasonTooMany, MaxRules},
		{"facts", withFacts(factIDs(MaxFacts)...), withFacts(factIDs(MaxFacts + 1)...), "/facts", ReasonTooMany, MaxFacts},
		{"pattern bytes", rule("forbid_code", `"pattern":"`+strings.Repeat("a", MaxPatternBytes)+`"`), rule("forbid_code", `"pattern":"`+strings.Repeat("a", MaxPatternBytes+1)+`"`),
			"/rules/0/pattern", ReasonPatternTooLong, MaxPatternBytes},
		{"success_line bytes", gate(`"success_line":["` + strings.Repeat("a", MaxPatternBytes) + `"]`), gate(`"success_line":["` + strings.Repeat("a", MaxPatternBytes+1) + `"]`),
			"/gates/0/success_line/0", ReasonPatternTooLong, MaxPatternBytes},
		{"why runes", withRules(strings.Replace(ruleWith("forbid_code", validRule["forbid_code"]), `"w"`, `"`+why(MaxWhyRunes)+`"`, 1)),
			withRules(strings.Replace(ruleWith("forbid_code", validRule["forbid_code"]), `"w"`, `"`+why(MaxWhyRunes+1)+`"`, 1)), "/rules/0/why", ReasonTooLong, MaxWhyRunes},
		{"hard_control runes", withRules(strings.Replace(ruleWith("forbid_code", validRule["forbid_code"]), `"h"`, `"`+why(MaxWhyRunes)+`"`, 1)),
			withRules(strings.Replace(ruleWith("forbid_code", validRule["forbid_code"]), `"h"`, `"`+why(MaxWhyRunes+1)+`"`, 1)), "/rules/0/hard_control", ReasonTooLong, MaxWhyRunes},
		{"alternative runes", rule("forbid_code", validRule["forbid_code"]+`,"alternative":"`+why(MaxAlternativeRunes)+`"`),
			rule("forbid_code", validRule["forbid_code"]+`,"alternative":"`+why(MaxAlternativeRunes+1)+`"`), "/rules/0/alternative", ReasonTooLong, MaxAlternativeRunes},
		{"fact why runes", withFacts(strings.Replace(validFact, `"mechanism":"profile"`, `"mechanism":"profile","why":"`+why(MaxWhyRunes)+`"`, 1)),
			withFacts(strings.Replace(validFact, `"mechanism":"profile"`, `"mechanism":"profile","why":"`+why(MaxWhyRunes+1)+`"`, 1)), "/facts/0/why", ReasonTooLong, MaxWhyRunes},
		// The proposed caps the archive leaves unnamed.
		{"gate argv words", withGates(strings.Replace(gateWith(``), `["go","test"]`, words(MaxArgvWords), 1)),
			withGates(strings.Replace(gateWith(``), `["go","test"]`, words(MaxArgvWords+1), 1)), "/gates/0/argv", ReasonTooMany, MaxArgvWords},
		{"ci command words", doc(`"ci_commands":[` + words(MaxArgvWords) + `]`), doc(`"ci_commands":[` + words(MaxArgvWords+1) + `]`), "/ci_commands/0", ReasonTooMany, MaxArgvWords},
		{"alias words", gate(`"aliases":[` + words(MaxArgvWords) + `]`), gate(`"aliases":[` + words(MaxArgvWords+1) + `]`), "/gates/0/aliases/0", ReasonTooMany, MaxArgvWords},
		{"aliases", gate(`"aliases":[` + strings.Repeat(`["a"],`, MaxAliases-1) + `["a"]]`), gate(`"aliases":[` + strings.Repeat(`["a"],`, MaxAliases) + `["a"]]`), "/gates/0/aliases", ReasonTooMany, MaxAliases},
		{"applies_to items", gate(`"applies_to":` + words(MaxListItems)), gate(`"applies_to":` + words(MaxListItems+1)), "/gates/0/applies_to", ReasonTooMany, MaxListItems},
		{"partial_flags items", gate(`"partial_flags":` + strings.Replace(words(MaxListItems), `"w`, `"-w`, -1)), gate(`"partial_flags":` + strings.Replace(words(MaxListItems+1), `"w`, `"-w`, -1)), "/gates/0/partial_flags", ReasonTooMany, MaxListItems},
		{"failure_line items", gate(`"failure_line":` + words(MaxListItems)), gate(`"failure_line":` + words(MaxListItems+1)), "/gates/0/failure_line", ReasonTooMany, MaxListItems},
		{"programs items", withFacts(strings.Replace(validFact, `["aws"]`, words(MaxListItems), 1)), withFacts(strings.Replace(validFact, `["aws"]`, words(MaxListItems+1), 1)), "/facts/0/programs", ReasonTooMany, MaxListItems},
		{"classes items", withFacts(strings.Replace(validFact, `["cred.aws_profile_missing"]`, `[`+strings.Join(gen(MaxListItems, func(i int) string { return fmt.Sprintf(`"cred.c%d_missing"`, i) }), ",")+`]`, 1)),
			withFacts(strings.Replace(validFact, `["cred.aws_profile_missing"]`, `[`+strings.Join(gen(MaxListItems+1, func(i int) string { return fmt.Sprintf(`"cred.c%d_missing"`, i) }), ",")+`]`, 1)), "/facts/0/classes", ReasonTooMany, MaxListItems},
		{"evidence items", rule("require_evidence", `"at":["push"],"evidence":`+words(MaxListItems)), rule("require_evidence", `"at":["push"],"evidence":`+words(MaxListItems+1)), "/rules/0/evidence", ReasonTooMany, MaxListItems},
		{"paths items", rule("protect_path", `"paths":`+words(MaxListItems)), rule("protect_path", `"paths":`+words(MaxListItems+1)), "/rules/0/paths", ReasonTooMany, MaxListItems},
		{"exclude_paths items", rule("forbid_code", validRule["forbid_code"]+`,"exclude_paths":`+words(MaxListItems)), rule("forbid_code", validRule["forbid_code"]+`,"exclude_paths":`+words(MaxListItems+1)), "/rules/0/exclude_paths", ReasonTooMany, MaxListItems},
		{"languages items", rule("forbid_code", validRule["forbid_code"]+`,"languages":`+words(MaxListItems)), rule("forbid_code", validRule["forbid_code"]+`,"languages":`+words(MaxListItems+1)), "/rules/0/languages", ReasonTooMany, MaxListItems},
		{"invalidated_by items", rule("order", validRule["order"]+`,"invalidated_by":`+words(MaxListItems)), rule("order", validRule["order"]+`,"invalidated_by":`+words(MaxListItems+1)), "/rules/0/invalidated_by", ReasonTooMany, MaxListItems},
		{"same_run_key items", rule("order", validRule["order"]+`,"same_run_key":`+strings.Replace(words(MaxListItems), `"w`, `"-w`, -1)), rule("order", validRule["order"]+`,"same_run_key":`+strings.Replace(words(MaxListItems+1), `"w`, `"-w`, -1)), "/rules/0/same_run_key", ReasonTooMany, MaxListItems},
		{"flags items", rule("forbid_command", `"command":{"argv":["x"],"flags":`+strings.Replace(words(MaxListItems), `"w`, `"-w`, -1)+`}`), rule("forbid_command", `"command":{"argv":["x"],"flags":`+strings.Replace(words(MaxListItems+1), `"w`, `"-w`, -1)+`}`), "/rules/0/command/flags", ReasonTooMany, MaxListItems},
		{"require_flags items", rule("forbid_command", `"command":{"argv":["x"],"require_flags":`+strings.Replace(words(MaxListItems), `"w`, `"-w`, -1)+`}`), rule("forbid_command", `"command":{"argv":["x"],"require_flags":`+strings.Replace(words(MaxListItems+1), `"w`, `"-w`, -1)+`}`), "/rules/0/command/require_flags", ReasonTooMany, MaxListItems},
		{"sql items", rule("forbid_command", `"command":{"argv":["psql"],"sql":`+letters(MaxListItems)+`}`), rule("forbid_command", `"command":{"argv":["psql"],"sql":`+letters(MaxListItems+1)+`}`), "/rules/0/command/sql", ReasonTooMany, MaxListItems},
		{"flag_values values", rule("forbid_command", `"command":{"argv":["x"],"flag_values":{"-f":`+words(MaxListItems)+`}}`), rule("forbid_command", `"command":{"argv":["x"],"flag_values":{"-f":`+words(MaxListItems+1)+`}}`), "/rules/0/command/flag_values/-f", ReasonTooMany, MaxListItems},
		{"flag_values keys", rule("forbid_command", `"command":{"argv":["x"],"flag_values":{`+strings.Join(gen(MaxFlagValueKeys, func(i int) string { return fmt.Sprintf(`"-f%d":["v"]`, i) }), ",")+`}}`),
			rule("forbid_command", `"command":{"argv":["x"],"flag_values":{`+strings.Join(gen(MaxFlagValueKeys+1, func(i int) string { return fmt.Sprintf(`"-f%d":["v"]`, i) }), ",")+`}}`), "/rules/0/command/flag_values", ReasonTooMany, MaxFlagValueKeys},
		{"exit_codes", gate(`"exit_codes":{` + strings.Join(gen(MaxExitCodes, func(i int) string { return fmt.Sprintf(`"%d":"pass"`, i) }), ",") + `}`),
			gate(`"exit_codes":{` + strings.Join(gen(MaxExitCodes+1, func(i int) string { return fmt.Sprintf(`"%d":"pass"`, i) }), ",") + `}`), "/gates/0/exit_codes", ReasonTooMany, MaxExitCodes},
		{"word bytes", gate(`"applies_to":["` + strings.Repeat("a", MaxWordBytes) + `"]`), gate(`"applies_to":["` + strings.Repeat("a", MaxWordBytes+1) + `"]`), "/gates/0/applies_to/0", ReasonTooLong, MaxWordBytes},
		{"argv word bytes", withGates(strings.Replace(gateWith(``), `"go"`, `"`+strings.Repeat("a", MaxWordBytes)+`"`, 1)), withGates(strings.Replace(gateWith(``), `"go"`, `"`+strings.Repeat("a", MaxWordBytes+1)+`"`, 1)), "/gates/0/argv/0", ReasonTooLong, MaxWordBytes},
		{"dir bytes", gate(`"dir":"` + strings.Repeat("a", MaxWordBytes) + `"`), gate(`"dir":"` + strings.Repeat("a", MaxWordBytes+1) + `"`), "/gates/0/dir", ReasonTooLong, MaxWordBytes},
		{"fact value bytes", withFacts(strings.Replace(validFact, `"value":"dev"`, `"value":"`+strings.Repeat("a", MaxFactValueBytes)+`"`, 1)), withFacts(strings.Replace(validFact, `"value":"dev"`, `"value":"`+strings.Repeat("a", MaxFactValueBytes+1)+`"`, 1)), "/facts/0/value", ReasonTooLong, MaxFactValueBytes},
		{"target id bytes", withFacts(strings.Replace(validFact, `"value"`, `"target":{"kind":"aws_region","id":"`+strings.Repeat("a", MaxFactValueBytes)+`"},"value"`, 1)),
			withFacts(strings.Replace(validFact, `"value"`, `"target":{"kind":"aws_region","id":"`+strings.Repeat("a", MaxFactValueBytes+1)+`"},"value"`, 1)), "/facts/0/target/id", ReasonTooLong, MaxFactValueBytes},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			accept(t, tc.ok)
			e := refuse(t, tc.bad, tc.pointer, tc.reason)
			if e.Limit != tc.limit {
				t.Errorf("Limit = %d, want %d", e.Limit, tc.limit)
			}
		})
	}
}

// TestSP2_ClosedEnumsAndIDs: each closed list accepts every member and refuses
// a word outside it; ids are the archive's charset, unique within their list.
func TestSP2_ClosedEnumsAndIDs(t *testing.T) {
	type enum struct {
		name    string
		members []string
		build   func(v string) string
		pointer string
		empty   Reason // the reason for an empty value; ReasonEmpty unless the value is a suffix
	}
	gate := func(extra string) string { return withGates(gateWith(extra)) }
	enums := []enum{
		{"gate kind", GateKinds, func(v string) string { return withGates(strings.Replace(gateWith(``), `"test"`, `"`+v+`"`, 1)) }, "/gates/0/kind", ""},
		{"gate match", GateMatches, func(v string) string { return gate(`"match":"` + v + `"`) }, "/gates/0/match", ""},
		{"exit state", ExitStates, func(v string) string { return gate(`"exit_codes":{"1":"` + v + `"}`) }, "/gates/0/exit_codes/1", ""},
		{"rule mode", Modes, func(v string) string { return withRules(ruleWith("protect_path", `"paths":["a"],"modes":["`+v+`"]`)) }, "/rules/0/modes/0", ""},
		{"rule at", Points, func(v string) string {
			return withRules(ruleWith("require_evidence", `"at":["`+v+`"],"evidence":["u"]`))
		}, "/rules/0/at/0", ""},
		{"fact target kind", TargetKinds, func(v string) string {
			return withFacts(strings.Replace(validFact, `"value"`, `"target":{"kind":"`+v+`","id":"x"},"value"`, 1))
		}, "/facts/0/target/kind", ""},
		{"evidence kind", GateKinds, func(v string) string {
			return withRules(ruleWith("require_evidence", `"at":["push"],"evidence":["kind:`+v+`"]`))
		}, "/rules/0/evidence/0", ReasonEnum},
	}
	for _, e := range enums {
		t.Run(e.name, func(t *testing.T) {
			for _, m := range e.members {
				accept(t, e.build(m))
			}
			for _, bad := range []string{"deploy", "Test", "TEST", "", " test", "test "} {
				if bad == "" {
					want := e.empty
					if want == "" {
						want = ReasonEmpty
					}
					refuse(t, e.build(bad), e.pointer, want)
					continue
				}
				refuse(t, e.build(bad), e.pointer, ReasonEnum)
			}
		})
	}
	// The rule kind, every member and a word outside.
	for _, k := range RuleKinds {
		accept(t, withRules(ruleWith(string(k), validRule[string(k)])))
	}
	refuse(t, withRules(ruleWith("forbid_everything", `"pattern":"x"`)), "/rules/0/kind", ReasonEnum)
	// Each fact kind with its own mechanism: the pairs the validator reads.
	for i, k := range FactKinds {
		bad := strings.NewReplacer(`"aws_profile"`, `"`+k+`"`, `"profile"`, `"`+factMechanisms[(i+1)%len(factMechanisms)]+`"`).Replace(validFact)
		refuse(t, withFacts(bad), "/facts/0/mechanism", ReasonMechanism)
	}
	// The closed lists themselves, in the archive's order.
	for name, got := range map[string]string{
		"GateKinds": strings.Join(GateKinds, ","), "GateMatches": strings.Join(GateMatches, ","), "ExitStates": strings.Join(ExitStates, ","),
		"Modes": strings.Join(Modes, ","), "Points": strings.Join(Points, ","), "FactKinds": strings.Join(FactKinds, ","),
		"TargetKinds": strings.Join(TargetKinds, ","), "SQLPrograms": strings.Join(SQLPrograms, ","),
	} {
		want := map[string]string{
			"GateKinds": "test,build,lint,typecheck,scan,custom", "GateMatches": "prefix,exact", "ExitStates": "pass,empty_run,fail",
			"Modes": "modify,delete,create", "Points": "commit,push,pr,done,subtask",
			"FactKinds":   "aws_profile,aws_region,kube_context,kube_namespace,package_index_host,env_var_name,sign_in_step",
			"TargetKinds": "aws_account,aws_region,endpoint_host,kube_context,gcp_project,azure_subscription",
			"SQLPrograms": "psql,mysql,sqlite3,cqlsh,clickhouse-client",
		}[name]
		if got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
	// The version field: 1 only.
	accept(t, `{"rashomon_spec":1}`)
	for _, tc := range []struct {
		doc    string
		reason Reason
	}{
		{`{"rashomon_spec":0}`, ReasonVersion}, {`{"rashomon_spec":2}`, ReasonVersion}, {`{"rashomon_spec":-1}`, ReasonVersion},
		{`{}`, ReasonRequired}, {`{"rashomon_spec":"1"}`, ReasonType}, {`{"rashomon_spec":true}`, ReasonType}, {`{"rashomon_spec":null}`, ReasonType},
		// A version-2 file is refused as a version, not as its first unknown key.
		{`{"rashomon_spec":2,"tags":[]}`, ReasonVersion},
	} {
		refuse(t, tc.doc, "/rashomon_spec", tc.reason)
	}
}

func TestSP2_IDCharsetAndUniqueness(t *testing.T) {
	build := map[string]func(id string) string{
		"/gates/0/id": func(id string) string { return withGates(strings.Replace(gateWith(``), `"g"`, `"`+id+`"`, 1)) },
		"/rules/0/id": func(id string) string {
			return withRules(strings.Replace(ruleWith("forbid_code", validRule["forbid_code"]), `"r"`, `"`+id+`"`, 1))
		},
		"/facts/0/id": func(id string) string { return withFacts(strings.Replace(validFact, `"f"`, `"`+id+`"`, 1)) },
	}
	for pointer, b := range build {
		t.Run(pointer, func(t *testing.T) {
			for _, ok := range []string{"a", "0", "a-b", "a.b", "a_b", "9z", strings.Repeat("a", MaxIDBytes), "a" + strings.Repeat("-", MaxIDBytes-1)} {
				accept(t, b(ok))
			}
			for _, bad := range []string{"A", "aB", "-a", ".a", "_a", "a b", "a/b", "a:b", "é", "a\\u00e9", strings.Repeat("a", MaxIDBytes+1)} {
				refuse(t, b(bad), pointer, ReasonID)
			}
			refuse(t, b(""), pointer, ReasonEmpty)
		})
	}
	// Ids are unique within their own list, and a gate may share an id with a
	// rule: they are different namespaces (a gate is spec:<id>).
	two := func(a, b string) string { return strings.Replace(a, `"g"`, `"`+b+`"`, 1) }
	refuse(t, withGates(gateWith(``), gateWith(``)), "/gates/1/id", ReasonDuplicateID)
	accept(t, withGates(gateWith(``), two(gateWith(``), "h")))
	r := ruleWith("forbid_code", validRule["forbid_code"])
	refuse(t, withRules(r, r), "/rules/1/id", ReasonDuplicateID)
	refuse(t, withFacts(validFact, validFact), "/facts/1/id", ReasonDuplicateID)
	accept(t, doc(`"gates":[`+gateWith(``)+`]`, `"rules":[`+strings.Replace(r, `"r"`, `"g"`, 1)+`]`))
}

// TestSP5_RegexCapsCountInstructions: a pattern at the instruction cap passes,
// one over fails; a repeat count over 100 and a counted repeat inside a
// counted repeat fail; the 20,000 total is over the whole spec.
//
// Every expected count is derived from how regexp/syntax compiles a
// concatenation of literal repeats: one fail instruction first, one match
// last, and one rune instruction per literal. "a{100}" is a hundred of
// them, so letters a to s at {100} plus t{98} is 1 + 1900 + 98 + 1 = 2000.
func TestSP5_RegexCapsCountInstructions(t *testing.T) {
	atCap := func(last int) string {
		var b strings.Builder
		for c := 'a'; c <= 's'; c++ {
			fmt.Fprintf(&b, "%c{100}", c)
		}
		fmt.Fprintf(&b, "t{%d}", last)
		return b.String()
	}
	if got, ok := countInstructions(t, atCap(98)); !ok || got != MaxPatternInstructions {
		t.Fatalf("fixture is %d instructions, want exactly %d", got, MaxPatternInstructions)
	}
	rule := func(p string) string { return withRules(ruleWith("forbid_code", `"pattern":"`+p+`"`)) }
	for _, tc := range []struct {
		name, ok, bad, pointer string
		reason                 Reason
	}{
		{"instructions per pattern", rule(atCap(98)), rule(atCap(99)), "/rules/0/pattern", ReasonPatternComplex},
		{"repeat count", rule("a{100}"), rule("a{101}"), "/rules/0/pattern", ReasonRepeatTooLarge},
		{"repeat range maximum", rule("a{2,100}"), rule("a{2,101}"), "/rules/0/pattern", ReasonRepeatTooLarge},
		{"repeat minimum with no maximum", rule("a{100,}"), rule("a{101,}"), "/rules/0/pattern", ReasonRepeatTooLarge},
		// Go's own parser stops at 1000; that refusal is still a repeat refusal.
		{"repeat past Go's ceiling", rule("a{100}"), rule("a{1001}"), "/rules/0/pattern", ReasonRepeatTooLarge},
		{"nested counted repeat", rule("(?:a*){5}"), rule("(?:a{2}){3}"), "/rules/0/pattern", ReasonNestedRepeat},
		{"nested counted repeat, deep", rule("(?:a{2}b)*"), rule("(?:x(?:y|(?:a{2}b)){2})"), "/rules/0/pattern", ReasonNestedRepeat},
		{"Go's own nested bomb", rule("(?:a{2})*"), rule(`(?:\\w{1000}){1000}`), "/rules/0/pattern", ReasonRepeatTooLarge},
		{"not a regular expression", rule("a(b)"), rule("a(b"), "/rules/0/pattern", ReasonPatternSyntax},
		{"backreference (not RE2)", rule(`(a)b`), rule(`(a)\\1`), "/rules/0/pattern", ReasonPatternSyntax},
		{"lookahead (not RE2)", rule(`a`), rule(`a(?=b)`), "/rules/0/pattern", ReasonPatternSyntax},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accept(t, tc.ok)
			refuse(t, tc.bad, tc.pointer, tc.reason)
		})
	}
	// The same caps hold for every field that carries a regular expression.
	for _, f := range []string{"success_line", "failure_line", "empty_run_line"} {
		t.Run(f, func(t *testing.T) {
			g := func(p string) string { return withGates(gateWith(`"` + f + `":["ok","` + p + `"]`)) }
			accept(t, g(atCap(98)))
			refuse(t, g(atCap(99)), "/gates/0/"+f+"/1", ReasonPatternComplex)
			refuse(t, g("a{101}"), "/gates/0/"+f+"/1", ReasonRepeatTooLarge)
			refuse(t, g("(?:a{2}){3}"), "/gates/0/"+f+"/1", ReasonNestedRepeat)
			refuse(t, g("a(b"), "/gates/0/"+f+"/1", ReasonPatternSyntax)
		})
	}
	// 20,000 across the spec: ten patterns of exactly 2,000 pass; an
	// eleventh, of three instructions ("a": fail, rune, match), is the one
	// that crosses, wherever it sits.
	ten := gen(10, func(i int) string {
		return fmt.Sprintf(`{"id":"r%d","kind":"forbid_code","pattern":"%s","why":"w","hard_control":"h"}`, i, atCap(98))
	})
	accept(t, withRules(ten...))
	eleven := fmt.Sprintf(`{"id":"r10","kind":"forbid_code","pattern":"a","why":"w","hard_control":"h"}`)
	refuse(t, withRules(append(ten, eleven)...), "/rules/10/pattern", ReasonSpecComplex)
	// Gate line patterns count in the same total, and a gate comes first.
	gateLine := `{"id":"g","kind":"test","argv":["go"],"success_line":["a"]}`
	refuse(t, doc(`"gates":[`+gateLine+`]`, `"rules":[`+strings.Join(ten, ",")+`]`), "/rules/9/pattern", ReasonSpecComplex)
	// The limit is reported.
	if e := refuse(t, withRules(append(ten, eleven)...), "/rules/10/pattern", ReasonSpecComplex); e.Limit != MaxSpecInstructions {
		t.Errorf("Limit = %d, want %d", e.Limit, MaxSpecInstructions)
	}
	// The source cap is checked before the pattern is parsed: a 513-byte
	// pattern that is also a syntax error is a length error.
	refuse(t, rule(strings.Repeat("(", MaxPatternBytes+1)), "/rules/0/pattern", ReasonPatternTooLong)
	// An empty pattern would match every edit.
	refuse(t, rule(""), "/rules/0/pattern", ReasonEmpty)
}

// countInstructions compiles src the way regexp does, straight from the
// standard library and not through Parse, and returns the instruction count:
// the check that the fixture is the 2,000 its comment derives.
func countInstructions(t *testing.T, src string) (int, bool) {
	t.Helper()
	re, err := syntax.Parse(src, syntax.Perl)
	if err != nil {
		return 0, false
	}
	prog, err := syntax.Compile(re.Simplify())
	if err != nil {
		return 0, false
	}
	return len(prog.Inst), true
}
