package acceptance

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

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
		{"forgotten_host", forgottenHostKeys},
		{"spec_event", specEventKeys},
		{"after_hit", afterHitKeys},
	} {
		t.Run(c.def, func(t *testing.T) {
			def, ok := defs[c.def].(map[string]any)
			if !ok {
				t.Fatalf("$defs.%s is missing", c.def)
			}
			got := map[string]bool{}
			schemaKeyPaths(t, def, "", nil, got)
			want := map[string]bool{}
			for _, k := range c.allowed {
				want[k] = true
			}
			if c.def == "declaration" {
				for _, k := range omittedWhenFalseKeys {
					want[k] = true
				}
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

// TestStoreSchemaHasNoUncheckedReasonVocabulary: every reason vocabulary the
// schema declares, anywhere in the document, sits at a location
// TestStoreSchemaEnumsAreTheirCodeLists compares -- and every location it
// compares is one the schema declares.
//
// That test compares at fixed places: one row per reason enum (coverage,
// terminal, gap and spec_event's properties.reason, and report_reason's own
// enum), and
// nothing else -- not a second *_reason property beside reason, not a
// reasons array's items.enum, not anything outside $defs. A fixed lookup
// checks only what it was told about. This test restores the discovery: it
// records the JSON-pointer path of every key named reason or reasons or
// ending in _reason across the whole schema, and requires that set to equal
// the five paths below exactly, less a reserved path that declares no enum
// (reservedReasonPaths). A new reason key fails here until a row compares it
// and its path is added to reasonVocabularyPaths; a row deleted there fails
// TestStoreSchemaReasonEnumsAreTheirCodeLists, which requires the reason
// rows' paths to be these five.
//
// Its only signal is the key's name: a vocabulary published under a key that
// is not reason-named is invisible to it, as it was to the walk it replaced.
func TestStoreSchemaHasNoUncheckedReasonVocabulary(t *testing.T) {
	unchecked, missing := reasonVocabularyMismatches(readSchema(t))
	for _, p := range unchecked {
		t.Errorf("%s declares a reason vocabulary that TestStoreSchemaEnumsAreTheirCodeLists "+
			"does not compare, so it is published without being compared against the code; "+
			"add the row, then add the path to reasonVocabularyPaths", p)
	}
	for _, p := range missing {
		t.Errorf("%s is compared by TestStoreSchemaEnumsAreTheirCodeLists as a reason vocabulary, "+
			"but the schema has no reason-named key there", p)
	}
}

// reasonVocabularyPaths is the set of JSON-pointer paths
// TestStoreSchemaEnumsAreTheirCodeLists compares, one per reason vocabulary.
// TestStoreSchemaReasonEnumsAreTheirCodeLists requires the reason rows'
// paths to equal this set, so the two change together.
var reasonVocabularyPaths = map[string]bool{
	"/$defs/coverage/properties/reason":   true,
	"/$defs/terminal/properties/reason":   true,
	"/$defs/gap/properties/reason":        true,
	"/$defs/report_reason":                true,
	"/$defs/spec_event/properties/reason": true,
}

// reservedReasonPaths are reason-named keys of schema 5 that are not reason
// codes: why an execution's command was stored (differs |
// declaration_not_in_tail), which no writer fills yet, and why a gate's
// output went unread, which the post hook fills (card GATEOUT-WIRE1) from
// three of its four words, verify_off having no producer.
// Each is published as a nullable string with no enum, so the schema states
// no vocabulary the code could disagree with, and the tripwire passes over
// it only while that holds: the change that adds the enum adds the
// comparison and moves the path to reasonVocabularyPaths. A reserved path the
// schema lacks is not missing, since nothing compares it.
var reservedReasonPaths = map[string]bool{
	"/$defs/execution/properties/executed_command_reason": true,
	"/$defs/gate_output/properties/reason":                true,
}

// reasonVocabularyMismatches returns, sorted, the reason-named paths in schema
// that reasonVocabularyPaths does not list (unchecked) and the listed paths
// the schema does not have (missing). It takes the decoded schema rather than
// reading the file so the tripwire's own test can hand it a mutated copy.
func reasonVocabularyMismatches(schema map[string]any) (unchecked, missing []string) {
	found := map[string]bool{}
	collectReasonPaths(schema, "", found)
	for p, vocabulary := range found {
		if reasonConstraintPaths[p] && declaresNoReason(pointerNode(schema, p)) {
			continue
		}
		if reasonVocabularyPaths[p] || (reservedReasonPaths[p] && !vocabulary) {
			continue
		}
		unchecked = append(unchecked, p)
	}
	for p := range reasonVocabularyPaths {
		if _, ok := found[p]; !ok {
			missing = append(missing, p)
		}
	}
	sort.Strings(unchecked)
	sort.Strings(missing)
	return unchecked, missing
}

// collectReasonPaths records the JSON-pointer path (RFC 6901) of every object
// key under node named reason or reasons or ending in _reason, whatever its
// value, and descends into every object and array; the path maps to whether
// the key's value declares an enum anywhere inside it. A string that merely
// equals "reason" -- an entry in a required list -- is a value, not a key, and
// is not recorded.
func collectReasonPaths(node any, path string, into map[string]bool) {
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			p := path + "/" + strings.NewReplacer("~", "~0", "/", "~1").Replace(k)
			if k == "reason" || k == "reasons" || strings.HasSuffix(k, "_reason") {
				into[p] = declaresEnum(child)
			}
			collectReasonPaths(child, p, into)
		}
	case []any:
		for i, child := range v {
			collectReasonPaths(child, path+"/"+strconv.Itoa(i), into)
		}
	}
}

// declaresEnum reports whether node, or anything inside it, has an enum or a
// const key: a const is a vocabulary of one value.
func declaresEnum(node any) bool {
	switch v := node.(type) {
	case map[string]any:
		if _, ok := v["enum"]; ok {
			return true
		}
		if _, ok := v["const"]; ok {
			return true
		}
		for _, child := range v {
			if declaresEnum(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if declaresEnum(child) {
				return true
			}
		}
	}
	return false
}

// TestRubricERRORS5_TheReasonTripwireCatchesWhatTheComparisonSkips mutates a copy of
// the real schema -- never the file -- in each of the ways a reason vocabulary
// can be added where TestStoreSchemaEnumsAreTheirCodeLists does not look, and
// requires the tripwire to name the new location. The last case goes the other
// way: a vocabulary the comparison reads disappears.
func TestRubricERRORS5_TheReasonTripwireCatchesWhatTheComparisonSkips(t *testing.T) {
	cases := []struct {
		name                       string
		mutate                     func(schema map[string]any)
		wantUnchecked, wantMissing string
	}{
		{
			name: "a second reason property on a compared record",
			mutate: func(s map[string]any) {
				props := schemaObject(t, s, "$defs", "coverage", "properties")
				props["pause_reason"] = map[string]any{"type": []any{"string", "null"}, "enum": []any{nil, "never_emitted_value"}}
			},
			wantUnchecked: "/$defs/coverage/properties/pause_reason",
		},
		{
			name: "a reasons array whose vocabulary is items.enum",
			mutate: func(s map[string]any) {
				props := schemaObject(t, s, "$defs", "gap", "properties")
				props["reasons"] = map[string]any{"type": "array", "items": map[string]any{"enum": []any{"never_emitted_value"}}}
			},
			wantUnchecked: "/$defs/gap/properties/reasons",
		},
		{
			name: "a reason vocabulary outside $defs",
			mutate: func(s map[string]any) {
				s["properties"] = map[string]any{"top_reason": map[string]any{"enum": []any{"never_emitted_value"}}}
			},
			wantUnchecked: "/properties/top_reason",
		},
		{
			name: "the coverage constraint path declares values",
			mutate: func(s map[string]any) {
				schemaObject(t, s, "$defs", "coverage")["allOf"] = []any{map[string]any{
					"then": map[string]any{"properties": map[string]any{
						"reason": map[string]any{"type": "string", "enum": []any{"never_emitted_value"}},
					}},
				}}
			},
			wantUnchecked: "/$defs/coverage/allOf/0/then/properties/reason",
		},
		{
			name: "the verified branch pins reason to a value, not to null",
			mutate: func(s map[string]any) {
				schemaObject(t, s, "$defs", "coverage")["allOf"] = []any{map[string]any{
					"else": map[string]any{"properties": map[string]any{
						"reason": map[string]any{"const": "never_emitted_value"},
					}},
				}}
			},
			wantUnchecked: "/$defs/coverage/allOf/0/else/properties/reason",
		},
		{
			name: "a reserved reason key that gains a vocabulary",
			mutate: func(s map[string]any) {
				props := schemaObject(t, s, "$defs", "execution", "properties")
				props["executed_command_reason"] = map[string]any{"type": []any{"string", "null"}, "enum": []any{nil, "differs"}}
			},
			wantUnchecked: "/$defs/execution/properties/executed_command_reason",
		},
		{
			name: "a reserved reason key pinned to one value",
			mutate: func(s map[string]any) {
				props := schemaObject(t, s, "$defs", "execution", "properties")
				props["executed_command_reason"] = map[string]any{"const": "differs"}
			},
			wantUnchecked: "/$defs/execution/properties/executed_command_reason",
		},
		{
			name: "a compared vocabulary removed",
			mutate: func(s map[string]any) {
				delete(schemaObject(t, s, "$defs", "terminal", "properties"), "reason")
			},
			wantMissing: "/$defs/terminal/properties/reason",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			schema := copySchema(t, readSchema(t))
			c.mutate(schema)
			unchecked, missing := reasonVocabularyMismatches(schema)
			if !containsString(unchecked, c.wantUnchecked) || !containsString(missing, c.wantMissing) {
				t.Errorf("the tripwire reported unchecked %q and missing %q; want %q among the unchecked and %q among the missing",
					unchecked, missing, c.wantUnchecked, c.wantMissing)
			}
		})
	}
}

// containsString reports whether want is in list; an empty want is trivially
// present, so a case that expects nothing in one direction does not check it.
func containsString(list []string, want string) bool {
	if want == "" {
		return true
	}
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// copySchema deep-copies a decoded schema by round-tripping it through JSON,
// so a mutation of the copy can never reach readSchema's next caller.
func copySchema(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	body, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// schemaObject walks keys down from node and returns the object there, failing
// the test if any step is not an object.
func schemaObject(t *testing.T, node map[string]any, keys ...string) map[string]any {
	t.Helper()
	for _, k := range keys {
		next, ok := node[k].(map[string]any)
		if !ok {
			t.Fatalf("the schema has no object at %q", k)
		}
		node = next
	}
	return node
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
//
// gated is the keys the parent's own version gate requires of this object
// (`then.properties.<key>.required`): a key inside the shape object is gated
// that way, since the shape carries no version of its own.
func schemaKeyPaths(t *testing.T, def map[string]any, prefix string, gated map[string]bool, into map[string]bool) {
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
	nestedGated := map[string]map[string]bool{}
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
		thenProps, _ := then["properties"].(map[string]any)
		for key, raw := range thenProps {
			child, _ := raw.(map[string]any)
			for k := range toStringSet(child["required"]) {
				if nestedGated[key] == nil {
					nestedGated[key] = map[string]bool{}
				}
				nestedGated[key][k] = true
			}
		}
	}
	for key := range gated {
		required[key] = true
	}
	for key, raw := range props {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		into[path] = true
		if !required[key] && !(prefix == "" && slices.Contains(omittedWhenFalseKeys, key)) {
			t.Errorf("%s is not in required; an absent key and a null one would be two spellings of one fact", path)
		}
		child, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, nested := child["properties"]; nested {
			schemaKeyPaths(t, child, path, nestedGated[key], into)
		}
	}
	for key := range required {
		if _, ok := props[key]; !ok {
			t.Errorf("%s requires %q, which it does not declare", where, key)
		}
	}
}

// TestStoreSchemaLabelsAreTheCodeLabels: file_label is a closed vocabulary
// like the reason codes, and it needs a check of its own.
//
// The reason tripwire (collectReasonPaths) finds vocabularies by keys named
// reason, reasons or *_reason -- deliberately, so that the reason comparison
// is not run against vocabularies that were never reasons. The cost of that
// scoping is that every OTHER closed vocabulary needs its own check, or it
// drifts from the schema in silence and the drift surfaces as a record in
// the field that the published contract refuses.
//
// This one is bidirectional, as the reason check is: a label the code can
// emit and the schema forbids is a record that fails validation, and a label
// the schema offers and the code cannot emit is a promise to a reader that
// nothing will ever keep.
func TestStoreSchemaLabelsAreTheCodeLabels(t *testing.T) {
	assertNamedEnums(t, "file_label", []string{"/$defs/declaration/properties/file_label"},
		"shape.Labels()", shape.Labels(), true)
}

// TestStoreSchemaVerbClassesAreTheCodeVerbClasses: verb_class is the third
// closed vocabulary on a declaration, and until the test class it had no walk
// of its own -- the enum and the constants agreed by luck. Bidirectional for
// the reason the label check is: a class the code emits and the schema
// forbids is a record that fails validation, and a class the schema offers
// that the code cannot emit is a promise nothing keeps.
func TestStoreSchemaVerbClassesAreTheCodeVerbClasses(t *testing.T) {
	assertNamedEnums(t, "verb_class", []string{"/$defs/declaration/properties/shape/properties/verb_class"},
		"shape.VerbClasses()", shape.VerbClasses(), false)
}

// TestStoreSchemaMaskedKindsAreTheCodeMaskedKinds: status_masked is a closed
// vocabulary too, checked both ways for the reason verb_class is, and per
// enum: the declaration's shape and the execution each publish one, and a
// pooled set would let one of them lose a word the other still has.
func TestStoreSchemaMaskedKindsAreTheCodeMaskedKinds(t *testing.T) {
	assertNamedEnums(t, "status_masked", []string{
		"/$defs/declaration/properties/shape/properties/status_masked",
		"/$defs/execution/properties/status_masked",
	}, "shape.MaskedKinds()", shape.MaskedKinds(), true)
}

// TestStoreSchemaRunnerKindsAreTheCodeRunnerKinds: runner_kind is a closed
// vocabulary too, checked both ways for the reason verb_class is, and per
// enum, for the reason status_masked is.
func TestStoreSchemaRunnerKindsAreTheCodeRunnerKinds(t *testing.T) {
	assertNamedEnums(t, "runner_kind", []string{
		"/$defs/declaration/properties/shape/properties/runner_kind",
		"/$defs/execution/properties/runner_kind",
	}, "shape.RunnerKinds()", shape.RunnerKinds(), true)
}

// assertNamedEnums checks every property of the schema named name against
// the code's vocabulary (from names where code comes from), each enum on its
// own and both ways, and checks that the properties are exactly the ones at
// paths. Pooling the enums of one name into one set lets one of them lose a
// word another still has, and the pooled set stays whole; and a property
// added under the name fails here until its path is listed, so it is
// compared on its own rather than pooled or missed. Break: drop a word from
// one of status_masked's two enums, and the row names that enum's path.
//
// It is assertEnumsPerPath with the one code list, and the one nullability,
// at every path.
func assertNamedEnums(t *testing.T, name string, paths []string, from string, code []string, nullable bool) {
	t.Helper()
	rows := make([]enumRow, len(paths))
	for i, p := range paths {
		rows[i] = enumRow{path: p, from: from, code: code, nullable: nullable}
	}
	assertEnumsPerPath(t, name, rows)
}

// enumRow is one enum the schema publishes and the code list it must equal:
// the property at path, compared both ways against code, which from names,
// and whether a record may carry null there.
type enumRow struct {
	path     string
	from     string
	code     []string
	nullable bool
}

// assertEnumsPerPath is assertNamedEnums for a name whose enums each publish
// a vocabulary of their own -- type, outcome, phase, reason, schema_version
// -- so each path is compared against its own code list. The properties
// named name outside every allOf must be exactly the rows' paths. One inside
// an allOf is a condition on a row's value, not a vocabulary: a const there
// must be a word of the row for the same def's property, and an enum there
// would be a second vocabulary nothing compares, so it fails until it has a
// row.
func assertEnumsPerPath(t *testing.T, name string, rows []enumRow) {
	t.Helper()
	byPath := map[string]enumRow{}
	var paths []string
	for _, r := range rows {
		byPath[r.path] = r
		paths = append(paths, r.path)
	}
	slices.Sort(paths)
	enums := map[string]namedEnum{}
	var at []string
	var conditions []namedEnum
	for _, e := range namedEnums(readSchema(t), name) {
		if strings.Contains(e.path, "/allOf/") {
			conditions = append(conditions, e)
			continue
		}
		enums[e.path] = e
		at = append(at, e.path)
	}
	if !slices.Equal(at, paths) {
		t.Fatalf("%s publishes %s at %q, want %q: list a new one here, so that it is compared on its own",
			schemaPath, name, at, paths)
	}
	for _, r := range rows {
		e := enums[r.path]
		inCode := map[string]bool{}
		for _, v := range r.code {
			inCode[v] = true
			if !e.words[v] {
				t.Errorf("%s carries %q, which the %s enum at %s in %s does not", r.from, v, name, e.path, schemaPath)
			}
		}
		for _, v := range slices.Sorted(maps.Keys(e.words)) {
			if !inCode[v] {
				t.Errorf("the %s enum at %s in %s offers %q, which %s cannot emit", name, e.path, schemaPath, v, r.from)
			}
		}
		assertNullability(t, name, r, e)
	}
	for _, c := range conditions {
		if c.enum {
			t.Errorf("%s publishes an enum for %s at %s, inside an allOf; give it a row, so that it is compared",
				schemaPath, name, c.path)
		}
		if c.konst == "" {
			continue
		}
		def, _, _ := strings.Cut(c.path, "/allOf/")
		r, ok := byPath[def+"/properties/"+name]
		switch {
		case !ok:
			t.Errorf("%s pins %s to %q at %s, and no row compares %s on that def", schemaPath, name, c.konst, c.path, name)
		case !slices.Contains(r.code, c.konst):
			t.Errorf("%s pins %s to %q at %s, which %s cannot emit", schemaPath, name, c.konst, c.path, r.from)
		}
	}
}

// namedEnum is one property the schema publishes under a vocabulary's name:
// its JSON pointer (RFC 6901), whether it has an enum, that enum's members,
// the const it pins, if any, and what it says about null. Strings are taken
// as they are and numbers in decimal, so schema_version's members compare as
// strconv.Itoa spells them. A null member is not a word: null says whether
// the field may be absent of a value, which assertNullability compares on
// its own, and a null const pins nothing.
type namedEnum struct {
	path     string
	enum     bool
	words    map[string]bool
	konst    string
	null     bool // the enum has a null member
	typeNull bool // the type admits null: no type, "null", or a list naming it
}

// namedEnums is every property under node named name, with an enum or
// without one, sorted by path.
func namedEnums(node any, name string) []namedEnum {
	var out []namedEnum
	var walk func(node any, path string)
	walk = func(node any, path string) {
		switch v := node.(type) {
		case map[string]any:
			for k, child := range v {
				p := path + "/" + strings.NewReplacer("~", "~0", "/", "~1").Replace(k)
				if prop, ok := child.(map[string]any); ok && k == name {
					raw, enum := prop["enum"]
					e := namedEnum{path: p, enum: enum, words: map[string]bool{}}
					for _, w := range toAnySlice(raw) {
						if s, ok := enumWord(w); ok {
							e.words[s] = true
						}
						e.null = e.null || w == nil
					}
					e.konst, _ = enumWord(prop["const"])
					switch typ := prop["type"].(type) {
					case nil:
						e.typeNull = true
					case string:
						e.typeNull = typ == "null"
					case []any:
						e.typeNull = slices.Contains(typ, any("null"))
					}
					out = append(out, e)
				}
				walk(child, p)
			}
		case []any:
			for i, child := range v {
				walk(child, path+"/"+strconv.Itoa(i))
			}
		}
	}
	walk(node, "")
	slices.SortFunc(out, func(a, b namedEnum) int { return strings.Compare(a.path, b.path) })
	return out
}

// enumWord is an enum member or a const as namedEnum keeps it: a string as
// it is, a number in decimal, and false for null or anything else.
func enumWord(v any) (string, bool) {
	switch w := v.(type) {
	case string:
		return w, true
	case float64:
		return strconv.FormatFloat(w, 'f', -1, 64), true
	}
	return "", false
}

// assertNullability holds a row's nullable to the schema and, where the row's
// path is a field of a record the store writes, to the Go type of that
// field: a pointer is the only field the writer can leave null. A nullable
// row needs a null member in its enum and a type that admits null, or a
// record the store writes with null there fails the contract; a row that is
// not nullable must offer no null, which the writer can never produce.
func assertNullability(t *testing.T, name string, r enumRow, e namedEnum) {
	t.Helper()
	switch {
	case r.nullable && !e.null:
		t.Errorf("%s may be null at %s, and the %s enum there in %s has no null member, so a record "+
			"carrying null fails the contract", r.from, e.path, name, schemaPath)
	case r.nullable && !e.typeNull:
		t.Errorf("%s may be null at %s, and the type there in %s does not admit null", r.from, e.path, schemaPath)
	case !r.nullable && e.null:
		t.Errorf("the %s enum at %s in %s offers null, which %s never writes", name, e.path, schemaPath, r.from)
	}
	f, record, ok := recordField(e.path)
	switch {
	case record && !ok:
		t.Errorf("%s is under a record def, and no field of the struct the store writes it from carries "+
			"that json name, so its nullability is compared against nothing", e.path)
	case ok && (f.Type.Kind() == reflect.Pointer) != r.nullable:
		t.Errorf("the row for %s says nullable %v, and the store writes it from %s.%s, a %s",
			e.path, r.nullable, f.owner, f.Name, f.Type)
	}
}

// recordTypes is the struct the store writes each record def from.
var recordTypes = map[string]reflect.Type{
	"coverage":       reflect.TypeFor[store.Coverage](),
	"declaration":    reflect.TypeFor[store.Declaration](),
	"execution":      reflect.TypeFor[store.Execution](),
	"forgotten_host": reflect.TypeFor[store.ForgottenHostRecord](),
	"gap":            reflect.TypeFor[store.Gap](),
	"terminal":       reflect.TypeFor[store.Terminal](),
}

// namedField is a struct field and the type that declares it.
type namedField struct {
	reflect.StructField
	owner string
}

// recordField is the Go field behind a schema path of the form
// /$defs/<def>/properties/<key>[/properties/<key>...], found by json tag and
// through embedded structs, as encoding/json finds it. record says the path
// is under a def in recordTypes, so a field should be there; report_reason,
// a def no record carries, is not.
func recordField(path string) (f namedField, record, ok bool) {
	rest, _ := strings.CutPrefix(path, "/$defs/")
	parts := strings.Split(rest, "/properties/")
	typ, record := recordTypes[parts[0]]
	if !record || len(parts) < 2 {
		return namedField{}, record, false
	}
	for _, key := range parts[1:] {
		if typ.Kind() != reflect.Struct {
			return namedField{}, true, false
		}
		if f, ok = jsonField(typ, key); !ok {
			return namedField{}, true, false
		}
		typ = f.Type
	}
	return f, true, true
}

// jsonField is the field of typ whose json name is key, looking through
// untagged embedded structs.
func jsonField(typ reflect.Type, key string) (namedField, bool) {
	for i := range typ.NumField() {
		f := typ.Field(i)
		tag, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if f.Anonymous && tag == "" && f.Type.Kind() == reflect.Struct {
			if nf, ok := jsonField(f.Type, key); ok {
				return nf, true
			}
			continue
		}
		if tag == key {
			return namedField{f, typ.Name()}, true
		}
	}
	return namedField{}, false
}
