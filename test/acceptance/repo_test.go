package acceptance

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/report"
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
// vocabulary, and a schema listing a subset of it would refuse a record this
// program writes.
func TestStoreSchemaReasonsAreTheCodeReasons(t *testing.T) {
	enums := map[string]bool{}
	collectEnums(readSchema(t), enums)
	if len(enums) < 20 {
		t.Fatalf("only %d enum values found in %s; the walk is not finding them", len(enums), schemaPath)
	}

	var want []string
	want = append(want, store.Reasons()...)
	want = append(want, report.Reasons()...)
	want = append(want, store.GapForget, store.GapSizeCap)
	for _, reason := range want {
		if !enums[reason] {
			t.Errorf("the reason code %q appears in no enum in %s", reason, schemaPath)
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
