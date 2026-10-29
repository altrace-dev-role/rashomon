package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// writeMarker returns a setup that writes content to the marker path as is.
func writeMarker(content string) func(*testing.T, string) {
	return func(t *testing.T, path string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// TestRS5_UnreadableOrUnknownMarkerIsV4: the marker names v5 only when it
// holds exactly that word (a trailing newline or space aside); every other
// state of the file -- absent, empty, another word, the word twice, a
// directory -- reads v4, with an error saying why, so no fault of the file
// can turn recording of content on.
func TestRS5_UnreadableOrUnknownMarkerIsV4(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(*testing.T, string)
		want    string
		wantErr func(error) bool
	}{
		{"v5", writeMarker("v5\n"), RecordScopeV5, nil},
		{"v5 without its newline", writeMarker("v5"), RecordScopeV5, nil},
		{"v4", writeMarker("v4\n"), RecordScopeV4, nil},
		{"absent", func(*testing.T, string) {}, RecordScopeV4,
			func(err error) bool { return errors.Is(err, fs.ErrNotExist) }},
		{"v6", writeMarker("v6\n"), RecordScopeV4,
			func(err error) bool { return errors.Is(err, ErrRecordScopeWord) }},
		{"empty", writeMarker(""), RecordScopeV4,
			func(err error) bool { return errors.Is(err, ErrRecordScopeWord) }},
		{"V5 in capitals", writeMarker("V5\n"), RecordScopeV4,
			func(err error) bool { return errors.Is(err, ErrRecordScopeWord) }},
		{"v5 twice", writeMarker("v5\nv5\n"), RecordScopeV4,
			func(err error) bool { return errors.Is(err, ErrRecordScopeWord) }},
		{"a directory", func(t *testing.T, path string) {
			if err := os.Mkdir(path, 0o700); err != nil {
				t.Fatal(err)
			}
		}, RecordScopeV4, func(err error) bool { return err != nil && !errors.Is(err, ErrRecordScopeWord) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			c.setup(t, filepath.Join(root, FileRecordScope))
			got, err := RecordScopeMarker(root)
			if got != c.want {
				t.Errorf("RecordScopeMarker = %q, want %q", got, c.want)
			}
			switch {
			case c.wantErr == nil && err != nil:
				t.Errorf("RecordScopeMarker error = %v, want none", err)
			case c.wantErr != nil && !c.wantErr(err):
				t.Errorf("RecordScopeMarker error = %v, not the one that says why the scope is v4", err)
			}
		})
	}
}

// TestRS5b_WriterRefusesAnUnknownScope: the writer takes v4 or v5 and
// nothing else, so the reader's closed vocabulary is the writer's too.
func TestRS5b_WriterRefusesAnUnknownScope(t *testing.T) {
	root := t.TempDir()
	for _, scope := range []string{RecordScopeV4, RecordScopeV5} {
		if err := WriteRecordScopeMarker(root, scope); err != nil {
			t.Fatalf("WriteRecordScopeMarker(%q): %v", scope, err)
		}
		if got, err := RecordScopeMarker(root); got != scope || err != nil {
			t.Errorf("after writing %q the marker reads %q, %v", scope, got, err)
		}
	}
	for _, scope := range []string{"", "v6", "V5", "v5\n"} {
		if err := WriteRecordScopeMarker(root, scope); err == nil {
			t.Errorf("WriteRecordScopeMarker(%q) succeeded, want a refusal", scope)
		}
	}
	if got, _ := RecordScopeMarker(root); got != RecordScopeV5 {
		t.Errorf("a refused write changed the marker: it reads %q, want the v5 written before", got)
	}
	if err := RemoveRecordScopeMarker(root); err != nil {
		t.Fatalf("RemoveRecordScopeMarker: %v", err)
	}
	if err := RemoveRecordScopeMarker(root); err != nil {
		t.Errorf("RemoveRecordScopeMarker with no marker: %v, want none", err)
	}
	if _, err := RecordScopeMarker(root); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("after the remove the marker reads with error %v, want not-exist", err)
	}
}

// TestRS6_MarkerReadMintsNoIdentity: reading the marker, and writing it, is a
// plain file operation, never through Open, which mints an install identity
// and a key on first use (Paused's rule). A root that does not exist stays
// absent when read; a root holding only the marker gains nothing.
func TestRS6_MarkerReadMintsNoIdentity(t *testing.T) {
	absent := filepath.Join(t.TempDir(), "never-recorded")
	if got, _ := RecordScopeMarker(absent); got != RecordScopeV4 {
		t.Errorf("an absent root reads %q, want v4", got)
	}
	if _, err := os.Stat(absent); err == nil {
		t.Errorf("RecordScopeMarker created %s merely by being asked", absent)
	}

	root := t.TempDir()
	if err := WriteRecordScopeMarker(root, RecordScopeV5); err != nil {
		t.Fatalf("WriteRecordScopeMarker: %v", err)
	}
	if got, err := RecordScopeMarker(root); got != RecordScopeV5 || err != nil {
		t.Fatalf("RecordScopeMarker = %q, %v; want v5", got, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{FileRecordScope}) {
		t.Errorf("a root holding only the marker holds %q after a write and a read, want the marker alone", names)
	}
}

// gate is one *_withheld key the record-scope gate stamps no_consent, with
// the keys it nulls.
type gate struct {
	withheld string
	nulls    []string
}

// gatedDeclaration and gatedExecution are the v5 content fields: text a
// user's environment supplied (a command, a path, an account or bucket name,
// a repository state) that the v4 record does not carry. At any record
// scope but v5 each is null with its *_withheld key no_consent.
var (
	gatedDeclaration = []gate{
		{"command_withheld", []string{"command", "command_cut", "masked"}},
		{"cwd_withheld", []string{"cwd"}},
		{"project_withheld", []string{"project"}},
		{"file_path_withheld", []string{"file_path"}},
		{"written_paths_withheld", []string{"written_paths"}},
		{"target_withheld", []string{"target"}},
	}
	gatedExecution = []gate{
		{"executed_command_withheld", []string{"executed_command", "executed_command_cut", "executed_masked"}},
		{"head_after_withheld", []string{"head_after"}},
	}
)

// exemptDeclaration and exemptExecution are every other *_withheld key, each
// with the reason the gate leaves its field alone.
var (
	exemptDeclaration = map[string]string{
		"prompt_id_withheld":         "a harness id, which the v4 record carries",
		"agent_id_withheld":          "a harness id, which the v4 record carries",
		"agent_type_withheld":        "a harness agent type, which the v4 record carries",
		"hosts_withheld":             "the hostnames, which the v4 record carries",
		"ssh_hosts_withheld":         "the ssh hostnames, which the v4 record carries",
		"edit_marks_before_withheld": "counts per closed mark id; no text",
	}
	exemptExecution = map[string]string{
		"failure_withheld":         "a closed class, its source and a byte count; no text",
		"output_evidence_withheld": "a family name, booleans and counts; no text",
		"edit_marks_withheld":      "counts per closed mark id; no text",
		"head_after.root_withheld": "inside head_after, which is gated whole",
	}
)

// withheldKeyPaths lists every json key ending in _withheld under t, embedded
// structs flattened, struct and pointer-to-struct fields entered with their
// key and a dot as a prefix.
func withheldKeyPaths(t reflect.Type, prefix string) []string {
	var keys []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			ft = ft.Elem()
		}
		key := strings.Split(f.Tag.Get("json"), ",")[0]
		switch {
		case f.Anonymous && ft.Kind() == reflect.Struct:
			keys = append(keys, withheldKeyPaths(ft, prefix)...)
		case ft.Kind() == reflect.Struct:
			keys = append(keys, withheldKeyPaths(ft, prefix+key+".")...)
		case strings.HasSuffix(key, "_withheld"):
			keys = append(keys, prefix+key)
		}
	}
	return keys
}

// TestRS7_EveryContentFieldIsGated: every *_withheld key on the declaration
// and the execution is either gated by the record scope or exempt with a
// reason, never both and never neither. A field added later with a withheld
// key and no entry here fails, so it cannot ship ungated by default; the
// written_paths producer (1.2w) and the target and head_after producers find
// theirs already gated.
func TestRS7_EveryContentFieldIsGated(t *testing.T) {
	cases := []struct {
		typ    reflect.Type
		gated  []gate
		exempt map[string]string
	}{
		{reflect.TypeOf(Declaration{}), gatedDeclaration, exemptDeclaration},
		{reflect.TypeOf(Execution{}), gatedExecution, exemptExecution},
	}
	for _, c := range cases {
		have := withheldKeyPaths(c.typ, "")
		if len(have) == 0 {
			t.Fatalf("%s: reflection found no _withheld key; the walk is broken", c.typ.Name())
		}
		listed := map[string]bool{}
		for _, g := range c.gated {
			listed[g.withheld] = true
			if _, both := c.exempt[g.withheld]; both {
				t.Errorf("%s: %s is both gated and exempt", c.typ.Name(), g.withheld)
			}
		}
		for k, reason := range c.exempt {
			listed[k] = true
			if reason == "" {
				t.Errorf("%s: %s is exempt with no reason", c.typ.Name(), k)
			}
		}
		for _, k := range have {
			if !listed[k] {
				t.Errorf("%s: %s is neither gated by the record scope nor exempt: gate its field in "+
					"store's scope gate if it holds content, or exempt it here with the reason", c.typ.Name(), k)
			}
			delete(listed, k)
		}
		for k := range listed {
			t.Errorf("%s: %s is listed here and is not a key of the type", c.typ.Name(), k)
		}
	}
}

// clearWithheld nulls every *string field whose json key ends in _withheld,
// at any depth, so a filled record says nothing was withheld before a test
// runs the gate.
func clearWithheld(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer:
		if !v.IsNil() {
			clearWithheld(v.Elem())
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			if strings.HasSuffix(strings.Split(f.Tag.Get("json"), ",")[0], "_withheld") {
				v.Field(i).SetZero()
				continue
			}
			clearWithheld(v.Field(i))
		}
	}
}

// wire is a record as a reader sees it: decoded from its JSON.
func wire(t *testing.T, v any) map[string]any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// at is the value under a dotted key, and whether every step was there.
func at(m map[string]any, key string) (any, bool) {
	parent, leaf, nested := strings.Cut(key, ".")
	v, ok := m[parent]
	if !nested || !ok {
		return v, ok
	}
	sub, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	return at(sub, leaf)
}

// checkGate checks a record the gate ran on at scope: at v5 every gated and
// exempt field kept its value and no withheld key is set; at any other scope
// every gated field is null with its key no_consent and every exempt field
// kept its value.
func checkGate(t *testing.T, name, scope string, m map[string]any, gated []gate, exempt map[string]string) {
	t.Helper()
	for _, g := range gated {
		w, _ := at(m, g.withheld)
		for _, k := range g.nulls {
			v, _ := at(m, k)
			if scope == RecordScopeV5 && v == nil {
				t.Errorf("%s at %q: %s is null, want the value kept", name, scope, k)
			}
			if scope != RecordScopeV5 && v != nil {
				t.Errorf("%s at %q: %s = %v, want null: the user did not agree to record it", name, scope, k, v)
			}
		}
		switch {
		case scope == RecordScopeV5 && w != nil:
			t.Errorf("%s at %q: %s = %v, want null", name, scope, g.withheld, w)
		case scope != RecordScopeV5 && w != withheldNoConsent:
			t.Errorf("%s at %q: %s = %v, want %q", name, scope, g.withheld, w, withheldNoConsent)
		}
	}
	for k := range exempt {
		base := strings.TrimSuffix(k, "_withheld")
		v, ok := at(m, base)
		if !ok {
			continue // inside a field the gate nulled whole
		}
		if v == nil {
			t.Errorf("%s at %q: %s is null, want it kept: the gate exempts it", name, scope, base)
		}
		if w, _ := at(m, k); w != nil {
			t.Errorf("%s at %q: %s = %v, want null", name, scope, k, w)
		}
	}
}

// TestRS1u_GateNullsEveryContentFieldOnlyBelowV5: the gate, on a declaration
// and an execution with every field filled, nulls each content field and
// stamps no_consent at v4 and at any word that is not v5, and at v5 changes
// nothing. The command fields have no producer in the hook yet, so this is
// where their gate is pinned.
func TestRS1u_GateNullsEveryContentFieldOnlyBelowV5(t *testing.T) {
	for _, scope := range []string{RecordScopeV4, "", "v6", RecordScopeV5} {
		var d Declaration
		fill(reflect.ValueOf(&d).Elem())
		clearWithheld(reflect.ValueOf(&d).Elem())
		GateDeclaration(&d, scope)
		m := wire(t, d)
		checkGate(t, "declaration", scope, m, gatedDeclaration, exemptDeclaration)
		complete, _ := at(m, "written_paths_complete")
		if want := scope == RecordScopeV5; complete != want {
			t.Errorf("declaration at %q: written_paths_complete = %v, want %v", scope, complete, want)
		}

		var x Execution
		fill(reflect.ValueOf(&x).Elem())
		clearWithheld(reflect.ValueOf(&x).Elem())
		GateExecution(&x, scope)
		checkGate(t, "execution", scope, wire(t, x), gatedExecution, exemptExecution)
		// content_sha256 has no withheld key: it is null below v5 and kept
		// at v5 (P5A).
		if sha, _ := at(wire(t, x), "content_sha256"); (sha == nil) != (scope != RecordScopeV5) {
			t.Errorf("execution at %q: content_sha256 = %v", scope, sha)
		}
	}
}

// TestRS1u_GateLeavesAnAbsentValueAlone: the gate stamps only a value that is
// there. A field already null keeps its withheld key as it was, null or a
// reason another rule set, so no_consent never stands for a value that never
// existed.
func TestRS1u_GateLeavesAnAbsentValueAlone(t *testing.T) {
	masker := "masker_error"
	d := Declaration{}
	d.CommandWithheld = &masker
	GateDeclaration(&d, RecordScopeV4)
	m := wire(t, d)
	for _, g := range gatedDeclaration {
		want := any(nil)
		if g.withheld == "command_withheld" {
			want = masker
		}
		if w, _ := at(m, g.withheld); w != want {
			t.Errorf("empty declaration at v4: %s = %v, want %v", g.withheld, w, want)
		}
	}
	if complete, _ := at(m, "written_paths_complete"); complete != nil {
		t.Errorf("empty declaration at v4: written_paths_complete = %v, want null", complete)
	}
	x := Execution{}
	GateExecution(&x, RecordScopeV4)
	for _, g := range gatedExecution {
		if w, _ := at(wire(t, x), g.withheld); w != nil {
			t.Errorf("empty execution at v4: %s = %v, want null", g.withheld, w)
		}
	}
}

// TestRS1u_GateRunsBeforeTheCaps: a content field over its cap at v4 reads
// no_consent, not oversize, and the caps then find nothing to withhold, so
// the call's coverage record does not claim record_oversize for a value the
// scope already left out. At v5 the same value is the cap's.
func TestRS1u_GateRunsBeforeTheCaps(t *testing.T) {
	over := strings.Repeat("p", MaxPathBytes+1)
	for _, scope := range []string{RecordScopeV4, RecordScopeV5} {
		d := Declaration{}
		d.CWD, d.Project, d.FilePath = &over, &over, &over
		GateDeclaration(&d, scope)
		withheld := CapDeclaration(&d)
		want, wantWithheld := withheldNoConsent, false
		if scope == RecordScopeV5 {
			want, wantWithheld = withheldOversize, true
		}
		if withheld != wantWithheld {
			t.Errorf("at %q: CapDeclaration reported %v, want %v", scope, withheld, wantWithheld)
		}
		for name, w := range map[string]*string{"cwd": d.CWDWithheld, "project": d.ProjectWithheld, "file_path": d.FilePathWithheld} {
			if w == nil || *w != want {
				t.Errorf("at %q: %s_withheld = %v, want %q", scope, name, w, want)
			}
		}
	}
}
