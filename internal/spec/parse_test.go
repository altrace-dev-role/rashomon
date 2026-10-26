package spec

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Fixtures. Each is built from a base object with the one key a row changes,
// so a row reads as the difference from a spec that is accepted.

type kv struct{ k, v string }

// obj renders base with over applied: a key already in base is replaced in
// place, a new key is appended, and a value "-" removes the key.
func obj(base []kv, over ...kv) string {
	out := append([]kv(nil), base...)
	for _, o := range over {
		found := false
		for i := range out {
			if out[i].k == o.k {
				out[i].v, found = o.v, true
			}
		}
		if !found {
			out = append(out, o)
		}
	}
	parts := []string{}
	for _, p := range out {
		if p.v != "-" {
			parts = append(parts, fmt.Sprintf("%q: %s", p.k, p.v))
		}
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func q(s string) string { b, _ := json.Marshal(s); return string(b) }

var (
	gateBase = []kv{{"id", `"unit"`}, {"kind", `"test"`}, {"argv", `["go", "test"]`}}
	ruleBase = []kv{{"id", `"r1"`}, {"kind", `"forbid_code"`}, {"why", `"w"`}, {"hard_control", `"h"`}}
	factBase = []kv{{"id", `"f1"`}, {"kind", `"aws_profile"`}, {"programs", `["aws"]`},
		{"classes", `["cred.sso_missing"]`}, {"value", `"dev"`}, {"mechanism", `"profile"`}}
)

func gate(over ...kv) string { return obj(gateBase, over...) }
func rule(over ...kv) string { return obj(ruleBase, over...) }
func fact(over ...kv) string { return obj(factBase, over...) }

func top(body string) string {
	if body == "" {
		return `{"rashomon_spec": 1}`
	}
	return `{"rashomon_spec": 1, ` + body + `}`
}
func withGate(over ...kv) string { return top(`"gates": [` + gate(over...) + `]`) }
func withRule(over ...kv) string { return top(`"rules": [` + rule(over...) + `]`) }
func withFact(over ...kv) string { return top(`"facts": [` + fact(over...) + `]`) }

// many repeats item n times, comma separated, with the index available.
func many(n int, item func(i int) string) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = item(i)
	}
	return strings.Join(parts, ", ")
}

// anyPointer marks a row whose pointer the rule leaves open (a JSON syntax
// error has no schema position worth pinning).
const anyPointer = "<any>"

type row struct {
	name   string
	in     string
	reason Reason // "" accepted
	ptr    string
	limit  int
}

func ok(name, in string) row { return row{name: name, in: in} }
func bad(name, in string, r Reason, ptr string) row {
	return row{name: name, in: in, reason: r, ptr: ptr}
}
func over(name, in string, r Reason, ptr string, limit int) row {
	return row{name: name, in: in, reason: r, ptr: ptr, limit: limit}
}

func runRows(t *testing.T, rows []row) {
	t.Helper()
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			f, digest, err := Parse([]byte(r.in))
			if r.reason == "" {
				if err != nil {
					t.Fatalf("refused, want accepted: %v", err)
				}
				if f == nil || !strings.HasPrefix(digest, DigestPrefix) {
					t.Fatalf("accepted with file %v and digest %q", f, digest)
				}
				return
			}
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("got err %v, want a *Error with reason %s", err, r.reason)
			}
			if f != nil {
				t.Errorf("an invalid spec returned a File: it must apply nothing")
			}
			if e.Reason != r.reason {
				t.Errorf("reason %s, want %s (%v)", e.Reason, r.reason, e)
			}
			if r.ptr != anyPointer && e.Pointer != r.ptr {
				t.Errorf("pointer %q, want %q", e.Pointer, r.ptr)
			}
			if e.Limit != r.limit {
				t.Errorf("limit %d, want %d", e.Limit, r.limit)
			}
		})
	}
}

// A pattern the compiler counts at exactly 2000 instructions: nineteen a{100}
// are 1,900, b{98} is 98, and the program has two more (a fail and a match).
// Measured as len(prog.Inst) of syntax.Compile(re.Simplify()) when the test was
// written: 2000 for pat2000 and 2001 for pat2001, whose last repeat is b{99}.
var (
	pat2000 = strings.Repeat("a{100}", 19) + "b{98}"
	pat2001 = strings.Repeat("a{100}", 19) + "b{99}"
)

// Patterns of 1997 and 1998 instructions: nineteen a{100}, then b{95} or b{96}
// (1,900 + 95 + 2 and 1,900 + 96 + 2), and "a" is 3 (a fail, a rune, a match).
// With nine of pat2000 they total 20,000 and 20,001 with the "a".
var (
	pat1997 = strings.Repeat("a{100}", 19) + "b{95}"
	pat1998 = strings.Repeat("a{100}", 19) + "b{96}"
)

func mixedRules(pats ...string) string {
	return top(`"rules": [` + many(len(pats), func(i int) string {
		return rule(kv{"id", q(fmt.Sprintf("r%d", i))}, kv{"pattern", q(pats[i])})
	}) + `]`)
}

func rulesOfPatterns(n int, pat string) string {
	return top(`"rules": [` + many(n, func(i int) string {
		return rule(kv{"id", q(fmt.Sprintf("r%d", i))}, kv{"pattern", q(pat)})
	}) + `]`)
}

func sized(total int) string {
	base := top("")
	return base[:len(base)-1] + strings.Repeat(" ", total-len(base)) + "}"
}

// TestVER37_SpecStrictSchema: every rule of the schema (archive 20.2.8), with
// a value it accepts and a value it refuses, and the over-cap path of every
// cap. Expected values come from the stated rule, not from the parser.
func TestVER37_SpecStrictSchema(t *testing.T) {
	var rows []row
	add := func(r ...row) { rows = append(rows, r...) }

	// Version, shape of the file, unknown keys at every level.
	add(
		ok("minimal file", top("")),
		bad("version missing", `{}`, ReasonRequired, "/rashomon_spec"),
		bad("version 2", `{"rashomon_spec": 2}`, ReasonVersion, "/rashomon_spec"),
		bad("version 0", `{"rashomon_spec": 0}`, ReasonVersion, "/rashomon_spec"),
		bad("version as string", `{"rashomon_spec": "1"}`, ReasonType, "/rashomon_spec"),
		bad("version as float", `{"rashomon_spec": 1.0}`, ReasonNotInteger, "/rashomon_spec"),
		bad("version with exponent", `{"rashomon_spec": 1e0}`, ReasonNotInteger, "/rashomon_spec"),
		bad("version beyond 2^53-1", `{"rashomon_spec": 9007199254740992}`, ReasonIntegerRange, "/rashomon_spec"),
		bad("version below -(2^53-1)", `{"rashomon_spec": -9007199254740992}`, ReasonIntegerRange, "/rashomon_spec"),
		bad("version at -(2^53-1) reads as an integer and is not 1", `{"rashomon_spec": -9007199254740991}`, ReasonVersion, "/rashomon_spec"),
		bad("trailing garbage", `{"rashomon_spec": 1} x`, ReasonSyntax, anyPointer),
		bad("a key of every kept character is shown", top(`"a.b_c-D9": 1`), ReasonUnknownKey, "/a.b_c-D9"),
		bad("version beyond int64", `{"rashomon_spec": 9223372036854775808}`, ReasonIntegerRange, "/rashomon_spec"),
		bad("version null", `{"rashomon_spec": null}`, ReasonType, "/rashomon_spec"),
		bad("file is an array", `[]`, ReasonType, ""),
		bad("file is null", `null`, ReasonType, ""),
		bad("file is empty", ``, ReasonSyntax, anyPointer),
		bad("two values", `{"rashomon_spec": 1} {}`, ReasonSyntax, anyPointer),
		bad("trailing comma", `{"rashomon_spec": 1,}`, ReasonSyntax, anyPointer),
		bad("duplicate key", `{"rashomon_spec": 1, "rashomon_spec": 1}`, ReasonDuplicateKey, "/rashomon_spec"),
		bad("unknown key at the top", top(`"extra": 1`), ReasonUnknownKey, "/extra"),
		bad("key in the wrong case", top(`"Gates": []`), ReasonUnknownKey, "/Gates"),
		bad("version beats unknown key", `{"rashomon_spec": 2, "extra": 1}`, ReasonVersion, "/rashomon_spec"),
		bad("unknown key in a gate", withGate(kv{"colour", `1`}), ReasonUnknownKey, "/gates/0/colour"),
		bad("unknown key in a rule", withRule(kv{"colour", `1`}), ReasonUnknownKey, "/rules/0/colour"),
		bad("unknown key in a fact", withFact(kv{"colour", `1`}), ReasonUnknownKey, "/facts/0/colour"),
		bad("unknown key in a fact target", withFact(kv{"target", `{"kind": "aws_account", "id": "x", "extra": 1}`}), ReasonUnknownKey, "/facts/0/target/extra"),
		bad("unknown key in a command", withRule(kv{"command", `{"argv": ["x"], "extra": 1}`}), ReasonUnknownKey, "/rules/0/command/extra"),
		bad("unknown key in before", withRule(kv{"before", `{"argv": ["x"], "extra": 1}`}), ReasonUnknownKey, "/rules/0/before/extra"),
		bad("unknown key in then", withRule(kv{"then", `{"argv": ["x"], "extra": 1}`}), ReasonUnknownKey, "/rules/0/then/extra"),
		bad("unknown key in after", withRule(kv{"after", `{"argv": ["x"], "extra": 1}`}), ReasonUnknownKey, "/rules/0/after/extra"),
		bad("gate key in the wrong case", withGate(kv{"ID", `"x"`}), ReasonUnknownKey, "/gates/0/ID"),
		bad("hostile key with spaces is not echoed", top(`"a b": 1`), ReasonUnknownKey, "/?"),
		bad("hostile long key is not echoed", top(`"`+strings.Repeat("k", 49)+`": 1`), ReasonUnknownKey, "/?"),
		bad("a 48-byte key is shown", top(`"`+strings.Repeat("k", 48)+`": 1`), ReasonUnknownKey, "/"+strings.Repeat("k", 48)),
	)

	// Caps on counts. Gates and ci_commands share MaxGates.
	gates := func(n int) string {
		return many(n, func(i int) string { return gate(kv{"id", q(fmt.Sprintf("g%d", i))}) })
	}
	cis := func(n int) string { return many(n, func(int) string { return `["go", "vet"]` }) }
	add(
		ok("32 gates", top(`"gates": [`+gates(32)+`]`)),
		over("33 gates", top(`"gates": [`+gates(33)+`]`), ReasonTooMany, "/gates", MaxGates),
		ok("31 gates and 1 ci command", top(`"gates": [`+gates(31)+`], "ci_commands": [`+cis(1)+`]`)),
		over("32 gates and 1 ci command", top(`"gates": [`+gates(32)+`], "ci_commands": [`+cis(1)+`]`), ReasonTooMany, "/ci_commands", MaxGates),
		ok("32 ci commands", top(`"ci_commands": [`+cis(32)+`]`)),
		over("33 ci commands", top(`"ci_commands": [`+cis(33)+`]`), ReasonTooMany, "/ci_commands", MaxGates),
		ok("64 rules", top(`"rules": [`+many(64, func(i int) string { return rule(kv{"id", q(fmt.Sprintf("r%d", i))}) })+`]`)),
		over("65 rules", top(`"rules": [`+many(65, func(i int) string { return rule(kv{"id", q(fmt.Sprintf("r%d", i))}) })+`]`), ReasonTooMany, "/rules", MaxRules),
		ok("64 facts", top(`"facts": [`+many(64, func(i int) string { return fact(kv{"id", q(fmt.Sprintf("f%d", i))}) })+`]`)),
		over("65 facts", top(`"facts": [`+many(65, func(i int) string { return fact(kv{"id", q(fmt.Sprintf("f%d", i))}) })+`]`), ReasonTooMany, "/facts", MaxFacts),
		over("a count is refused before the items", top(`"rules": [`+many(65, func(i int) string { return rule(kv{"id", `"BAD"`}) })+`]`), ReasonTooMany, "/rules", MaxRules),
		ok("file of exactly 256 KiB", sized(MaxBytes)),
		over("file of 256 KiB and one byte", sized(MaxBytes+1), ReasonTooLarge, "", MaxBytes),
		// The same boundary as a literal, so a cap moved in spec.go is seen.
		ok("file of exactly 262144 bytes", sized(262144)),
		over("file of 262145 bytes", sized(262145), ReasonTooLarge, "", 262144),
	)

	// Ids, for each of the three lists.
	long48, long49 := "a"+strings.Repeat("b", 47), "a"+strings.Repeat("b", 48)
	for _, k := range []struct {
		list string
		make func(id string) string
	}{
		{"gates", func(id string) string { return withGate(kv{"id", id}) }},
		{"rules", func(id string) string { return withRule(kv{"id", id}) }},
		{"facts", func(id string) string { return withFact(kv{"id", id}) }},
	} {
		ptr := "/" + k.list + "/0/id"
		for _, good := range []string{"a", "0", "a.b_c-d", "9z", long48} {
			add(ok(k.list+" id "+good[:min(len(good), 8)], k.make(q(good))))
		}
		for _, id := range []string{long49, "", "A", ".a", "_a", "-a", "a b", "a/b", "é", "a:b"} {
			add(bad(k.list+" id refused "+fmt.Sprintf("%.8q", id), k.make(q(id)), ReasonID, ptr))
		}
		add(bad(k.list+" id null", k.make(`null`), ReasonType, ptr),
			bad(k.list+" id number", k.make(`7`), ReasonType, ptr),
			bad(k.list+" id missing", k.make(`-`), ReasonRequired, ptr))
	}
	add(
		bad("two gates, one id", top(`"gates": [`+gate()+`, `+gate()+`]`), ReasonDuplicateID, "/gates/1/id"),
		bad("two rules, one id", top(`"rules": [`+rule()+`, `+rule()+`]`), ReasonDuplicateID, "/rules/1/id"),
		bad("two facts, one id", top(`"facts": [`+fact()+`, `+fact()+`]`), ReasonDuplicateID, "/facts/1/id"),
		ok("a gate and a rule may share an id", top(`"gates": [`+gate(kv{"id", `"same"`})+`], "rules": [`+rule(kv{"id", `"same"`})+`]`)),
		ok("after_gate is an id", withRule(kv{"after_gate", `"verify"`})),
		bad("after_gate is not an id", withRule(kv{"after_gate", `"Verify"`}), ReasonID, "/rules/0/after_gate"),
		ok("evidence: gate id and kind", withRule(kv{"evidence", `["unit", "kind:scan"]`})),
		bad("evidence: unknown kind", withRule(kv{"evidence", `["kind:bogus"]`}), ReasonEnum, "/rules/0/evidence/0"),
		bad("evidence: not an id", withRule(kv{"evidence", `["Unit"]`}), ReasonID, "/rules/0/evidence/0"),
	)

	// Rune caps. A rune is not a byte: 300 two-byte runes are accepted.
	r300, r301 := strings.Repeat("x", 300), strings.Repeat("x", 301)
	e300, e301 := strings.Repeat("é", 300), strings.Repeat("é", 301)
	p300 := strings.Repeat("\U0001F600", 300)
	a160, a161 := strings.Repeat("x", 160), strings.Repeat("x", 161)
	add(
		ok("why of 300 runes", withRule(kv{"why", q(r300)})),
		ok("why of 300 two-byte runes", withRule(kv{"why", q(e300)})),
		ok("why of 300 four-byte runes", withRule(kv{"why", q(p300)})),
		over("why of 301 runes", withRule(kv{"why", q(r301)}), ReasonTooLong, "/rules/0/why", MaxWhyRunes),
		over("why of 301 two-byte runes", withRule(kv{"why", q(e301)}), ReasonTooLong, "/rules/0/why", MaxWhyRunes),
		ok("hard_control of 300 runes", withRule(kv{"hard_control", q(r300)})),
		over("hard_control of 301 runes", withRule(kv{"hard_control", q(r301)}), ReasonTooLong, "/rules/0/hard_control", MaxWhyRunes),
		ok("alternative of 160 runes", withRule(kv{"alternative", q(a160)})),
		over("alternative of 161 runes", withRule(kv{"alternative", q(a161)}), ReasonTooLong, "/rules/0/alternative", MaxAlternativeRunes),
		ok("fact why of 300 runes", withFact(kv{"why", q(r300)})),
		over("fact why of 301 runes", withFact(kv{"why", q(r301)}), ReasonTooLong, "/facts/0/why", MaxWhyRunes),
		bad("why missing", withRule(kv{"why", "-"}), ReasonRequired, "/rules/0/why"),
		bad("hard_control missing", withRule(kv{"hard_control", "-"}), ReasonRequired, "/rules/0/hard_control"),
		bad("why empty", withRule(kv{"why", `""`}), ReasonEmpty, "/rules/0/why"),
		bad("hard_control empty", withRule(kv{"hard_control", `""`}), ReasonEmpty, "/rules/0/hard_control"),
		ok("U+2028 is text, not a control", withRule(kv{"why", `"a b"`})),
		bad("why with a NUL escape", withRule(kv{"why", `"a\u0000b"`}), ReasonControl, "/rules/0/why"),
		bad("why with a newline escape", withRule(kv{"why", `"a\nb"`}), ReasonControl, "/rules/0/why"),
		bad("why with a tab escape", withRule(kv{"why", `"a\tb"`}), ReasonControl, "/rules/0/why"),
		bad("why with DEL", withRule(kv{"why", `"a\u007fb"`}), ReasonControl, "/rules/0/why"),
		bad("why with a C1 control", withRule(kv{"why", `"a\u0085b"`}), ReasonControl, "/rules/0/why"),
		bad("why as a number", withRule(kv{"why", `1`}), ReasonType, "/rules/0/why"),
	)

	// Closed lists: one accepted value per member, one refused.
	for _, v := range []string{"test", "build", "lint", "typecheck", "scan", "custom"} {
		add(ok("gate kind "+v, withGate(kv{"kind", q(v)})))
	}
	for _, v := range []string{"forbid_code", "forbid_command", "protect_path", "require_evidence", "order"} {
		add(ok("rule kind "+v, withRule(kv{"kind", q(v)})))
	}
	for _, v := range []string{"aws_profile", "aws_region", "kube_context", "kube_namespace", "package_index_host", "env_var_name", "sign_in_step"} {
		add(ok("fact kind "+v, withFact(kv{"kind", q(v)})))
	}
	for _, v := range []string{"profile", "region", "context", "namespace", "index", "env var name", "sign-in step"} {
		add(ok("mechanism "+v, withFact(kv{"mechanism", q(v)})))
	}
	for _, v := range []string{"aws_account", "aws_region", "endpoint_host", "kube_context", "gcp_project", "azure_subscription"} {
		add(ok("target kind "+v, withFact(kv{"target", `{"kind": ` + q(v) + `, "id": "x"}`})))
	}
	add(
		bad("gate kind a number", withGate(kv{"kind", `7`}), ReasonType, "/gates/0/kind"),
		bad("match a number", withGate(kv{"match", `7`}), ReasonType, "/gates/0/match"),
		bad("mode a number", withRule(kv{"modes", `[7]`}), ReasonType, "/rules/0/modes/0"),
		bad("gate kind unknown", withGate(kv{"kind", `"deploy"`}), ReasonEnum, "/gates/0/kind"),
		bad("gate kind in the wrong case", withGate(kv{"kind", `"Test"`}), ReasonEnum, "/gates/0/kind"),
		bad("gate kind missing", withGate(kv{"kind", "-"}), ReasonRequired, "/gates/0/kind"),
		bad("rule kind unknown", withRule(kv{"kind", `"forbid_file"`}), ReasonEnum, "/rules/0/kind"),
		bad("fact kind unknown", withFact(kv{"kind", `"aws_secret"`}), ReasonEnum, "/facts/0/kind"),
		bad("mechanism unknown", withFact(kv{"mechanism", `"token"`}), ReasonEnum, "/facts/0/mechanism"),
		bad("target kind unknown", withFact(kv{"target", `{"kind": "aws_user", "id": "x"}`}), ReasonEnum, "/facts/0/target/kind"),
		bad("target without id", withFact(kv{"target", `{"kind": "aws_account"}`}), ReasonRequired, "/facts/0/target/id"),
		ok("match prefix", withGate(kv{"match", `"prefix"`})),
		ok("match exact", withGate(kv{"match", `"exact"`})),
		bad("match unknown", withGate(kv{"match", `"glob"`}), ReasonEnum, "/gates/0/match"),
		bad("match empty", withGate(kv{"match", `""`}), ReasonEnum, "/gates/0/match"),
		ok("modes all three", withRule(kv{"kind", `"protect_path"`}, kv{"modes", `["modify", "delete", "create"]`})),
		bad("mode unknown", withRule(kv{"modes", `["chmod"]`}), ReasonEnum, "/rules/0/modes/0"),
		bad("second mode unknown", withRule(kv{"modes", `["modify", "chmod"]`}), ReasonEnum, "/rules/0/modes/1"),
		ok("at each point", withRule(kv{"at", `["commit", "push", "pr", "done", "subtask"]`})),
		bad("at unknown", withRule(kv{"at", `["merge"]`}), ReasonEnum, "/rules/0/at/0"),
	)

	// Types and required members of the nested objects.
	add(
		bad("gates is an object", top(`"gates": {}`), ReasonType, "/gates"),
		bad("a gate is a string", top(`"gates": ["x"]`), ReasonType, "/gates/0"),
		ok("an empty gates list", top(`"gates": []`)),
		bad("an empty list inside a rule says nothing", withRule(kv{"modes", `[]`}), ReasonEmpty, "/rules/0/modes"),
		bad("an empty map inside a gate says nothing", withGate(kv{"exit_codes", `{}`}), ReasonEmpty, "/gates/0/exit_codes"),
		bad("argv missing", withGate(kv{"argv", "-"}), ReasonRequired, "/gates/0/argv"),
		bad("argv empty", withGate(kv{"argv", `[]`}), ReasonEmpty, "/gates/0/argv"),
		bad("argv word empty", withGate(kv{"argv", `["go", ""]`}), ReasonEmpty, "/gates/0/argv/1"),
		bad("argv word a number", withGate(kv{"argv", `["go", 1]`}), ReasonType, "/gates/0/argv/1"),
		bad("ci command empty", top(`"ci_commands": [[]]`), ReasonEmpty, "/ci_commands/0"),
		bad("ci command a string", top(`"ci_commands": ["go vet"]`), ReasonType, "/ci_commands/0"),
		ok("aliases", withGate(kv{"aliases", `[["make", "verify"]]`})),
		bad("alias a string", withGate(kv{"aliases", `["make verify"]`}), ReasonType, "/gates/0/aliases/0"),
		ok("exit codes", withGate(kv{"exit_codes", `{"0": "pass", "5": "empty_run"}`})),
		bad("exit code value a number", withGate(kv{"exit_codes", `{"0": 1}`}), ReasonType, "/gates/0/exit_codes/0"),
		ok("required false", withGate(kv{"required", `false`})),
		bad("required a string", withGate(kv{"required", `"yes"`}), ReasonType, "/gates/0/required"),
		bad("required null", withGate(kv{"required", `null`}), ReasonType, "/gates/0/required"),
		ok("command", withRule(kv{"command", `{"argv": ["cqlsh"], "sql": ["TRUNCATE"], "flag_values": {"--mode": ["a"]}}`})),
		bad("command without argv", withRule(kv{"command", `{"sql": ["X"]}`}), ReasonRequired, "/rules/0/command/argv"),
		bad("flag_values value a string", withRule(kv{"command", `{"argv": ["x"], "flag_values": {"--m": "a"}}`}), ReasonType, "/rules/0/command/flag_values/--m"),
		// Archive 20.2.8, Facts: identity and target kinds fire on authentication
		// classes only; sign_in_step and env_var_name are not so restricted.
		ok("class cred.*_missing", withFact(kv{"classes", `["cred.aws_profile_missing"]`})),
		ok("class *_profile_missing", withFact(kv{"classes", `["aws.sso_profile_missing"]`})),
		ok("class *_expired", withFact(kv{"classes", `["cred.aws_token_expired"]`})),
		ok("class *_sso_needs_human", withFact(kv{"classes", `["cred.aws_sso_needs_human"]`})),
		ok("class cred.registry", withFact(kv{"classes", `["cred.registry"]`})),
		ok("class cred.kube_context", withFact(kv{"classes", `["cred.kube_context"]`})),
		ok("class mcp.auth", withFact(kv{"classes", `["mcp.auth"]`})),
		ok("class mcp.token_expired", withFact(kv{"classes", `["mcp.token_expired"]`})),
		ok("several allowed classes", withFact(kv{"classes", `["cred.registry", "mcp.auth"]`})),
		bad("class authz.denied", withFact(kv{"classes", `["authz.denied"]`}), ReasonClassNotAuthn, "/facts/0/classes/0"),
		bad("class authz.* even when it ends _expired", withFact(kv{"classes", `["authz.token_expired"]`}), ReasonClassNotAuthn, "/facts/0/classes/0"),
		bad("class *.forbidden", withFact(kv{"classes", `["cred.aws.forbidden"]`}), ReasonClassNotAuthn, "/facts/0/classes/0"),
		bad("class forbidden that would otherwise end _expired", withFact(kv{"classes", `["cred.forbidden_expired"]`}), ReasonClassNotAuthn, "/facts/0/classes/0"),
		bad("class 403 that would otherwise end _expired", withFact(kv{"classes", `["http.403_expired"]`}), ReasonClassNotAuthn, "/facts/0/classes/0"),
		bad("class _missing without the cred. prefix", withFact(kv{"classes", `["net.dns_missing"]`}), ReasonClassNotAuthn, "/facts/0/classes/0"),
		bad("class 403", withFact(kv{"classes", `["http.403"]`}), ReasonClassNotAuthn, "/facts/0/classes/0"),
		bad("class not on the list", withFact(kv{"classes", `["net.timeout"]`}), ReasonClassNotAuthn, "/facts/0/classes/0"),
		bad("second class refused", withFact(kv{"classes", `["mcp.auth", "authz.denied"]`}), ReasonClassNotAuthn, "/facts/0/classes/1"),
		bad("cred.registry is not a prefix", withFact(kv{"classes", `["cred.registry_other"]`}), ReasonClassNotAuthn, "/facts/0/classes/0"),
		bad("authz class on kube_context", withFact(kv{"kind", `"kube_context"`}, kv{"mechanism", `"context"`}, kv{"classes", `["authz.denied"]`}), ReasonClassNotAuthn, "/facts/0/classes/0"),
		bad("authz class on package_index_host", withFact(kv{"kind", `"package_index_host"`}, kv{"mechanism", `"index"`}, kv{"classes", `["authz.denied"]`}), ReasonClassNotAuthn, "/facts/0/classes/0"),
		bad("authz class on aws_region", withFact(kv{"kind", `"aws_region"`}, kv{"mechanism", `"region"`}, kv{"classes", `["authz.denied"]`}), ReasonClassNotAuthn, "/facts/0/classes/0"),
		bad("authz class on kube_namespace", withFact(kv{"kind", `"kube_namespace"`}, kv{"mechanism", `"namespace"`}, kv{"classes", `["authz.denied"]`}), ReasonClassNotAuthn, "/facts/0/classes/0"),
		ok("env_var_name is not restricted", withFact(kv{"kind", `"env_var_name"`}, kv{"mechanism", `"env var name"`}, kv{"classes", `["net.timeout"]`})),
		ok("sign_in_step is not restricted", withFact(kv{"kind", `"sign_in_step"`}, kv{"mechanism", `"sign-in step"`}, kv{"classes", `["net.timeout"]`})),
		ok("value is an identifier", withFact(kv{"value", `"my-profile_1.x"`})),
		bad("value with a space", withFact(kv{"value", `"my profile"`}), ReasonNotIdentifier, "/facts/0/value"),
		bad("value with a tab", withFact(kv{"value", `"my\tprofile"`}), ReasonControl, "/facts/0/value"),
		bad("value with a no-break space", withFact(kv{"value", `"my\u00a0profile"`}), ReasonNotIdentifier, "/facts/0/value"),
		bad("fact without programs", withFact(kv{"programs", "-"}), ReasonRequired, "/facts/0/programs"),
		bad("fact without classes", withFact(kv{"classes", "-"}), ReasonRequired, "/facts/0/classes"),
		bad("fact without value", withFact(kv{"value", "-"}), ReasonRequired, "/facts/0/value"),
		bad("fact without mechanism", withFact(kv{"mechanism", "-"}), ReasonRequired, "/facts/0/mechanism"),
	)

	// First error is the first in schema walk order.
	add(
		bad("walk: gates before rules", top(`"gates": [`+gate(kv{"kind", `"x"`})+`], "rules": [`+rule(kv{"id", `"BAD"`})+`]`), ReasonEnum, "/gates/0/kind"),
		bad("walk: rules before facts", top(`"rules": [`+rule(kv{"id", `"BAD"`})+`], "facts": [`+fact(kv{"id", `"BAD"`})+`]`), ReasonID, "/rules/0/id"),
		bad("walk: id before why", withRule(kv{"id", `"BAD"`}, kv{"why", `""`}), ReasonID, "/rules/0/id"),
		bad("walk: file order inside a list", top(`"rules": [`+rule()+`, `+rule(kv{"id", `"r2"`}, kv{"why", `""`})+`, `+rule(kv{"id", `"BAD"`})+`]`), ReasonEmpty, "/rules/1/why"),
	)

	// Regular expressions: RE2 and the archive's bounds.
	pat := func(p string) string { return withRule(kv{"pattern", q(p)}) }
	pp := "/rules/0/pattern"
	add(
		ok("pattern a", pat(`a`)),
		ok("pattern of 512 bytes", pat(strings.Repeat("a", 512))),
		over("pattern of 513 bytes", pat(strings.Repeat("a", 513)), ReasonPatternLong, pp, MaxPatternBytes),
		bad("pattern empty", pat(``), ReasonEmpty, pp),
		ok("repeat of 100", pat(`a{100}`)),
		over("repeat of 101", pat(`a{101}`), ReasonRepeatTooLarge, pp, MaxRepeat),
		over("repeat of 1001, Go's own ceiling", pat(`a{1001}`), ReasonRepeatTooLarge, pp, MaxRepeat),
		over("repeat range max 101", pat(`a{2,101}`), ReasonRepeatTooLarge, pp, MaxRepeat),
		over("repeat open min 101", pat(`a{101,}`), ReasonRepeatTooLarge, pp, MaxRepeat),
		ok("repeat open min 100", pat(`a{100,}`)),
		ok("sibling counted repeats", pat(`a{2}b{3}`)),
		ok("counted repeats in alternation", pat(`(?:ab){2}|c{3}`)),
		ok("star inside a counted repeat", pat(`(?:a*){3}`)),
		ok("counted repeat inside a star", pat(`(?:a{2})*`)),
		bad("nested counted repeat", pat(`(?:a{2}){3}`), ReasonNestedRepeat, pp),
		bad("nested counted repeat near Go's own product ceiling", pat(`(?:a{31}){31}`), ReasonNestedRepeat, pp),
		// Go's parser refuses a repeat whose product passes 1000 before the
		// nesting rule can look; the pattern is refused either way, and the
		// reported rule is the repeat count.
		over("nested repeat past Go's own product ceiling", pat(`(?:a{100}){100}`), ReasonRepeatTooLarge, pp, MaxRepeat),
		bad("nested counted repeat two groups deep", pat(`(?:(?:a|b{2})c){2}`), ReasonNestedRepeat, pp),
		over("the archive's own example", pat(`(?:\w{1000}){1000}`), ReasonRepeatTooLarge, pp, MaxRepeat),
		bad("a repeat of a repeat is not RE2", pat(`a{2}{3}`), ReasonPatternSyntax, pp),
		bad("unclosed group", pat(`(`), ReasonPatternSyntax, pp),
		bad("unclosed class", pat(`[`), ReasonPatternSyntax, pp),
		bad("backreference", pat(`(a)\1`), ReasonPatternSyntax, pp),
		bad("lookahead", pat(`(?=a)`), ReasonPatternSyntax, pp),
		bad("lookbehind", pat(`(?<=a)b`), ReasonPatternSyntax, pp),
		ok("pattern at 2000 instructions", pat(pat2000)),
		over("pattern at 2001 instructions", pat(pat2001), ReasonPatternComplex, pp, MaxPatternInstructions),
		ok("success_line", withGate(kv{"success_line", `["^ok$"]`})),
		bad("success_line bad", withGate(kv{"success_line", `["("]`}), ReasonPatternSyntax, "/gates/0/success_line/0"),
		bad("failure_line bad", withGate(kv{"failure_line", `["^ok$", "("]`}), ReasonPatternSyntax, "/gates/0/failure_line/1"),
		bad("empty_run_line bad", withGate(kv{"empty_run_line", `["(?=x)"]`}), ReasonPatternSyntax, "/gates/0/empty_run_line/0"),
		bad("success_line a string, not a list", withGate(kv{"success_line", `"^ok$"`}), ReasonType, "/gates/0/success_line"),
		over("failure_line 101", withGate(kv{"failure_line", `["a{101}"]`}), ReasonRepeatTooLarge, "/gates/0/failure_line/0", MaxRepeat),
		ok("ten patterns of 2000 are 20,000 in all", rulesOfPatterns(10, pat2000)),
		ok("patterns that total exactly 20,000", mixedRules(pat2000, pat2000, pat2000, pat2000, pat2000, pat2000, pat2000, pat2000, pat2000, pat1997, "a")),
		over("patterns that total 20,001", mixedRules(pat2000, pat2000, pat2000, pat2000, pat2000, pat2000, pat2000, pat2000, pat2000, pat1998, "a"), ReasonSpecComplex, "/rules/10/pattern", MaxSpecInstructions),
		over("an eleventh pattern passes 20,000", rulesOfPatterns(11, pat2000), ReasonSpecComplex, "/rules/10/pattern", MaxSpecInstructions),
		over("gate lines count before rules",
			top(`"gates": [`+many(10, func(i int) string {
				return gate(kv{"id", q(fmt.Sprintf("g%d", i))}, kv{"success_line", `[` + q(pat2000) + `]`})
			})+`], "rules": [`+rule(kv{"pattern", `"a"`})+`]`),
			ReasonSpecComplex, "/rules/0/pattern", MaxSpecInstructions),
	)

	runRows(t, rows)
}
