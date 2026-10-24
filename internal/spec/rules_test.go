package spec

import (
	"fmt"
	"strings"
	"testing"
)

// ruleFieldSamples is one valid JSON value for each field a rule can carry,
// spelled here independently of the package's own field table.
var ruleFieldSamples = map[string]string{
	"pattern": `"x"`, "languages": `["go"]`, "paths": `["a"]`, "exclude_paths": `["a"]`,
	"command": `{"argv":["x"]}`, "modes": `["modify"]`,
	"at": `["push"]`, "before": `{"argv":["x"]}`, "evidence": `["u"]`,
	"then": `{"argv":["x"]}`, "after": `{"argv":["x"]}`, "after_gate": `"u"`, "same_run_key": `["-m"]`, "invalidated_by": `["a"]`,
}

// ruleFieldsOf is the archive's grouping of fields under kinds, as comments
// in its Rule type.
var ruleFieldsOf = map[string][]string{
	"forbid_code":      {"pattern", "languages", "paths", "exclude_paths"},
	"forbid_command":   {"command"},
	"protect_path":     {"paths", "modes"},
	"require_evidence": {"at", "before", "evidence"},
	"order":            {"then", "after", "after_gate", "same_run_key", "invalidated_by"},
}

func TestSP8_RuleKindFields(t *testing.T) {
	for kind, own := range ruleFieldsOf {
		t.Run(kind, func(t *testing.T) {
			// Each field the kind owns is accepted beside its required ones.
			for _, f := range own {
				extra := validRule[kind]
				if !strings.Contains(extra, `"`+f+`"`) {
					extra += `,"` + f + `":` + ruleFieldSamples[f]
				}
				// after and after_gate exclude each other; the sample is added to
				// a rule that already has the other, so swap rather than add.
				if kind == "order" && f == "after" {
					extra = `"then":{"argv":["x"]},"after":{"argv":["x"]}`
				}
				accept(t, withRules(ruleWith(kind, extra)))
			}
			// Every field another kind owns is refused, at its own pointer.
			owns := map[string]bool{}
			for _, f := range own {
				owns[f] = true
			}
			for _, other := range ruleFieldsOf {
				for _, f := range other {
					if owns[f] {
						continue
					}
					refuse(t, withRules(ruleWith(kind, validRule[kind]+`,"`+f+`":`+ruleFieldSamples[f])), "/rules/0/"+f, ReasonFieldForKind)
				}
			}
		})
	}
	for _, tc := range []struct {
		name, doc, pointer string
		reason             Reason
	}{
		{"forbid_code needs a pattern", withRules(ruleWith("forbid_code", `"languages":["go"]`)), "/rules/0/pattern", ReasonRequired},
		{"forbid_command needs a command", withRules(ruleWith("forbid_command", ``)), "/rules/0/command", ReasonRequired},
		{"protect_path needs paths", withRules(ruleWith("protect_path", `"modes":["modify"]`)), "/rules/0/paths", ReasonRequired},
		{"protect_path paths not empty", withRules(ruleWith("protect_path", `"paths":[]`)), "/rules/0/paths", ReasonEmpty},
		{"require_evidence needs evidence", withRules(ruleWith("require_evidence", `"at":["push"]`)), "/rules/0/evidence", ReasonRequired},
		{"require_evidence evidence not empty", withRules(ruleWith("require_evidence", `"at":["push"],"evidence":[]`)), "/rules/0/evidence", ReasonEmpty},
		{"require_evidence needs at or before", withRules(ruleWith("require_evidence", `"evidence":["u"]`)), "/rules/0/at", ReasonOneOfRequired},
		{"order needs then", withRules(ruleWith("order", `"after_gate":"u"`)), "/rules/0/then", ReasonRequired},
		{"order needs after or after_gate", withRules(ruleWith("order", `"then":{"argv":["x"]}`)), "/rules/0/after", ReasonOneOfRequired},
		{"order takes one of after, after_gate", withRules(ruleWith("order", `"then":{"argv":["x"]},"after":{"argv":["y"]},"after_gate":"u"`)), "/rules/0/after_gate", ReasonExclusive},
		{"why is required", withRules(`{"id":"r","kind":"forbid_code","pattern":"x","hard_control":"h"}`), "/rules/0/why", ReasonRequired},
		{"hard_control is required", withRules(`{"id":"r","kind":"forbid_code","pattern":"x","why":"w"}`), "/rules/0/hard_control", ReasonRequired},
		{"why is not empty", withRules(`{"id":"r","kind":"forbid_code","pattern":"x","why":"","hard_control":"h"}`), "/rules/0/why", ReasonEmpty},
		{"id is required", withRules(`{"kind":"forbid_code","pattern":"x","why":"w","hard_control":"h"}`), "/rules/0/id", ReasonRequired},
		{"kind is required", withRules(`{"id":"r","pattern":"x","why":"w","hard_control":"h"}`), "/rules/0/kind", ReasonRequired},
		{"a rule is an object", withRules(`"r"`), "/rules/0", ReasonType},
		{"rules is an array", doc(`"rules":{}`), "/rules", ReasonType},
		{"why is a string", withRules(`{"id":"r","kind":"forbid_code","pattern":"x","why":7,"hard_control":"h"}`), "/rules/0/why", ReasonType},
		{"command needs argv", withRules(ruleWith("forbid_command", `"command":{}`)), "/rules/0/command/argv", ReasonRequired},
		{"command argv not empty", withRules(ruleWith("forbid_command", `"command":{"argv":[]}`)), "/rules/0/command/argv", ReasonEmpty},
		{"command is an object", withRules(ruleWith("forbid_command", `"command":["x"]`)), "/rules/0/command", ReasonType},
		{"protect_path modes at most three", withRules(ruleWith("protect_path", `"paths":["a"],"modes":["modify","delete","create","modify"]`)), "/rules/0/modes", ReasonTooMany},
		{"at at most five", withRules(ruleWith("require_evidence", `"at":["commit","push","pr","done","subtask","pr"],"evidence":["u"]`)), "/rules/0/at", ReasonTooMany},
		{"a flag holds no space", withRules(ruleWith("forbid_command", `"command":{"argv":["x"],"flags":["-a b"]}`)), "/rules/0/command/flags/0", ReasonShape},
		{"sql only for the SQL clients", withRules(ruleWith("forbid_command", `"command":{"argv":["rm"],"sql":["DROP"]}`)), "/rules/0/command/sql", ReasonShape},
		{"sql keywords are letters", withRules(ruleWith("forbid_command", `"command":{"argv":["psql"],"sql":["DROP;"]}`)), "/rules/0/command/sql/0", ReasonShape},
		{"flags start with a dash", withRules(ruleWith("forbid_command", `"command":{"argv":["x"],"flags":["force"]}`)), "/rules/0/command/flags/0", ReasonShape},
		{"a lone dash is no flag", withRules(ruleWith("forbid_command", `"command":{"argv":["x"],"require_flags":["-"]}`)), "/rules/0/command/require_flags/0", ReasonShape},
		{"flag_values keys are flags", withRules(ruleWith("forbid_command", `"command":{"argv":["x"],"flag_values":{"force":["1"]}}`)), "/rules/0/command/flag_values/force", ReasonShape},
		{"flag_values values not empty", withRules(ruleWith("forbid_command", `"command":{"argv":["x"],"flag_values":{"-f":[]}}`)), "/rules/0/command/flag_values/-f", ReasonEmpty},
		{"same_run_key are flags", withRules(ruleWith("order", `"then":{"argv":["x"]},"after_gate":"u","same_run_key":["mode"]`)), "/rules/0/same_run_key/0", ReasonShape},
		{"evidence is a gate id", withRules(ruleWith("require_evidence", `"at":["push"],"evidence":["Unit"]`)), "/rules/0/evidence/0", ReasonID},
		{"after_gate is a gate id", withRules(ruleWith("order", `"then":{"argv":["x"]},"after_gate":"Unit"`)), "/rules/0/after_gate", ReasonID},
		{"a list is a list", withRules(ruleWith("protect_path", `"paths":"a"`)), "/rules/0/paths", ReasonType},
		{"a list item is a string", withRules(ruleWith("protect_path", `"paths":[1]`)), "/rules/0/paths/0", ReasonType},
	} {
		t.Run(tc.name, func(t *testing.T) { refuse(t, tc.doc, tc.pointer, tc.reason) })
	}
	// A rule that is checked in field order reports the earlier error: the id
	// before the kind, the kind before its fields.
	refuse(t, withRules(`{"id":"BAD","kind":"nope"}`), "/rules/0/id", ReasonID)
	refuse(t, withRules(`{"id":"r","kind":"nope","bogus":1}`), "/rules/0/bogus", ReasonUnknownKey)
	refuse(t, withRules(`{"id":"r","kind":"nope"}`), "/rules/0/kind", ReasonEnum)
}

func TestSP8_PathsAreProjectRelative(t *testing.T) {
	for pointer, build := range map[string]func(v string) string{
		"/gates/0/dir":          func(v string) string { return withGates(gateWith(`"dir":"` + v + `"`)) },
		"/gates/0/applies_to/0": func(v string) string { return withGates(gateWith(`"applies_to":["` + v + `"]`)) },
		"/rules/0/paths/0":      func(v string) string { return withRules(ruleWith("protect_path", `"paths":["`+v+`"]`)) },
		"/rules/0/exclude_paths/0": func(v string) string {
			return withRules(ruleWith("forbid_code", validRule["forbid_code"]+`,"exclude_paths":["`+v+`"]`))
		},
		"/rules/0/invalidated_by/0": func(v string) string {
			return withRules(ruleWith("order", validRule["order"]+`,"invalidated_by":["`+v+`"]`))
		},
	} {
		t.Run(pointer, func(t *testing.T) {
			for _, ok := range []string{".", "svc/api", "**/*.go", "a b/c", "..x", "x..", "a/..b/c", "db/migrations/**"} {
				accept(t, build(ok))
			}
			for _, bad := range []string{"/etc", "/", "../x", "a/../b", "a/..", ".."} {
				refuse(t, build(bad), pointer, ReasonShape)
			}
		})
	}
}

func TestSP9_FactsConstraints(t *testing.T) {
	fact := func(kind, mech, classes, value string) string {
		return fmt.Sprintf(`{"id":"f","kind":"%s","programs":["aws"],"classes":%s,"value":"%s","mechanism":"%s"}`, kind, classes, value, mech)
	}
	// Identity facts fire on authentication classes only (20.2.8, Facts;
	// TestVER62). Every kind is held to the list, the five the archive names
	// and the two it does not.
	good := []string{
		"cred.aws_profile_missing", // cred.*_missing
		"aws_profile_missing",      // *_profile_missing
		"sso_session_expired",      // *_expired
		"aws_sso_needs_human",      // *_sso_needs_human
		"cred.registry", "cred.kube_context", "mcp.auth", "mcp.token_expired",
	}
	authz := []string{"authz.denied", "s3.forbidden", "http.403", "authz.token_expired", "forbidden_missing", "cred.403_missing", "aws.authz_profile_missing"}
	notAuth := []string{"build.failed", "cred._missing", "x_expired_not", "cred.registry2", "mcp.auth.x", "cred.missing", "mcp.token", "profile_missing_x"}
	for i, kind := range FactKinds {
		mech := factMechanisms[i]
		if strings.Contains(mech, " ") { // see TestSP9_MechanismSpacesConflict
			mech = "profile"
		}
		pairOK := !strings.Contains(factMechanisms[i], " ")
		for _, c := range good {
			if !pairOK {
				continue
			}
			t.Run(kind+"/accepts "+c, func(t *testing.T) { accept(t, withFacts(fact(kind, mech, `["`+c+`"]`, "dev"))) })
		}
		if !pairOK {
			continue
		}
		for _, c := range authz {
			t.Run(kind+"/authz "+c, func(t *testing.T) {
				refuse(t, withFacts(fact(kind, mech, `["cred.aws_profile_missing","`+c+`"]`, "dev")), "/facts/0/classes/1", ReasonAuthzClass)
			})
		}
		for _, c := range notAuth {
			t.Run(kind+"/not authentication "+c, func(t *testing.T) {
				refuse(t, withFacts(fact(kind, mech, `["`+c+`"]`, "dev")), "/facts/0/classes/0", ReasonClassNotAllowed)
			})
		}
	}
	// A class is at most 64 bytes: "cred." + 51 + "_missing" is exactly 64.
	accept(t, withFacts(fact("aws_profile", "profile", `["cred.`+strings.Repeat("a", 51)+`_missing"]`, "dev")))
	refuse(t, withFacts(fact("aws_profile", "profile", `["cred.`+strings.Repeat("a", 52)+`_missing"]`, "dev")), "/facts/0/classes/0", ReasonChars)
	// A class is a short lower-case token: no wildcard, space or capital.
	for _, c := range []string{"cred.*_missing", "cred.a b_missing", "CRED.a_missing", "cred.é_missing", "_expired", strings.Repeat("a", 65) + "_expired"} {
		refuse(t, withFacts(fact("aws_profile", "profile", `["`+c+`"]`, "dev")), "/facts/0/classes/0", ReasonChars)
	}
	// Mechanism: the kind's own word is accepted, any other word of the closed
	// list is a mismatch, a word outside it is not in the list.
	for i, kind := range FactKinds[:5] {
		accept(t, withFacts(fact(kind, factMechanisms[i], `["mcp.auth"]`, "dev")))
		refuse(t, withFacts(fact(kind, factMechanisms[(i+1)%5], `["mcp.auth"]`, "dev")), "/facts/0/mechanism", ReasonMechanism)
	}
	refuse(t, withFacts(fact("aws_profile", "token", `["mcp.auth"]`, "dev")), "/facts/0/mechanism", ReasonEnum)
	refuse(t, withFacts(fact("aws_profile", "Profile", `["mcp.auth"]`, "dev")), "/facts/0/mechanism", ReasonEnum)
	// Value: an identifier in STEER's charset, no spaces, no URL.
	for _, v := range []string{"dev", "prod-eks", "arn:aws:eks:us-east-1:1:cluster/x", "a@b+c=d.e_f-g", strings.Repeat("a", MaxFactValueBytes)} {
		accept(t, withFacts(fact("aws_profile", "profile", `["mcp.auth"]`, v)))
	}
	for _, v := range []string{"my profile", "a,b", "a;b", "a$b", "a`b`", "é", "a\\\\b"} {
		refuse(t, withFacts(fact("aws_profile", "profile", `["mcp.auth"]`, v)), "/facts/0/value", ReasonChars)
	}
	for _, v := range []string{"https://x.example", "ftp://u:p@h/x", "a://b"} {
		refuse(t, withFacts(fact("aws_profile", "profile", `["mcp.auth"]`, v)), "/facts/0/value", ReasonShape)
	}
	refuse(t, withFacts(fact("aws_profile", "profile", `["mcp.auth"]`, "")), "/facts/0/value", ReasonEmpty)
	// A target id is held to the same charset and has no URL either.
	tgt := func(id string) string {
		return withFacts(strings.Replace(validFact, `"value"`, `"target":{"kind":"endpoint_host","id":"`+id+`"},"value"`, 1))
	}
	accept(t, tgt("registry.npmjs.org"))
	refuse(t, tgt("a b"), "/facts/0/target/id", ReasonChars)
	refuse(t, tgt("https://x"), "/facts/0/target/id", ReasonShape)
	// Required fields and shapes.
	for _, tc := range []struct {
		name, doc, pointer string
		reason             Reason
	}{
		{"programs missing", withFacts(strings.Replace(validFact, `"programs":["aws"],`, ``, 1)), "/facts/0/programs", ReasonRequired},
		{"programs empty", withFacts(strings.Replace(validFact, `["aws"]`, `[]`, 1)), "/facts/0/programs", ReasonEmpty},
		{"program charset", withFacts(strings.Replace(validFact, `["aws"]`, `["AWS"]`, 1)), "/facts/0/programs/0", ReasonID},
		{"classes missing", withFacts(strings.Replace(validFact, `"classes":["cred.aws_profile_missing"],`, ``, 1)), "/facts/0/classes", ReasonRequired},
		{"classes empty", withFacts(strings.Replace(validFact, `["cred.aws_profile_missing"]`, `[]`, 1)), "/facts/0/classes", ReasonEmpty},
		{"value missing", withFacts(strings.Replace(validFact, `"value":"dev",`, ``, 1)), "/facts/0/value", ReasonRequired},
		{"mechanism missing", withFacts(strings.Replace(validFact, `,"mechanism":"profile"`, ``, 1)), "/facts/0/mechanism", ReasonRequired},
		{"kind missing", withFacts(strings.Replace(validFact, `"kind":"aws_profile",`, ``, 1)), "/facts/0/kind", ReasonRequired},
		{"target kind missing", withFacts(strings.Replace(validFact, `"value"`, `"target":{"id":"x"},"value"`, 1)), "/facts/0/target/kind", ReasonRequired},
		{"target id missing", withFacts(strings.Replace(validFact, `"value"`, `"target":{"kind":"aws_region"},"value"`, 1)), "/facts/0/target/id", ReasonRequired},
		{"target is an object", withFacts(strings.Replace(validFact, `"value"`, `"target":"x","value"`, 1)), "/facts/0/target", ReasonType},
		{"why is a string", withFacts(strings.Replace(validFact, `"value"`, `"why":1,"value"`, 1)), "/facts/0/why", ReasonType},
		{"a fact is an object", withFacts(`[]`), "/facts/0", ReasonType},
	} {
		t.Run(tc.name, func(t *testing.T) { refuse(t, tc.doc, tc.pointer, tc.reason) })
	}
}

// TestSP9_MechanismSpacesConflict pins what the archive's two rules about a
// mechanism do together. The closed list ends "env var name | sign-in step",
// and the Facts section says value and mechanism are identifiers with no
// spaces (STEER's Mechanism charset has none either). Read narrowly, no
// mechanism of those two spellings can be written, so no env_var_name or
// sign_in_step fact parses; the question is Karthik's (the report names it).
func TestSP9_MechanismSpacesConflict(t *testing.T) {
	// ACCEPTED GAP: the archive's own spelling of the last two mechanisms is
	// refused for its space. The tool refuses every fact of these two kinds
	// until the archive spells the mechanism as an identifier.
	for _, tc := range []struct{ kind, mech string }{{"env_var_name", "env var name"}, {"sign_in_step", "sign-in step"}} {
		f := fmt.Sprintf(`{"id":"f","kind":"%s","programs":["gh"],"classes":["mcp.auth"],"value":"GH_TOKEN","mechanism":"%s"}`, tc.kind, tc.mech)
		refuse(t, withFacts(f), "/facts/0/mechanism", ReasonChars)
		// The identifier spellings are not in the archive's list.
		g := strings.Replace(f, tc.mech, strings.NewReplacer(" ", "_", "-", "_").Replace(tc.mech), 1)
		refuse(t, withFacts(g), "/facts/0/mechanism", ReasonEnum)
	}
}
