package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// B2 -- schema 3 reserves fields a later layer will populate.
//
// This build writes schema 3. Among its fields are three a later rule-match
// layer will populate: which SOURCE a declared host came from, and a
// rule-match object on declarations and executions. Reserving them now means
// that layer adds behaviour rather than a second schema bump.
//
// The two hazards this guards are opposite. Reserve too little and that layer
// has to bump the schema again. Reserve too strictly -- put the new
// fields in the top-level `required` -- and every record already on disk
// stops validating against its own published contract, which is worse than
// either, because the contract is the thing consumers are told to rely on.

// TestAcceptsAdmitsSchema3 is the data-loss guard, and it is the same bug the
// reader already carries a comment about: `Accepts` used to be an equality
// against SchemaVersion, so the moment the writer moved to v2 every v1 record
// on disk was skipped and the report rendered empty with nothing saying why.
// This build writes v4, and Accepts must keep admitting 1, 2 and 3 so that
// older stores still read.
func TestAcceptsAdmitsSchema3(t *testing.T) {
	for _, v := range []int{1, 2, 3, 4} {
		if !store.Accepts(v) {
			t.Errorf("store.Accepts(%d) is false. Records at that version are SKIPPED, not "+
				"reported as unreadable, so the symptom is an empty report and no reason "+
				"for it.", v)
		}
	}
	if store.Accepts(5) {
		t.Error("store.Accepts(5) is true; a reader must not claim to understand a shape " +
			"that does not exist yet")
	}
}

// TestSchema3FieldsAreDeclaredButNotGloballyRequired is the second hazard
// stated directly. The new fields must be DECLARED, so a v3 record carrying
// them validates and `additionalProperties: false` does not reject them; and
// they must not be in the top-level `required`, or every v1 and v2 record ever
// written fails the contract.
func TestSchema3FieldsAreDeclaredButNotGloballyRequired(t *testing.T) {
	defs := schemaDefs(t)

	for _, c := range []struct {
		def    string
		fields []string
	}{
		{"declaration", []string{"host_source", "rule_match", "cwd_digest"}},
		{"execution", []string{"rule_match", "backgrounded"}},
	} {
		t.Run(c.def, func(t *testing.T) {
			def, ok := defs[c.def].(map[string]any)
			if !ok {
				t.Fatalf("$defs.%s missing", c.def)
			}
			props, _ := def["properties"].(map[string]any)
			required := toStringSet(def["required"])

			for _, f := range c.fields {
				if _, declared := props[f]; !declared {
					t.Errorf("%s does not declare %q. With additionalProperties false, a v3 "+
						"record carrying it is REJECTED by the published contract.", c.def, f)
				}
				if required[f] {
					t.Errorf("%s lists %q in the top-level required. Every v1 and v2 record "+
						"already on disk lacks it, so they all stop validating against the "+
						"contract they were written under.", c.def, f)
				}
			}
		})
	}
}

// TestSchema3IsRequiredOnlyAtVersion3 pins the mechanism, not just the
// absence: there has to be a conditional keyed on schema_version that makes
// the new fields required when, and only when, the record says it is v3.
func TestSchema3IsRequiredOnlyAtVersion3(t *testing.T) {
	defs := schemaDefs(t)

	for _, c := range []struct {
		def   string
		field string
	}{
		{"declaration", "host_source"},
		{"declaration", "rule_match"},
		{"declaration", "cwd_digest"},
		{"declaration", "shape.may_write"},
		{"execution", "rule_match"},
		{"execution", "backgrounded"},
	} {
		t.Run(c.def+"."+c.field, func(t *testing.T) {
			def := defs[c.def].(map[string]any)
			all, ok := def["allOf"].([]any)
			if !ok || len(all) == 0 {
				t.Fatalf("$defs.%s has no allOf; nothing makes the schema-3 fields required "+
					"at v3, so a v3 record could omit them and still validate", c.def)
			}
			var found bool
			for _, entry := range all {
				e, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				cond, _ := e["if"].(map[string]any)
				then, _ := e["then"].(map[string]any)
				if cond == nil || then == nil {
					continue
				}
				// if: properties.schema_version.const == 3
				cp, _ := cond["properties"].(map[string]any)
				sv, _ := cp["schema_version"].(map[string]any)
				if sv == nil {
					continue
				}
				if n, ok := sv["const"].(float64); !ok || int(n) != 3 {
					continue
				}
				if toStringSet(then["required"])[c.field] {
					found = true
				}
				// A key inside the shape object is gated through
				// then.properties.shape.required.
				if parent, key, ok := strings.Cut(c.field, "."); ok {
					props, _ := then["properties"].(map[string]any)
					child, _ := props[parent].(map[string]any)
					if toStringSet(child["required"])[key] {
						found = true
					}
				}
			}
			if !found {
				t.Errorf("no `if schema_version == 3 then required: [... %q ...]` in $defs.%s. "+
					"Without it the field is optional at every version, and a v3 record that "+
					"forgot it passes the contract.", c.field, c.def)
			}
		})
	}
}

// TestSchemaVersionEnumAdmits3. The enum is the other half: a record declaring
// 3 must be allowed to say so.
func TestSchemaVersionEnumAdmits3(t *testing.T) {
	defs := schemaDefs(t)
	for _, name := range []string{"declaration", "execution", "terminal", "coverage", "gap"} {
		def, ok := defs[name].(map[string]any)
		if !ok {
			continue
		}
		props, _ := def["properties"].(map[string]any)
		sv, _ := props["schema_version"].(map[string]any)
		if sv == nil {
			t.Errorf("$defs.%s has no schema_version property", name)
			continue
		}
		vals := map[int]bool{}
		for _, v := range toAnySlice(sv["enum"]) {
			if n, ok := v.(float64); ok {
				vals[int(n)] = true
			}
		}
		for _, want := range []int{1, 2, 3, 4} {
			if !vals[want] {
				t.Errorf("$defs.%s.schema_version does not admit %d; a record at that version "+
					"fails the contract even though the reader accepts it", name, want)
			}
		}
	}
}

func schemaDefs(t *testing.T) map[string]any {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(moduleRoot, schemaPath))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(body, &schema); err != nil {
		t.Fatalf("%s is not valid JSON: %v", schemaPath, err)
	}
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("%s has no $defs", schemaPath)
	}
	return defs
}

func toStringSet(v any) map[string]bool {
	out := map[string]bool{}
	for _, x := range toAnySlice(v) {
		if s, ok := x.(string); ok {
			out[s] = true
		}
	}
	return out
}

func toAnySlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

// TestSchema3_WrittenRecordsMeetTheVersion3Gate: this build writes schema 4,
// additive over 3, and every record it writes carries each key the published
// v3 gate requires, and the v4 gate's as well.
// The version is compared against the literal, not against
// store.SchemaVersion, which would compare the writer with itself. Break:
// write 2 while the vocabulary is 3's (the test class, the test-file label),
// and an older reader buckets the new words as unknown; or drop a gated key,
// and a validator using the published schema rejects the record.
func TestSchema3_WrittenRecordsMeetTheVersion3Gate(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))

	defs := schemaDefs(t)
	for _, c := range []struct {
		def  string
		recs []record
	}{
		{"declaration", e.declarations(testSession)},
		{"execution", e.executions(testSession)},
	} {
		gated := versionGate(defs[c.def].(map[string]any), 3)
		if len(gated) == 0 {
			t.Fatalf("$defs.%s has no schema 3 gate", c.def)
		}
		v4 := versionGate(defs[c.def].(map[string]any), 4)
		if len(v4) < len(gated) {
			t.Fatalf("$defs.%s's schema 4 gate requires %d keys, fewer than schema 3's %d: v4 is additive", c.def, len(v4), len(gated))
		}
		gated = append(gated, v4...)
		if len(c.recs) == 0 {
			t.Fatalf("no %s record written", c.def)
		}
		for _, r := range c.recs {
			if got := r.fields["schema_version"]; got != float64(4) {
				t.Errorf("%s written at schema_version %v, want 4", c.def, got)
			}
			for _, k := range gated {
				if _, ok := nested(r, k); !ok {
					t.Errorf("%s lacks %q, which the schema 3 or 4 gate requires", c.def, k)
				}
			}
		}
	}
}

// versionGate lists the dotted keys a definition requires at schema_version
// v: its gate's `then.required`, and each `then.properties.<key>.required`
// as `<key>.<nested>` -- how a key inside the shape object is gated, since
// the shape carries no version of its own.
func versionGate(def map[string]any, v int) []string {
	var out []string
	for _, entry := range toAnySlice(def["allOf"]) {
		e, _ := entry.(map[string]any)
		cond, _ := e["if"].(map[string]any)
		then, _ := e["then"].(map[string]any)
		cp, _ := cond["properties"].(map[string]any)
		sv, _ := cp["schema_version"].(map[string]any)
		if n, ok := sv["const"].(float64); !ok || int(n) != v || then == nil {
			continue
		}
		for k := range toStringSet(then["required"]) {
			out = append(out, k)
		}
		props, _ := then["properties"].(map[string]any)
		for parent, raw := range props {
			child, _ := raw.(map[string]any)
			for k := range toStringSet(child["required"]) {
				out = append(out, parent+"."+k)
			}
		}
	}
	return out
}
