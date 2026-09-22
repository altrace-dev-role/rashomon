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
// vocabulary, and the schema and the code have to agree in BOTH directions --
// per vocabulary, not pooled into one set.
//
// There are two deliberately separate vocabularies. store.Reasons() names
// reasons a RECORD carries: a hook writes them into a coverage or terminal
// record at the moment it runs, so they belong in those records' own
// `reason` enums. report.Reasons() names reasons the REPORT derives by
// looking at the whole run after the fact -- no record a hook writes ever
// carries one, which is exactly why they live in their own report_reason
// def rather than in a record's enum.
//
// It used to check one union: every code reason (from both vocabularies,
// pooled) appears somewhere in the schema (any enum named reason/reasons or
// *_reason), and vice versa. That passes a reason placed in the WRONG
// vocabulary -- present in store.Reasons() and present somewhere in the
// schema, just not in the record enum it claims to describe -- which is
// exactly the shape of the store.GapForgetHost gap this test's history
// already names once. Comparing each vocabulary against its own part of the
// schema is what catches that.
func TestStoreSchemaReasonsAreTheCodeReasons(t *testing.T) {
	defs := schemaDefs(t)

	// A record's reason lives on coverage.reason or terminal.reason.
	// terminal.reason is a deliberate subset -- a Terminal only ever closes
	// with signal/lock_timeout/internal_error -- so the two enums are pooled
	// before comparing against store.Reasons() in full; checking terminal's
	// enum alone against the whole list would fail on a subset that is
	// correct by design.
	recordReasons := map[string]bool{}
	for _, def := range []string{"coverage", "terminal"} {
		for r := range reasonEnum(t, defs, def) {
			recordReasons[r] = true
		}
	}
	assertReasonVocabulary(t, "store.Reasons() vs. the record reason enums (coverage.reason, terminal.reason)",
		store.Reasons(), recordReasons)

	assertReasonVocabulary(t, "report.Reasons() vs. report_reason",
		report.Reasons(), reasonEnum(t, defs, "report_reason"))

	// Every gap reason the store can write. GapForgetHost was once missing
	// from this exact list, which is why it is spelled out rather than
	// folded into a helper that could omit one again silently.
	assertReasonVocabulary(t, "gap reasons vs. gap.reason",
		[]string{store.GapForget, store.GapForgetHost, store.GapSizeCap}, reasonEnum(t, defs, "gap"))
}

// reasonEnum reads the string enum values naming one reason vocabulary in
// the schema: the `reason` property's enum for a record def (coverage,
// terminal, gap), or the def's own enum for report_reason, which is declared
// at the top level rather than nested under a record because no record
// carries it. null is never a member of either shape, so it is dropped by
// construction -- a JSON null fails the string type assertion below.
func reasonEnum(t *testing.T, defs map[string]any, def string) map[string]bool {
	t.Helper()
	d, ok := defs[def].(map[string]any)
	if !ok {
		t.Fatalf("$defs.%s is missing", def)
	}
	enum := toAnySlice(d["enum"])
	if props, ok := d["properties"].(map[string]any); ok {
		if r, ok := props["reason"].(map[string]any); ok {
			enum = toAnySlice(r["enum"])
		}
	}
	out := map[string]bool{}
	for _, e := range enum {
		if s, ok := e.(string); ok {
			out[s] = true
		}
	}
	if len(out) == 0 {
		t.Fatalf("$defs.%s names no reason enum values; the lookup is not finding them", def)
	}
	return out
}

// TestStoreSchemaHasNoUncheckedReasonVocabulary: every reason vocabulary the
// schema declares is one TestStoreSchemaReasonsAreTheCodeReasons compares.
//
// That test looks its vocabularies up by def. The walk it replaced found them
// by what they were called -- any reason or reasons property, any *_reason
// def -- so a vocabulary added later was checked without anyone remembering to
// check it. A lookup checks only what it was told about: a new record type
// with its own reason enum would be published unchecked, in either direction.
// This keeps the discovery, and fails on anything the comparison misses.
func TestStoreSchemaHasNoUncheckedReasonVocabulary(t *testing.T) {
	compared := map[string]bool{"coverage": true, "terminal": true, "gap": true, "report_reason": true}

	found := map[string]bool{}
	for name, def := range schemaDefs(t) {
		collectReasonDefs(def, name, name, found)
	}
	for name := range found {
		if !compared[name] {
			t.Errorf("$defs.%s declares a reason vocabulary that TestStoreSchemaReasonsAreTheCodeReasons "+
				"does not compare against the code; add the comparison, then add it here", name)
		}
	}
	for name := range compared {
		if !found[name] {
			t.Errorf("$defs.%s is compared as a reason vocabulary but declares none; "+
				"the discovery below is not finding it", name)
		}
	}
}

// collectReasonDefs records def when a reason-named key occurs anywhere under
// it: a reason or reasons property, or a def whose own name ends in _reason.
func collectReasonDefs(node any, def, key string, into map[string]bool) {
	switch v := node.(type) {
	case map[string]any:
		if key == "reason" || key == "reasons" || strings.HasSuffix(key, "_reason") {
			into[def] = true
		}
		for k, child := range v {
			collectReasonDefs(child, def, k, into)
		}
	case []any:
		for _, child := range v {
			collectReasonDefs(child, def, key, into)
		}
	}
}

// assertReasonVocabulary checks that a code-side reason list and the schema
// enum meant to publish it name exactly the same set. Kept as one helper
// used three times, rather than three inline loops, so a fix to the
// direction or the wording lands in every caller at once.
func assertReasonVocabulary(t *testing.T, label string, code []string, schemaEnum map[string]bool) {
	t.Helper()
	want := map[string]bool{}
	for _, r := range code {
		want[r] = true
	}
	for r := range want {
		if !schemaEnum[r] {
			t.Errorf("%s: %q is in the code's list but not in the schema's enum, so a value "+
				"the code emits fails the published contract", label, r)
		}
	}
	for r := range schemaEnum {
		if !want[r] {
			t.Errorf("%s: %q is in the schema's enum but the code never emits it there. A "+
				"contract that names a value the program never produces tells a consumer "+
				"to wait for something that will not arrive.", label, r)
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
