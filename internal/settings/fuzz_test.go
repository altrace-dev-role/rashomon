package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// The settings editor rewrites a file the user owns and Claude Code keeps
// writing. Its promise (the package doc) is that everything it does not edit
// comes back as the bytes it was read as, and that what it does edit is still
// the document it read. These properties are checked over arbitrary input, with
// encoding/json as the oracle for what the bytes mean:
//
//   - Parse accepts only valid JSON, and refuses a valid object only for a
//     duplicate key;
//   - a refusal is an *Error whose line and column lie inside the input;
//   - Bytes means what the input meant, keeps every value byte for byte and in
//     order, and is a fixed point: parsing it and rendering again moves nothing;
//   - an edit of one hook event reads back as written, carries every other
//     member and event byte for byte, and undoing it restores the meaning.
//
// Keys are compared by what they decode to. The editor re-encodes the keys of
// the two levels it renders, so "hooks" comes back as "hooks"; no byte of
// a value moves.

var roundTripSeeds = []string{
	``,
	`{}`,
	"{\n  \"permissions\": {\"allow\": [ \"Bash(ls:*)\" ]},\n  \"hooks\": {\"PreToolUse\": [{\"matcher\": \"*\"}], \"Stop\": null}\n}\n",
	`{"hooks": null}`,
	`{"hooks": {"PreToolUse": {}}}`,
	`{"hooks": []}`,
	`{"hooks": {"PreToolUse": [], "PreToolUse": []}}`,
	`{"a": 1, "a": 2}`,
	`{"a": 1,}`,
	`{"a": 1} trailing`,
	`{"hooks": {"<&>": [1]}, "k\u0007ey": "\ud800", "x": 1e400}`,
	"{\"bad utf8 \xff\": \"\xfe\"}",
	`[1]`,
	`"settings"`,
	`{"a": {"b": [{"c": "` + strings.Repeat("d", 300) + `"}]}}`,
}

func FuzzDocumentRoundTrip(f *testing.F) {
	for _, s := range roundTripSeeds {
		f.Add([]byte(s), "PreToolUse")
	}
	f.Add([]byte(`{"hooks": {"Stop": []}}`), "< \x00>")
	f.Fuzz(func(t *testing.T, data []byte, event string) { checkRoundTrip(t, data, event) })
}

// TestRubricTest10_RoundTripSeeds runs the properties over the seed corpus as
// a plain test, which is what a mutant names: a fuzz target's seeds run only
// under its own name.
func TestRubricTest10_RoundTripSeeds(t *testing.T) {
	for _, s := range roundTripSeeds {
		for _, event := range []string{"PreToolUse", "Stop", "< \x00>"} {
			checkRoundTrip(t, []byte(s), event)
		}
	}
}

func checkRoundTrip(t *testing.T, data []byte, event string) {
	t.Helper()
	doc, err := Parse(data)
	blank := len(bytes.TrimSpace(data)) == 0
	if err != nil {
		checkRefusal(t, data, err)
		return
	}
	if !blank && !json.Valid(data) {
		t.Fatalf("Parse accepted %q, which is not JSON", data)
	}
	out := doc.Bytes()
	again, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse refused its own output %q: %v", out, err)
	}
	if !blank {
		sameMeaning(t, "Bytes", data, out)
	}
	sameMembers(t, "Bytes", doc.members, again.members)
	if b := again.Bytes(); !bytes.Equal(b, out) {
		t.Fatalf("a second round trip moved the file:\n%q\n%q", out, b)
	}
	checkEdit(t, doc, out, event)
}

// checkRefusal: a refusal is an *Error placed inside the input, and a valid
// object is refused only for a duplicate key.
func checkRefusal(t *testing.T, data []byte, err error) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("Parse(%q) error %v is %T, not *settings.Error", data, err, err)
	}
	if lines := bytes.Count(data, []byte{'\n'}) + 1; e.Line < 1 || e.Line > lines || e.Column < 1 {
		t.Fatalf("Parse(%q) error %v is at %d:%d, outside %d lines", data, err, e.Line, e.Column, lines)
	}
	// Where settings' own error wraps a json.SyntaxError -- never true for
	// its semantic refusals ("not a JSON object" and the like, which wrap
	// no encoding/json error at all -- a fresh run of encoding/json's own
	// scanner over the WHOLE input is a second, independent place for it.
	// That fresh run, not the wrapped SyntaxError itself, is the oracle:
	// the wrapped one can be one encoding/json ran over a narrower slice (a
	// broken "hooks" value, parsed on its own bytes) with an Offset
	// settings re-bases before this test ever sees it, where the scanner
	// run over the whole document lands on the same absolute byte either
	// way. Its Offset is the first byte after the one the scanner stopped
	// on, which is refusal's own Offset-1. Computed without calling
	// lineColumn, so this is a check ON that function, not one that passes
	// however it adds as long as it adds consistently with itself.
	var wrapped *json.SyntaxError
	var fresh *json.SyntaxError
	if errors.As(err, &wrapped) && errors.As(json.Unmarshal(data, new(json.RawMessage)), &fresh) {
		wantLine, wantCol := independentLineColumn(data, max(int(fresh.Offset)-1, 0))
		if e.Line != wantLine || e.Column != wantCol {
			t.Fatalf("Parse(%q) error %v is at %d:%d, want %d:%d from encoding/json's own offset %d",
				data, err, e.Line, e.Column, wantLine, wantCol, fresh.Offset)
		}
	}
	trimmed := bytes.TrimSpace(data)
	if json.Valid(data) && trimmed[0] == '{' && !strings.Contains(err.Error(), "duplicate key") {
		t.Fatalf("Parse refused the valid object %q: %v", data, err)
	}
}

// independentLineColumn is lineColumn's contract (1-based line and byte
// column of offset off in src), computed by a byte-by-byte walk rather than
// lineColumn's own bytes.Count and bytes.LastIndexByte, so checkRefusal has
// an oracle that does not call the function it is checking.
func independentLineColumn(src []byte, off int) (line, column int) {
	line, lastNL := 1, -1
	for i := 0; i < off && i < len(src); i++ {
		if src[i] == '\n' {
			line++
			lastNL = i
		}
	}
	return line, off - lastNL
}

// checkEdit replaces one hook event, checks what moved, then removes it again.
func checkEdit(t *testing.T, doc *Document, before []byte, event string) {
	t.Helper()
	if _, err := doc.hookEvents(); err != nil {
		return // refused by the editor, which the error tests cover
	}
	if !utf8.ValidString(event) {
		return // outside SetHookEntries' contract; see its doc
	}
	had, err := doc.HookEntries(event)
	if err != nil {
		return
	}
	want := []json.RawMessage{json.RawMessage(`{"matcher":"*","hooks":[{"type":"command","command":"x"}]}`)}
	keep := append([]member(nil), doc.members...)
	keepEvents, _ := doc.hookEvents()
	if err := doc.SetHookEntries(event, want); err != nil {
		t.Fatalf("SetHookEntries(%q): %v", event, err)
	}
	edited := doc.Bytes()
	re, err := Parse(edited)
	if err != nil {
		t.Fatalf("Parse refused the edited document %q: %v", edited, err)
	}
	got, err := re.HookEntries(event)
	if err != nil || len(got) != 1 || !bytes.Equal(got[0], want[0]) {
		t.Fatalf("hooks.%q reads back as %q (%v), want %q", event, got, err, want)
	}
	sameMembers(t, "edit", without(keep, hooksKey), without(re.members, hooksKey))
	reEvents, _ := re.hookEvents()
	sameMembers(t, "edit of hooks", without(keepEvents, event), without(reEvents, event))

	// Undo: put back what was there, or remove what was not.
	if had == nil {
		err = re.RemoveHookEvent(event)
	} else {
		err = re.SetHookEntries(event, had)
	}
	if err != nil {
		t.Fatalf("undoing the edit: %v", err)
	}
	if hadKey(keepEvents, event) && had == nil {
		return // a null or empty event is removed, not restored
	}
	// A hooks member that held nothing -- null or {} -- is removed with the
	// last event in it rather than restored: detach runs in a later process
	// than watch and cannot know an empty hooks was there before. That is the
	// one difference undoing an edit may leave.
	va, vb := decodeAny(t, before), decodeAny(t, re.Bytes())
	if m, ok := va.(map[string]any); ok {
		if h, present := m[hooksKey]; present && isEmptyHooks(h) {
			if _, still := vb.(map[string]any)[hooksKey]; !still {
				delete(m, hooksKey)
			}
		}
	}
	if !reflect.DeepEqual(va, vb) {
		t.Fatalf("undo changed the meaning:\n%q\n%q", before, re.Bytes())
	}
}

func isEmptyHooks(v any) bool {
	m, ok := v.(map[string]any)
	return v == nil || (ok && len(m) == 0)
}

func hadKey(ms []member, key string) bool {
	for _, m := range ms {
		if m.key == key {
			return true
		}
	}
	return false
}

func without(ms []member, key string) []member {
	var out []member
	for _, m := range ms {
		if m.key != key {
			out = append(out, m)
		}
	}
	return out
}

// sameMembers: the same keys, in the same order, with values byte for byte.
func sameMembers(t *testing.T, what string, a, b []member) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("%s: %d members became %d", what, len(a), len(b))
	}
	for i := range a {
		if a[i].key != b[i].key || !bytes.Equal(a[i].raw, b[i].raw) {
			t.Fatalf("%s: member %d %q=%q became %q=%q", what, i, a[i].key, a[i].raw, b[i].key, b[i].raw)
		}
	}
}

// sameMeaning compares two documents as encoding/json reads them.
func sameMeaning(t *testing.T, what string, a, b []byte) {
	t.Helper()
	va, vb := decodeAny(t, a), decodeAny(t, b)
	if !reflect.DeepEqual(va, vb) {
		t.Fatalf("%s changed the meaning:\n%q\n%q", what, a, b)
	}
}

func decodeAny(t *testing.T, data []byte) any {
	t.Helper()
	if len(bytes.TrimSpace(data)) == 0 {
		data = []byte(`{}`)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decoding %q: %v", data, err)
	}
	return v
}
