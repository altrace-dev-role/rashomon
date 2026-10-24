package spec

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// exampleSpec is archive 20.2.8's generic example, character for character.
const exampleSpec = `{
  "rashomon_spec": 1,
  "gates": [
    {"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]},
    {"id": "lint", "kind": "lint", "argv": ["golangci-lint", "run"], "success_line": ["^0 issues\\.$"]},
    {"id": "verify", "kind": "custom", "argv": ["python", "tools/verify.py"], "aliases": [["make", "verify"]],
     "applies_to": ["jobs/**/*.py"]}
  ],
  "ci_commands": [["go", "vet", "./..."]],
  "rules": [
    {"id": "no-shell-true", "kind": "forbid_code", "languages": ["python"],
     "pattern": "subprocess\\.(Popen|run|call|check_output)\\([^)]*shell\\s*=\\s*True",
     "why": "The platform SAST gate rejects shell=True.", "hard_control": "CI scanner job",
     "alternative": "pass an argument list; for AWS calls use boto3"},
    {"id": "frozen-migrations", "kind": "protect_path", "paths": ["db/migrations/**"], "modes": ["modify", "delete"],
     "why": "Applied migrations are immutable.", "hard_control": "CODEOWNERS on db/migrations",
     "alternative": "add a new migration file"},
    {"id": "scan-before-push", "kind": "require_evidence", "at": ["push"], "evidence": ["kind:scan", "unit"],
     "why": "CI rejects pushes with scanner findings.", "hard_control": "CI scanner job"},
    {"id": "export-after-verify", "kind": "order",
     "then": {"argv": ["python", "jobs/export.py"]}, "after_gate": "verify", "same_run_key": ["--mode"],
     "invalidated_by": ["jobs/**"],
     "why": "Exports must follow a verification of the same mode.", "hard_control": "none; review only"},
    {"id": "no-truncate", "kind": "forbid_command", "command": {"argv": ["cqlsh"], "sql": ["TRUNCATE"]},
     "why": "Shared keyspaces are never truncated.", "hard_control": "database role grants"}
  ]
}`

// exampleDigest and minimalDigest were computed by a second route, a
// standalone script that sorts keys by UTF-16 code unit and writes RFC 8785
// strings (hashlib over its output), from the example above and from
// {"rashomon_spec":1}.
const (
	exampleDigest = "sha256:80ba3d175c10d926d988622acd8d43778b3f38e9980d70df093ec4401f3f7faa"
	minimalDigest = "sha256:e736acc287246d179f9da3baa6ba9fd05849e1ed4ca02ff5befca3c4899436ac"
)

// Builders for documents. Each returns JSON text; none validates anything.

func gateWith(extra string) string {
	if extra != "" {
		extra = "," + extra
	}
	return `{"id":"g","kind":"test","argv":["go","test"]` + extra + `}`
}

func ruleWith(kind, extra string) string {
	if extra != "" {
		extra = "," + extra
	}
	return `{"id":"r","kind":"` + kind + `","why":"w","hard_control":"h"` + extra + `}`
}

// validRule is the smallest valid rule of each kind: the fields that kind
// requires and nothing else.
var validRule = map[string]string{
	"forbid_code":      `"pattern":"x"`,
	"forbid_command":   `"command":{"argv":["rm"]}`,
	"protect_path":     `"paths":["a/**"]`,
	"require_evidence": `"at":["push"],"evidence":["unit"]`,
	"order":            `"then":{"argv":["x"]},"after_gate":"unit"`,
}

const validFact = `{"id":"f","kind":"aws_profile","programs":["aws"],"classes":["cred.aws_profile_missing"],"value":"dev","mechanism":"profile"}`

func doc(parts ...string) string {
	return `{"rashomon_spec":1` + strings.Join(append([]string{""}, parts...), ",") + `}`
}

func withGates(gs ...string) string { return doc(`"gates":[` + strings.Join(gs, ",") + `]`) }
func withRules(rs ...string) string { return doc(`"rules":[` + strings.Join(rs, ",") + `]`) }
func withFacts(fs ...string) string { return doc(`"facts":[` + strings.Join(fs, ",") + `]`) }

func gen(n int, one func(i int) string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = one(i)
	}
	return out
}

func accept(t *testing.T, text string) *Compiled {
	t.Helper()
	c, err := Parse("spec.json", []byte(text), Untrusted)
	if err != nil {
		t.Fatalf("want accepted, got %v\n%.300s", err, text)
	}
	if c == nil {
		t.Fatal("nil Compiled and nil error")
	}
	return c
}

func refuse(t *testing.T, text, pointer string, reason Reason) *Error {
	t.Helper()
	c, err := Parse("spec.json", []byte(text), Untrusted)
	if err == nil {
		t.Fatalf("want %s at %q, got accepted\n%.300s", reason, pointer, text)
	}
	if c != nil {
		t.Errorf("a refused spec returned a Compiled: %+v", c)
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error is %T, want *Error: %v", err, err)
	}
	if e.Pointer != pointer || e.Reason != reason {
		t.Errorf("first error = %s at %q, want %s at %q", e.Reason, e.Pointer, reason, pointer)
	}
	return e
}

func TestVER39_DigestCanonical(t *testing.T) {
	// The same document three ways: as written, keys reversed and spread over
	// lines with tabs, and compact with the nested keys reversed.
	a := accept(t, `{"rashomon_spec":1,"gates":[{"id":"u","kind":"test","argv":["go","test"]}]}`)
	b := accept(t, "{\n\t\"gates\" :\t[ { \"argv\":[\"go\",\"test\"],\n \"kind\":\"test\" , \"id\":\"u\" } ] ,\r\n\"rashomon_spec\" : 1 }\n")
	c := accept(t, `{"gates":[{"kind":"test","id":"u","argv":["go","test"]}],"rashomon_spec":1}`)
	if a.Digest != b.Digest || a.Digest != c.Digest {
		t.Errorf("digest moved with key order or whitespace:\n%s\n%s\n%s", a.Digest, b.Digest, c.Digest)
	}
	// Content moves it: one changed word, and an escape spelling of the same
	// string does not ("g\u006f" is "go").
	d := accept(t, `{"rashomon_spec":1,"gates":[{"id":"u","kind":"test","argv":["go","vet"]}]}`)
	e := accept(t, `{"rashomon_spec":1,"gates":[{"id":"u","kind":"test","argv":["g\u006f","test"]}]}`)
	if d.Digest == a.Digest {
		t.Error("digest did not move when a word changed")
	}
	if e.Digest != a.Digest {
		t.Errorf("an escaped spelling of the same string moved the digest:\n%s\n%s", e.Digest, a.Digest)
	}
	// Pinned values from the second route.
	if got := accept(t, `{"rashomon_spec":1}`).Digest; got != minimalDigest {
		t.Errorf("minimal digest = %s, want %s", got, minimalDigest)
	}
	ex := accept(t, exampleSpec)
	if ex.Digest != exampleDigest {
		t.Errorf("example digest = %s, want %s", ex.Digest, exampleDigest)
	}
	if len(ex.Sources) != 1 || ex.Sources[0].Digest != ex.Digest || ex.Sources[0].Path != "spec.json" || ex.Sources[0].Trust != Untrusted {
		t.Errorf("Sources = %+v, want the one file with its digest", ex.Sources)
	}
}

func TestSP1_ParseWithinLimits(t *testing.T) {
	c := accept(t, exampleSpec)
	yes := true
	wantGates := []Gate{
		{ID: "unit", Kind: "test", Argv: []string{"go", "test", "./..."}, Match: "prefix", Dir: ".", Required: &yes},
		{ID: "lint", Kind: "lint", Argv: []string{"golangci-lint", "run"}, Match: "prefix", Dir: ".", Required: &yes, SuccessLine: []string{`^0 issues\.$`}},
		{ID: "verify", Kind: "custom", Argv: []string{"python", "tools/verify.py"}, Match: "prefix", Dir: ".", Required: &yes,
			Aliases: [][]string{{"make", "verify"}}, AppliesTo: []string{"jobs/**/*.py"}},
	}
	if !reflect.DeepEqual(c.Gates, wantGates) {
		t.Errorf("Gates = %+v\nwant   %+v", c.Gates, wantGates)
	}
	if want := [][]string{{"go", "vet", "./..."}}; !reflect.DeepEqual(c.CICommands, want) {
		t.Errorf("CICommands = %v, want %v", c.CICommands, want)
	}
	wantRules := []Rule{
		{ID: "no-shell-true", Kind: ForbidCode, Languages: []string{"python"},
			Pattern: `subprocess\.(Popen|run|call|check_output)\([^)]*shell\s*=\s*True`,
			Why:     "The platform SAST gate rejects shell=True.", HardControl: "CI scanner job",
			Alternative: "pass an argument list; for AWS calls use boto3"},
		{ID: "frozen-migrations", Kind: ProtectPath, Paths: []string{"db/migrations/**"}, Modes: []string{"modify", "delete"},
			Why: "Applied migrations are immutable.", HardControl: "CODEOWNERS on db/migrations", Alternative: "add a new migration file"},
		{ID: "scan-before-push", Kind: RequireEvidence, At: []string{"push"}, Evidence: []string{"kind:scan", "unit"},
			Why: "CI rejects pushes with scanner findings.", HardControl: "CI scanner job"},
		{ID: "export-after-verify", Kind: Order, Then: &CommandMatch{Argv: []string{"python", "jobs/export.py"}},
			AfterGate: "verify", SameRunKey: []string{"--mode"}, InvalidatedBy: []string{"jobs/**"},
			Why: "Exports must follow a verification of the same mode.", HardControl: "none; review only"},
		{ID: "no-truncate", Kind: ForbidCommand, Command: &CommandMatch{Argv: []string{"cqlsh"}, SQL: []string{"TRUNCATE"}},
			Why: "Shared keyspaces are never truncated.", HardControl: "database role grants"},
	}
	if !reflect.DeepEqual(c.Rules, wantRules) {
		for i := range wantRules {
			if i < len(c.Rules) && !reflect.DeepEqual(c.Rules[i], wantRules[i]) {
				t.Errorf("Rules[%d] = %+v\nwant     %+v", i, c.Rules[i], wantRules[i])
			}
		}
		t.Errorf("Rules differ (%d parsed, %d wanted)", len(c.Rules), len(wantRules))
	}
	if len(c.Facts) != 0 {
		t.Errorf("Facts = %+v, want none", c.Facts)
	}
	// A fact, every field, with the optional target and why.
	f := accept(t, withFacts(`{"id":"prod","kind":"kube_context","programs":["kubectl","helm"],`+
		`"target":{"kind":"kube_context","id":"arn:aws:eks:us-east-1:123:cluster/x"},`+
		`"classes":["cred.kube_context","kube_token_expired"],"value":"prod-eks","mechanism":"context","why":"the prod cluster"}`))
	wantFact := Fact{ID: "prod", Kind: "kube_context", Programs: []string{"kubectl", "helm"},
		Target:  &FactTarget{Kind: "kube_context", ID: "arn:aws:eks:us-east-1:123:cluster/x"},
		Classes: []string{"cred.kube_context", "kube_token_expired"}, Value: "prod-eks", Mechanism: "context", Why: "the prod cluster"}
	if len(f.Facts) != 1 || !reflect.DeepEqual(f.Facts[0], wantFact) {
		t.Errorf("Facts = %+v, want %+v", f.Facts, wantFact)
	}
	// Every gate field, including exit codes and the three line lists.
	g := accept(t, withGates(gateWith(`"match":"exact","dir":"svc/api","partial_flags":["--short"],`+
		`"success_line":["^ok"],"failure_line":["^FAIL"],"empty_run_line":["no tests"],`+
		`"exit_codes":{"0":"pass","5":"empty_run","2":"fail"},"required":false`)))
	no := false
	wantGate := Gate{ID: "g", Kind: "test", Argv: []string{"go", "test"}, Match: "exact", Dir: "svc/api", Required: &no,
		PartialFlags: []string{"--short"}, SuccessLine: []string{"^ok"}, FailureLine: []string{"^FAIL"}, EmptyRunLine: []string{"no tests"},
		ExitCodes: map[string]string{"0": "pass", "5": "empty_run", "2": "fail"}}
	if len(g.Gates) != 1 || !reflect.DeepEqual(g.Gates[0], wantGate) {
		t.Errorf("Gate = %+v, want %+v", g.Gates, wantGate)
	}
	// Every CommandMatch field, on a forbid_command.
	cm := accept(t, withRules(ruleWith("forbid_command", `"command":{"argv":["git","push"],"flags":["--force","-f"],`+
		`"require_flags":["--dry-run"],"flag_values":{"--force-with-lease":["main","Master"]}}`)))
	wantCM := &CommandMatch{Argv: []string{"git", "push"}, Flags: []string{"--force", "-f"}, RequireFlags: []string{"--dry-run"},
		FlagValues: map[string][]string{"--force-with-lease": {"main", "Master"}}}
	if !reflect.DeepEqual(cm.Rules[0].Command, wantCM) {
		t.Errorf("Command = %+v, want %+v", cm.Rules[0].Command, wantCM)
	}
	// Trust is carried; an unknown trust is untrusted.
	for _, tc := range []struct{ in, want Trust }{{Trusted, Trusted}, {Managed, Managed}, {Untrusted, Untrusted}, {"bogus", Untrusted}} {
		got, err := Parse("p", []byte(`{"rashomon_spec":1}`), tc.in)
		if err != nil || got.Trust != tc.want || got.Sources[0].Trust != tc.want {
			t.Errorf("trust %q: got %+v, %v; want %q", tc.in, got, err, tc.want)
		}
	}
}

func TestSP3_UnknownKeyRefusedAtEveryLevel(t *testing.T) {
	// cmd is a command match with one extra key: "bogus" with a number, or
	// "flags" with a flag list.
	cmd := func(key string) string {
		if key == "flags" {
			return `{"argv":["x"],"flags":["-y"]}`
		}
		return `{"argv":["x"],"` + key + `":1}`
	}
	for _, tc := range []struct{ name, bad, ok, pointer string }{
		{"top", `{"rashomon_spec":1,"bogus":[]}`, `{"rashomon_spec":1}`, "/bogus"},
		{"gate", withGates(gateWith(`"bogus":1`)), withGates(gateWith(``)), "/gates/0/bogus"},
		{"rule", withRules(ruleWith("forbid_code", validRule["forbid_code"]+`,"bogus":1`)), withRules(ruleWith("forbid_code", validRule["forbid_code"])), "/rules/0/bogus"},
		{"command", withRules(ruleWith("forbid_command", `"command":`+cmd("bogus"))), withRules(ruleWith("forbid_command", `"command":`+cmd("flags"))), "/rules/0/command/bogus"},
		{"before", withRules(ruleWith("require_evidence", `"before":`+cmd("bogus")+`,"evidence":["u"]`)), withRules(ruleWith("require_evidence", `"before":`+cmd("flags")+`,"evidence":["u"]`)), "/rules/0/before/bogus"},
		{"then", withRules(ruleWith("order", `"then":`+cmd("bogus")+`,"after_gate":"u"`)), withRules(ruleWith("order", `"then":`+cmd("flags")+`,"after_gate":"u"`)), "/rules/0/then/bogus"},
		{"after", withRules(ruleWith("order", `"then":{"argv":["x"]},"after":`+cmd("bogus"))), withRules(ruleWith("order", `"then":{"argv":["x"]},"after":`+cmd("flags"))), "/rules/0/after/bogus"},
		{"fact", withFacts(strings.Replace(validFact, `"value"`, `"bogus":1,"value"`, 1)), withFacts(validFact), "/facts/0/bogus"},
		{"fact target", withFacts(strings.Replace(validFact, `"value"`, `"target":{"kind":"aws_region","id":"us-east-1","bogus":1},"value"`, 1)),
			withFacts(strings.Replace(validFact, `"value"`, `"target":{"kind":"aws_region","id":"us-east-1"},"value"`, 1)), "/facts/0/target/bogus"},
		// encoding/json would match "ID" to "id" and "Kind" to "kind"; a key
		// is exact here.
		{"case variant", withGates(strings.Replace(gateWith(``), `"id"`, `"ID"`, 1)), withGates(gateWith(``)), "/gates/0/ID"},
		// A key for a field of another level is unknown at this one.
		{"gate key on a rule", withRules(ruleWith("forbid_code", validRule["forbid_code"]+`,"argv":["x"]`)), withRules(ruleWith("forbid_code", validRule["forbid_code"])), "/rules/0/argv"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			accept(t, tc.ok)
			refuse(t, tc.bad, tc.pointer, ReasonUnknownKey)
		})
	}
	// The first unknown key in the file's order is the one named.
	refuse(t, `{"rashomon_spec":1,"zzz":1,"aaa":2}`, "/zzz", ReasonUnknownKey)
	// A duplicate key is refused, not resolved to the last value.
	refuse(t, `{"rashomon_spec":1,"rashomon_spec":1}`, "/rashomon_spec", ReasonDuplicateKey)
	refuse(t, withGates(`{"id":"a","id":"b","kind":"test","argv":["x"]}`), "/gates/0/id", ReasonDuplicateKey)
}

func TestSP4_RequiredDefaultsTrue(t *testing.T) {
	for _, tc := range []struct {
		name, extra string
		want        bool
	}{
		{"absent", ``, true},
		{"explicit true", `"required":true`, true},
		{"explicit false", `"required":false`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := accept(t, withGates(gateWith(tc.extra))).Gates[0]
			if g.Required == nil || *g.Required != tc.want {
				t.Errorf("Required = %v, want %v", g.Required, tc.want)
			}
		})
	}
	// The other defaults ride with it: prefix, ".", and all three modes.
	g := accept(t, withGates(gateWith(``))).Gates[0]
	if g.Match != "prefix" || g.Dir != "." {
		t.Errorf("defaults: Match %q Dir %q, want prefix and .", g.Match, g.Dir)
	}
	r := accept(t, withRules(ruleWith("protect_path", validRule["protect_path"]))).Rules[0]
	if want := []string{"modify", "delete", "create"}; !reflect.DeepEqual(r.Modes, want) {
		t.Errorf("default Modes = %v, want %v", r.Modes, want)
	}
	// Two parsed gates do not share one default pointer.
	two := accept(t, withGates(gateWith(``), strings.Replace(gateWith(``), `"g"`, `"h"`, 1))).Gates
	*two[0].Required = false
	if !*two[1].Required {
		t.Error("the default Required pointer is shared between gates")
	}
	// A non-boolean required is a type error at its own pointer.
	refuse(t, withGates(gateWith(`"required":"yes"`)), "/gates/0/required", ReasonType)
	refuse(t, withGates(gateWith(`"required":null`)), "/gates/0/required", ReasonType)
}

// TestSP7_ErrorCarriesTheDigest: an invalid spec still has a digest to key
// its one flag by. A file that is JSON passing the lexical checks is keyed
// by its canonical form, so reformatting the same invalid document does not
// raise a second flag; anything else is keyed by its raw bytes.
func TestSP7_ErrorCarriesTheDigest(t *testing.T) {
	bad1 := `{"rashomon_spec":1,"bogus":1}`
	bad2 := "{ \"bogus\" : 1 ,\n \"rashomon_spec\" : 1 }"
	e1 := refuse(t, bad1, "/bogus", ReasonUnknownKey)
	e2 := refuse(t, bad2, "/bogus", ReasonUnknownKey)
	if e1.Digest != e2.Digest || !e1.Canonical || !e2.Canonical {
		t.Errorf("same invalid document, two layouts: %s (canonical %v) and %s (canonical %v)", e1.Digest, e1.Canonical, e2.Digest, e2.Canonical)
	}
	// Independently: the canonical form of bad1 is {"bogus":1,"rashomon_spec":1}.
	if want := "sha256:cc1290f657b0c75e33f6f7da54396ec32593b7d6e36734d2b769b9b6a5f58714"; e1.Digest != want {
		t.Errorf("digest = %s, want %s", e1.Digest, want)
	}
	// Not JSON: the digest is of the raw bytes, and says so.
	raw := `{"rashomon_spec":1,`
	e3 := refuse(t, raw, "", ReasonSyntax)
	if e3.Canonical || e3.Digest != rawDigest([]byte(raw)) {
		t.Errorf("unparseable file: digest %s canonical %v, want the raw digest %s", e3.Digest, e3.Canonical, rawDigest([]byte(raw)))
	}
	// A float is lexically valid JSON but not canonicalisable here: raw.
	e4 := refuse(t, `{"rashomon_spec":1.5}`, "/rashomon_spec", ReasonNotInteger)
	if e4.Canonical {
		t.Error("a float spec was given a canonical digest")
	}
	// Two different invalid files never share a digest.
	if e1.Digest == e3.Digest || e3.Digest == e4.Digest {
		t.Error("distinct files share a digest")
	}
	// The message names the path, reason, pointer and limit and nothing else.
	got := (&Error{Path: "p", Pointer: "/a", Reason: ReasonTooMany, Limit: 3}).Error()
	if want := "spec p: too_many at /a (limit 3)"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got := (&Error{Path: "p", Reason: ReasonSyntax}).Error(); got != "spec p: json_syntax" {
		t.Errorf("Error() without pointer or limit = %q", got)
	}
}

// TestSP7_ErrorTextIsBounded: a hostile key or value never reaches the error.
func TestSP7_ErrorTextIsBounded(t *testing.T) {
	long := strings.Repeat("k", 49)
	for _, tc := range []struct{ name, doc, pointer string }{
		{"long key", `{"rashomon_spec":1,"` + long + `":1}`, "/?"},
		{"upper-case key", `{"rashomon_spec":1,"SECRET_TOKEN":1}`, "/SECRET_TOKEN"},
		{"key with spaces", `{"rashomon_spec":1,"ignore previous":1}`, "/?"},
		{"slash in key", `{"rashomon_spec":1,"a/b":1}`, "/?"},
		{"tilde in key", `{"rashomon_spec":1,"a~b":1}`, "/?"},
		{"48 byte key", `{"rashomon_spec":1,"` + long[:48] + `":1}`, "/" + long[:48]},
		{"short key", `{"rashomon_spec":1,"note.v2-x_y":1}`, "/note.v2-x_y"},
		{"unicode key", `{"rashomon_spec":1,"é":1}`, "/?"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := refuse(t, tc.doc, tc.pointer, ReasonUnknownKey)
			if len(e.Error()) > 200 {
				t.Errorf("Error() is %d bytes: %s", len(e.Error()), e.Error())
			}
		})
	}
	// An id that is wrong never appears in the message either.
	e := refuse(t, withGates(strings.Replace(gateWith(``), `"g"`, `"SECRET_ID"`, 1)), "/gates/0/id", ReasonID)
	if strings.Contains(e.Error(), "SECRET") {
		t.Errorf("error repeats the file's text: %s", e.Error())
	}
}

// TestSP6_HostileBytes: NUL, invalid UTF-8, deep nesting, oversize, and the
// other things a hostile repository can put in a file, each refused with a
// closed reason and no panic.
func TestSP6_HostileBytes(t *testing.T) {
	for _, tc := range []struct {
		name, doc, pointer string
		reason             Reason
	}{
		{"empty", ``, "", ReasonSyntax},
		{"whitespace only", " \n\t ", "", ReasonSyntax},
		{"NUL byte", "{\"rashomon_spec\":1}\x00", "", ReasonSyntax},
		{"NUL inside a string", "{\"rashomon_spec\":1,\"gates\":[{\"id\":\"a\x00b\"}]}", "/gates/0/id", ReasonSyntax},
		{"invalid UTF-8", "{\"rashomon_spec\":1,\"x\":\"\xff\"}", "", ReasonNotUTF8},
		{"overlong UTF-8", "{\"rashomon_spec\":1,\"x\":\"\xc0\xaf\"}", "", ReasonNotUTF8},
		{"byte order mark", "\xef\xbb\xbf{\"rashomon_spec\":1}", "", ReasonSyntax},
		{"trailing data", `{"rashomon_spec":1} {}`, "", ReasonSyntax},
		{"trailing comma", `{"rashomon_spec":1,}`, "", ReasonSyntax},
		{"single quotes", `{'rashomon_spec':1}`, "", ReasonSyntax},
		{"comment", `{"rashomon_spec":1 /* x */}`, "", ReasonSyntax},
		{"unterminated string", `{"rashomon_spec":1,"x":"abc`, "/x", ReasonSyntax},
		{"raw newline in string", "{\"rashomon_spec\":1,\"x\":\"a\nb\"}", "/x", ReasonSyntax},
		{"bad escape", `{"rashomon_spec":1,"x":"\q"}`, "/x", ReasonSyntax},
		{"short unicode escape", `{"rashomon_spec":1,"x":"\u12"}`, "/x", ReasonSyntax},
		{"lone high surrogate", `{"rashomon_spec":1,"x":"\ud800"}`, "/x", ReasonBadEscape},
		{"lone low surrogate", `{"rashomon_spec":1,"x":"\udc00"}`, "/x", ReasonBadEscape},
		{"low then low", `{"rashomon_spec":1,"x":"\udc00\udc00"}`, "/x", ReasonBadEscape},
		{"high then not low", `{"rashomon_spec":1,"x":"\ud800\u0041"}`, "/x", ReasonBadEscape},
		{"top-level array", `[1]`, "", ReasonType},
		{"top-level string", `"x"`, "", ReasonType},
		{"top-level null", `null`, "", ReasonType},
		{"leading zero", `{"rashomon_spec":01}`, "", ReasonSyntax},
		{"bare minus", `{"rashomon_spec":-}`, "/rashomon_spec", ReasonSyntax},
		{"float", `{"rashomon_spec":1.0}`, "/rashomon_spec", ReasonNotInteger},
		{"exponent", `{"rashomon_spec":1e0}`, "/rashomon_spec", ReasonNotInteger},
		{"float elsewhere", `{"rashomon_spec":1,"x":[0.5]}`, "/x/0", ReasonNotInteger},
		{"integer past 2^53", `{"rashomon_spec":9007199254740993}`, "/rashomon_spec", ReasonIntegerRange},
		{"integer past int64", `{"rashomon_spec":99999999999999999999}`, "/rashomon_spec", ReasonIntegerRange},
		{"negative past 2^53", `{"rashomon_spec":-9007199254740992}`, "/rashomon_spec", ReasonIntegerRange},
		{"control char in why", withRules(strings.Replace(ruleWith("forbid_code", validRule["forbid_code"]), `"w"`, `"a\u0007b"`, 1)), "/rules/0/why", ReasonChars},
		{"newline in why", withRules(strings.Replace(ruleWith("forbid_code", validRule["forbid_code"]), `"w"`, `"a\nb"`, 1)), "/rules/0/why", ReasonChars},
		{"bidi override in why", withRules(strings.Replace(ruleWith("forbid_code", validRule["forbid_code"]), `"w"`, `"a‮b"`, 1)), "/rules/0/why", ReasonChars},
		{"bidi embedding in why", withRules(strings.Replace(ruleWith("forbid_code", validRule["forbid_code"]), `"w"`, `"a\u202ab"`, 1)), "/rules/0/why", ReasonChars},
		{"bidi isolate in why", withRules(strings.Replace(ruleWith("forbid_code", validRule["forbid_code"]), `"w"`, `"a\u2066b"`, 1)), "/rules/0/why", ReasonChars},
		{"bidi isolate pop in why", withRules(strings.Replace(ruleWith("forbid_code", validRule["forbid_code"]), `"w"`, `"a\u2069b"`, 1)), "/rules/0/why", ReasonChars},
		{"C1 control in why", withRules(strings.Replace(ruleWith("forbid_code", validRule["forbid_code"]), `"w"`, `"a\u0085b"`, 1)), "/rules/0/why", ReasonChars},
		{"space in an argv word", withGates(`{"id":"g","kind":"test","argv":["go test"]}`), "/gates/0/argv/0", ReasonChars},
	} {
		t.Run(tc.name, func(t *testing.T) { refuse(t, tc.doc, tc.pointer, tc.reason) })
	}
	// Characters next to the refused ranges are text: U+2029, U+202F, U+2065
	// and U+206A are not controls and not bidirectional overrides.
	for _, ok := range []string{`\u2029`, `\u202f`, `\u2065`, `\u206a`, `\u00e9`, `\ud83d\ude00`} {
		accept(t, withRules(strings.Replace(ruleWith("forbid_code", validRule["forbid_code"]), `"w"`, `"a`+ok+`b"`, 1)))
	}
	// Depth: the file is level 1, so "x" may nest arrays to level MaxDepth,
	// that is MaxDepth-1 arrays; one more is refused where it opens, and
	// 100,000 more stop at the same place without recursing that far.
	nest := func(k int) string {
		return `{"rashomon_spec":1,"x":` + strings.Repeat("[", k) + strings.Repeat("]", k) + `}`
	}
	opens := "/x" + strings.Repeat("/0", MaxDepth-1)
	if e := refuse(t, nest(MaxDepth-1), "/x", ReasonUnknownKey); !e.Canonical {
		t.Error("a document at the depth cap was not canonicalised")
	}
	if e := refuse(t, nest(MaxDepth), opens, ReasonTooDeep); e.Canonical {
		t.Error("a document over the depth cap was canonicalised")
	}
	refuse(t, `{"rashomon_spec":1,"x":`+strings.Repeat("[", 100000), opens, ReasonTooDeep)
	// Value count: MaxNodes values pass the lexical pass, one more does not.
	many := func(n int) string {
		return `{"rashomon_spec":1,"x":[` + strings.TrimSuffix(strings.Repeat("0,", n-3), ",") + `]}`
	}
	if e := refuse(t, many(MaxNodes), "/x", ReasonUnknownKey); !e.Canonical {
		t.Error("a document at the value cap was not canonicalised")
	}
	refuse(t, many(MaxNodes+1), "/x/"+fmt.Sprint(MaxNodes-3), ReasonTooManyValues)
	// A giant document of values is refused by size, with no tree built.
	refuse(t, `{"rashomon_spec":1,"x":[`+strings.Repeat("0,", MaxBytes)+`0]}`, "", ReasonTooLarge)
}

func TestSP10_ParseIsDeterministic(t *testing.T) {
	a, errA := Parse("p", []byte(exampleSpec), Trusted)
	b, errB := Parse("p", []byte(exampleSpec), Trusted)
	if errA != nil || errB != nil || !reflect.DeepEqual(a, b) {
		t.Fatalf("two parses differ: %v %v", errA, errB)
	}
	// Two errors in one file: the first in schema order is the one named, the
	// same every time (a map walk would vary).
	bad := `{"rashomon_spec":1,"facts":[{"id":"BAD"}],"rules":[{"id":"BAD"}],"gates":[{"id":"BAD"}]}`
	for i := 0; i < 20; i++ {
		refuse(t, bad, "/gates/0/id", ReasonID)
	}
	// The parsed value keeps nothing of the input slice.
	in := []byte(exampleSpec)
	c, _ := Parse("p", in, Untrusted)
	want := fmt.Sprintf("%+v", *c)
	for i := range in {
		in[i] = 'X'
	}
	if got := fmt.Sprintf("%+v", *c); got != want {
		t.Error("the parsed spec changed when the input buffer was overwritten")
	}
}

// TestSP6_EveryPrefixIsRefused: a file cut off anywhere is a syntax error and
// never a panic or a partial spec. Every prefix of the example, which
// exercises every lexer state the example reaches.
func TestSP6_EveryPrefixIsRefused(t *testing.T) {
	for i := 0; i < len(exampleSpec); i++ {
		c, err := Parse("p", []byte(exampleSpec[:i]), Untrusted)
		var e *Error
		if c != nil || !errors.As(err, &e) {
			t.Fatalf("prefix of %d bytes: Compiled %v, error %v", i, c != nil, err)
		}
		if e.Reason != ReasonSyntax && e.Reason != ReasonNotUTF8 {
			t.Fatalf("prefix of %d bytes: reason %s", i, e.Reason)
		}
	}
	// And the escapes at the end of input, where the prefixes above stop in
	// the middle of ASCII text.
	for _, tc := range []struct {
		cut, pointer string
		reason       Reason
	}{
		{`"a":"\`, "/a", ReasonSyntax}, {`"a":"\u`, "/a", ReasonSyntax}, {`"a":"\u00`, "/a", ReasonSyntax},
		{`"a":"\ud83d`, "/a", ReasonBadEscape}, {`"a":"\ud83d\`, "/a", ReasonBadEscape}, {`"a":"\ud83d\ude0`, "/a", ReasonBadEscape},
		{`"a":-`, "/a", ReasonSyntax}, {`"a":tru`, "/a", ReasonSyntax}, {`"a":nul`, "/a", ReasonSyntax},
		{`"a`, "", ReasonSyntax}, {`"a"`, "/a", ReasonSyntax}, {`"a":`, "/a", ReasonSyntax}, {`"a":1`, "", ReasonSyntax},
		{`"a":[`, "/a/0", ReasonSyntax}, {`"a":[1`, "/a", ReasonSyntax}, {``, "", ReasonSyntax},
	} {
		refuse(t, `{"rashomon_spec":1,`+tc.cut, tc.pointer, tc.reason)
	}
}

// TestSP6_LexerEscapes: every escape the grammar has decodes to its rune, hex
// digits in either case, and a surrogate pair to its one rune.
func TestSP6_LexerEscapes(t *testing.T) {
	n, err := lex([]byte("[\"" + `\"\\\/\b\f\n\r\t\u00e9\u00E9\ud83d\ude00\uD83D\uDE00` + "\"]"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := n.vals[0].str, "\"\\/\b\f\n\r\téé\U0001F600\U0001F600"; got != want {
		t.Errorf("decoded %q, want %q", got, want)
	}
	for _, bad := range []string{`"\u12g4"`, `"\uzzzz"`, `"\x41"`, `"\'"`} {
		if _, err := lex([]byte(bad)); err == nil || err.Reason != ReasonSyntax {
			t.Errorf("%s: %v, want a syntax error", bad, err)
		}
	}
}

// TestSP12_WrongTypesAtEveryField: each field refuses a value of another JSON
// type at its own pointer (parse, do not validate: a number is not a list).
func TestSP12_WrongTypesAtEveryField(t *testing.T) {
	gate := func(extra string) string { return withGates(gateWith(extra)) }
	cmd := func(extra string) string {
		return withRules(ruleWith("forbid_command", `"command":{"argv":["x"],`+extra+`}`))
	}
	fact := func(old, new string) string { return withFacts(strings.Replace(validFact, old, new, 1)) }
	for _, tc := range []struct{ name, doc, pointer string }{
		{"gates", doc(`"gates":1`), "/gates"},
		{"ci_commands", doc(`"ci_commands":"x"`), "/ci_commands"},
		{"facts", doc(`"facts":{}`), "/facts"},
		{"gate", withGates(`1`), "/gates/0"},
		{"ci command", doc(`"ci_commands":["go"]`), "/ci_commands/0"},
		{"ci word", doc(`"ci_commands":[[1]]`), "/ci_commands/0/0"},
		{"gate id", withGates(`{"id":1,"kind":"test","argv":["x"]}`), "/gates/0/id"},
		{"gate kind", withGates(`{"id":"g","kind":[],"argv":["x"]}`), "/gates/0/kind"},
		{"gate argv", withGates(`{"id":"g","kind":"test","argv":"x"}`), "/gates/0/argv"},
		{"gate argv word", withGates(`{"id":"g","kind":"test","argv":[null]}`), "/gates/0/argv/0"},
		{"gate match", gate(`"match":true`), "/gates/0/match"},
		{"gate aliases", gate(`"aliases":"x"`), "/gates/0/aliases"},
		{"gate alias", gate(`"aliases":["x"]`), "/gates/0/aliases/0"},
		{"gate alias word", gate(`"aliases":[[1]]`), "/gates/0/aliases/0/0"},
		{"gate dir", gate(`"dir":1`), "/gates/0/dir"},
		{"gate applies_to", gate(`"applies_to":{}`), "/gates/0/applies_to"},
		{"gate partial_flags", gate(`"partial_flags":"-x"`), "/gates/0/partial_flags"},
		{"gate success_line", gate(`"success_line":"x"`), "/gates/0/success_line"},
		{"gate success_line item", gate(`"success_line":[1]`), "/gates/0/success_line/0"},
		{"gate failure_line item", gate(`"failure_line":[[]]`), "/gates/0/failure_line/0"},
		{"gate empty_run_line", gate(`"empty_run_line":1`), "/gates/0/empty_run_line"},
		{"gate exit_codes", gate(`"exit_codes":[]`), "/gates/0/exit_codes"},
		{"gate exit state", gate(`"exit_codes":{"1":1}`), "/gates/0/exit_codes/1"},
		{"command flags", cmd(`"flags":"-x"`), "/rules/0/command/flags"},
		{"command flag_values", cmd(`"flag_values":[]`), "/rules/0/command/flag_values"},
		{"command flag_values list", cmd(`"flag_values":{"-f":"x"}`), "/rules/0/command/flag_values/-f"},
		{"command sql", cmd(`"sql":"X"`), "/rules/0/command/sql"},
		{"fact id", fact(`"id":"f"`, `"id":1`), "/facts/0/id"},
		{"fact kind", fact(`"aws_profile"`, `7`), "/facts/0/kind"},
		{"fact programs", fact(`["aws"]`, `"aws"`), "/facts/0/programs"},
		{"fact classes", fact(`["cred.aws_profile_missing"]`, `"cred.aws_profile_missing"`), "/facts/0/classes"},
		{"fact class", fact(`["cred.aws_profile_missing"]`, `[1]`), "/facts/0/classes/0"},
		{"fact value", fact(`"dev"`, `1`), "/facts/0/value"},
		{"fact mechanism", fact(`"profile"`, `1`), "/facts/0/mechanism"},
		{"target kind", fact(`"value"`, `"target":{"kind":1,"id":"x"},"value"`), "/facts/0/target/kind"},
	} {
		t.Run(tc.name, func(t *testing.T) { refuse(t, tc.doc, tc.pointer, ReasonType) })
	}
	// An exit code is a canonical decimal from 0 to 255.
	for _, ok := range []string{"0", "1", "255", "127"} {
		accept(t, gate(`"exit_codes":{"`+ok+`":"pass"}`))
	}
	for _, bad := range []struct{ key, segment string }{
		{"256", "256"}, {"07", "07"}, {"-1", "-1"}, {"a", "a"}, {"1.0", "1.0"}, {"+1", "?"}, {" 1", "?"}, {"0x1", "0x1"},
	} {
		refuse(t, gate(`"exit_codes":{"`+bad.key+`":"pass"}`), "/gates/0/exit_codes/"+bad.segment, ReasonShape)
	}
}

// TestSP6_StructureErrors: the separators and nesting of objects, where the
// array rows of the hostile table do not reach.
func TestSP6_StructureErrors(t *testing.T) {
	objects := func(k int) string {
		return `{"rashomon_spec":1,"x":` + strings.Repeat(`{"a":`, k) + `1` + strings.Repeat(`}`, k) + `}`
	}
	// x is level 2, so k nested objects fill levels 2 to k+1: MaxDepth-1 fit.
	if e := refuse(t, objects(MaxDepth-1), "/x", ReasonUnknownKey); !e.Canonical {
		t.Error("objects at the depth cap were not canonicalised")
	}
	refuse(t, objects(MaxDepth), "/x"+strings.Repeat("/a", MaxDepth-1), ReasonTooDeep)
	refuse(t, `{"rashomon_spec":1 "x":1}`, "", ReasonSyntax)
	refuse(t, `{"rashomon_spec":1,"x":[1 2]}`, "/x", ReasonSyntax)
	refuse(t, `{"rashomon_spec" 1}`, "/rashomon_spec", ReasonSyntax)
	refuse(t, `{1:1}`, "", ReasonSyntax)
	// A flag_values key is checked like any flag: too long, with a control
	// character, or not a flag at all.
	fv := func(k string) string {
		return withRules(ruleWith("forbid_command", `"command":{"argv":["x"],"flag_values":{"`+k+`":["v"]}}`))
	}
	accept(t, fv("-"+strings.Repeat("f", MaxWordBytes-1)))
	refuse(t, fv("-"+strings.Repeat("f", MaxWordBytes)), "/rules/0/command/flag_values/?", ReasonTooLong)
	refuse(t, fv(`-f\u0007`), "/rules/0/command/flag_values/?", ReasonChars)
	refuse(t, fv(`f`), "/rules/0/command/flag_values/f", ReasonShape)
}

// TestSP7_OversizeDigestIgnoresTheTail: the digest of a file over the cap is
// of its first MaxBytes+1 bytes, so it is the same whether the caller read
// just over the cap or the whole file.
func TestSP7_OversizeDigestIgnoresTheTail(t *testing.T) {
	a := refuse(t, sized(MaxBytes+1), "", ReasonTooLarge)
	b := refuse(t, sized(MaxBytes+1)+strings.Repeat("x", 5000), "", ReasonTooLarge)
	if a.Digest != b.Digest || a.Canonical || b.Canonical {
		t.Errorf("digests %s and %s differ, or claim to be canonical", a.Digest, b.Digest)
	}
	if want := rawDigest([]byte(sized(MaxBytes + 1))); a.Digest != want {
		t.Errorf("digest = %s, want the raw digest of the first MaxBytes+1 bytes", a.Digest)
	}
}

// TestSP2_CapsArePinned: the limits are the archive's numbers, written here
// as literals. A test that builds its over-limit input from the constant
// would follow the constant wherever it was moved.
func TestSP2_CapsArePinned(t *testing.T) {
	for _, c := range []struct {
		name      string
		got, want int
	}{
		{"MaxBytes", MaxBytes, 262144}, {"MaxGates", MaxGates, 32}, {"MaxRules", MaxRules, 64}, {"MaxFacts", MaxFacts, 64},
		{"MaxPatternBytes", MaxPatternBytes, 512}, {"MaxPatternInstructions", MaxPatternInstructions, 2000},
		{"MaxSpecInstructions", MaxSpecInstructions, 20000}, {"MaxRepeat", MaxRepeat, 100},
		{"MaxWhyRunes", MaxWhyRunes, 300}, {"MaxAlternativeRunes", MaxAlternativeRunes, 160}, {"MaxIDBytes", MaxIDBytes, 48},
		// Proposed: the archive names none of these.
		{"MaxArgvWords", MaxArgvWords, 32}, {"MaxAliases", MaxAliases, 8}, {"MaxListItems", MaxListItems, 64},
		{"MaxWordBytes", MaxWordBytes, 256}, {"MaxFlagValueKeys", MaxFlagValueKeys, 16}, {"MaxExitCodes", MaxExitCodes, 16},
		{"MaxFactValueBytes", MaxFactValueBytes, 128}, {"MaxDepth", MaxDepth, 16}, {"MaxNodes", MaxNodes, 20000},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
			}
		})
	}
}
