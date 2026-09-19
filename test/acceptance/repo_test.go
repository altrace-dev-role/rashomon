package acceptance

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestNoSourceFileIsGitIgnored exists because a `coverage.*` pattern once
// ignored internal/hook/coverage.go, which would have committed a tree that
// does not compile. A pattern that hides a source file is a bug in the
// repository, and this is the only place it can be caught before a commit.
func TestNoSourceFileIsGitIgnored(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}
	files, err := filepath.Glob(filepath.Join(moduleRoot, "*", "*", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	more, _ := filepath.Glob(filepath.Join(moduleRoot, "*", "*", "*", "*.go"))
	files = append(files, more...)
	if len(files) < 10 {
		t.Fatalf("found only %d source files; the glob is not looking in the right place", len(files))
	}

	cmd := exec.Command("git", "check-ignore", "--stdin")
	cmd.Dir = moduleRoot
	cmd.Stdin = strings.NewReader(strings.Join(files, "\n"))
	out, _ := cmd.Output() // exit 1 means nothing is ignored, which is the pass
	if ignored := strings.TrimSpace(string(out)); ignored != "" {
		t.Errorf("source files are git-ignored:\n%s", ignored)
	}
}

// schemaPath is the machine-checkable store contract: a JSON Schema with one
// definition per record type.
const schemaPath = "docs/store-schema.json"

// TestStoreSchemaMatchesTheAllowlists ties the document to the tests. The
// allowlists above are what H-13 enforces on the records themselves, and a
// schema that drifts from them is worse than no schema: it is a contract a
// consumer would be entitled to rely on that describes a store this program
// does not write.
func TestStoreSchemaMatchesTheAllowlists(t *testing.T) {
	schema := readSchema(t)
	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatalf("%s has no $defs", schemaPath)
	}

	for _, c := range []struct {
		def     string
		allowed []string
	}{
		{"declaration", declarationKeys},
		{"execution", executionKeys},
		{"terminal", terminalKeys},
		{"coverage", coverageKeys},
		{"gap", gapKeys},
	} {
		t.Run(c.def, func(t *testing.T) {
			def, ok := defs[c.def].(map[string]any)
			if !ok {
				t.Fatalf("$defs.%s is missing", c.def)
			}
			got := map[string]bool{}
			schemaKeyPaths(t, def, "", got)
			want := map[string]bool{}
			for _, k := range c.allowed {
				want[k] = true
			}
			for k := range got {
				if !want[k] {
					t.Errorf("the schema declares %q, which is not in the allowlist the tests enforce", k)
				}
			}
			for k := range want {
				if !got[k] {
					t.Errorf("the schema does not declare %q, which the records carry", k)
				}
			}
		})
	}
}

// TestStoreSchemaReasonsAreTheCodeReasons: the reason codes are a closed
// vocabulary, and the schema and the code have to agree in BOTH directions.
//
// It used to check one: every code reason appears in the schema. That misses
// the other failure, and the other failure had already happened --
// store.GapForgetHost was absent from the fixed `want` list here, so nothing
// noticed whether the schema knew about it. A one-directional check over a
// hand-maintained list is a check that decays exactly as fast as the list.
//
// The reverse direction matters for a different reason: a schema enum naming a
// reason the code never emits is a contract promising a value consumers will
// wait for forever.
func TestStoreSchemaReasonsAreTheCodeReasons(t *testing.T) {
	enums := map[string]bool{}
	collectReasonEnums(readSchema(t), "", enums)
	// A floor, not a count: the bidirectional comparison below is the real
	// check and it asserts exact equality. This only catches the walk finding
	// NOTHING, which is how a source-scanning test goes quietly green. The
	// number was 20 when this walk collected every enum in the file --
	// including type, outcome, phase and state -- and 20 became unreachable
	// the moment it was scoped to reasons alone.
	if len(enums) < 10 {
		t.Fatalf("only %d reason enum values found in %s; the walk is not finding them",
			len(enums), schemaPath)
	}

	want := map[string]bool{}
	for _, r := range store.Reasons() {
		want[r] = true
	}
	for _, r := range report.Reasons() {
		want[r] = true
	}
	// Every gap reason the store can write. GapForgetHost was the one missing
	// from this list, which is why it is spelled out rather than folded into a
	// helper that could omit one again silently.
	for _, r := range []string{store.GapForget, store.GapForgetHost, store.GapSizeCap} {
		want[r] = true
	}

	for reason := range want {
		if !enums[reason] {
			t.Errorf("the reason code %q appears in no reason enum in %s, so a record "+
				"carrying it fails the published contract", reason, schemaPath)
		}
	}
	for reason := range enums {
		if !want[reason] {
			t.Errorf("%s declares the reason %q, which no code path emits. A contract that "+
				"names a value the program never produces tells a consumer to wait for "+
				"something that will not arrive.", schemaPath, reason)
		}
	}
}

// collectReasonEnums gathers enum values from properties that carry REASON
// codes, and only those.
//
// Scoped deliberately: the schema is full of other closed vocabularies -- the
// record type discriminator, schema_version, outcome, host_source, coverage
// state -- and a bidirectional check over all of them would compare the reason
// vocabulary against values that were never meant to be reasons.
func collectReasonEnums(node any, key string, into map[string]bool) {
	switch v := node.(type) {
	case map[string]any:
		// A property named reason/reasons, or a named definition whose name
		// ends in _reason -- report_reason is declared once at the top level
		// and referenced, so a walk keyed only on property names finds the
		// record reasons and silently misses the report ones.
		if key == "reason" || key == "reasons" || strings.HasSuffix(key, "_reason") {
			for _, e := range toAnySlice(v["enum"]) {
				if s, ok := e.(string); ok {
					into[s] = true
				}
			}
			// reasons is usually an array of strings with the enum on items.
			if items, ok := v["items"].(map[string]any); ok {
				for _, e := range toAnySlice(items["enum"]) {
					if s, ok := e.(string); ok {
						into[s] = true
					}
				}
			}
		}
		for k, child := range v {
			collectReasonEnums(child, k, into)
		}
	case []any:
		for _, child := range v {
			collectReasonEnums(child, key, into)
		}
	}
}

func readSchema(t *testing.T) map[string]any {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(moduleRoot, schemaPath))
	if err != nil {
		t.Fatalf("reading the store schema: %v", err)
	}
	var schema map[string]any
	if err := json.Unmarshal(body, &schema); err != nil {
		t.Fatalf("%s is not JSON: %v", schemaPath, err)
	}
	if got, _ := schema["$schema"].(string); !strings.Contains(got, "2020-12") {
		t.Errorf("$schema is %q, want draft 2020-12", got)
	}
	if got, _ := schema["description"].(string); !strings.Contains(got, "records.ndjson") {
		t.Errorf("the top-level description does not state the file layout: %q", got)
	}
	return schema
}

// schemaKeyPaths flattens a definition's properties into dotted paths, as
// keyPaths flattens a record, and asserts on the way that every property is
// required and that no object accepts one that is not declared.
func schemaKeyPaths(t *testing.T, def map[string]any, prefix string, into map[string]bool) {
	t.Helper()
	where := prefix
	if where == "" {
		where = "the record"
	}
	if def["additionalProperties"] != false {
		t.Errorf("%s does not set additionalProperties: false, so the schema would accept a key nobody declared", where)
	}

	props, ok := def["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s declares no properties", where)
	}
	names, ok := def["required"].([]any)
	if !ok {
		t.Fatalf("%s has no required list", where)
	}
	required := map[string]bool{}
	for _, name := range names {
		key, ok := name.(string)
		if !ok {
			t.Fatalf("%s requires %v, which is not a key", where, name)
		}
		required[key] = true
	}
	// A field introduced by a later schema version is required AT THAT VERSION,
	// through an `if schema_version == N then required` in allOf, and must NOT
	// be in the top-level required list -- every record already on disk lacks
	// it and would stop validating against its own published contract.
	//
	// So the convention this check enforces is "no key is optional", not "every
	// key is in one list". A version-gated key is not optional: a v3 record
	// without it is rejected. Counting it as required here is what lets the two
	// rules coexist.
	for _, entry := range toAnySlice(def["allOf"]) {
		e, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		then, _ := e["then"].(map[string]any)
		if then == nil {
			continue
		}
		for key := range toStringSet(then["required"]) {
			required[key] = true
		}
	}
	for key, raw := range props {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		into[path] = true
		if !required[key] {
			t.Errorf("%s is not in required; an absent key and a null one would be two spellings of one fact", path)
		}
		child, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, nested := child["properties"]; nested {
			schemaKeyPaths(t, child, path, into)
		}
	}
	for key := range required {
		if _, ok := props[key]; !ok {
			t.Errorf("%s requires %q, which it does not declare", where, key)
		}
	}
}

// collectEnums gathers every enum value anywhere in the schema, so that the
// assertion holds however the vocabularies are arranged across definitions.
func collectEnums(node any, into map[string]bool) {
	switch v := node.(type) {
	case map[string]any:
		if values, ok := v["enum"].([]any); ok {
			for _, value := range values {
				if s, ok := value.(string); ok {
					into[s] = true
				}
			}
		}
		for _, child := range v {
			collectEnums(child, into)
		}
	case []any:
		for _, child := range v {
			collectEnums(child, into)
		}
	}
}

// TestStoreSchemaLabelsAreTheCodeLabels: file_label is a closed vocabulary
// like the reason codes, and it needs a check of its own.
//
// collectReasonEnums is scoped to keys named reason, reasons or *_reason --
// deliberately, so that a bidirectional check does not compare reasons
// against vocabularies that were never reasons. The cost of that scoping is
// that every OTHER closed vocabulary needs its own walk, or it drifts from
// the schema in silence and the drift surfaces as a record in the field that
// the published contract refuses.
//
// This one is bidirectional, which the reason check is not: a label the code
// can emit and the schema forbids is a record that fails validation, and a
// label the schema offers and the code cannot emit is a promise to a reader
// that nothing will ever keep.
func TestStoreSchemaLabelsAreTheCodeLabels(t *testing.T) {
	inSchema := map[string]bool{}
	collectNamedEnum(readSchema(t), "", "file_label", inSchema)
	if len(inSchema) == 0 {
		t.Fatalf("no file_label enum found in %s; the walk is not finding it", schemaPath)
	}

	inCode := map[string]bool{}
	for _, l := range shape.Labels() {
		inCode[l] = true
		if !inSchema[l] {
			t.Errorf("shape.Labels() carries %q, which appears in no file_label enum in %s", l, schemaPath)
		}
	}
	for l := range inSchema {
		if !inCode[l] {
			t.Errorf("%s offers the label %q, which shape.Labels() cannot emit", schemaPath, l)
		}
	}
}

// collectNamedEnum gathers the string members of the enum on every property
// with the given name. Null members are skipped: the nullability is carried
// by the type, and Labels() is a list of labels rather than of states.
func collectNamedEnum(node any, key, want string, into map[string]bool) {
	switch v := node.(type) {
	case map[string]any:
		if key == want {
			for _, e := range toAnySlice(v["enum"]) {
				if s, ok := e.(string); ok {
					into[s] = true
				}
			}
		}
		for k, child := range v {
			collectNamedEnum(child, k, want, into)
		}
	case []any:
		for _, child := range v {
			collectNamedEnum(child, key, want, into)
		}
	}
}
