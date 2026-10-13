package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A settings.json that will not parse is one a person has to open and fix, and
// "invalid character ',' looking for beginning of value" says neither which
// file nor where in it. These tests pin the place: the file's path, then the
// 1-based line and byte column of the byte the parser stopped on, in the form
// editors and compilers use (path:line:column), and the hook event a
// structural error is about.

// errorOf asserts err is a *Error and returns it.
func errorOf(t *testing.T, err error) *Error {
	t.Helper()
	if err == nil {
		t.Fatal("got no error, want one")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error %q (%T) is not a *settings.Error", err, err)
	}
	return e
}

func TestRubricErrors10_SyntaxErrorsNameLineAndColumn(t *testing.T) {
	for _, tc := range []struct {
		name, in   string
		line, col  int
		wantInText string
	}{
		// The second comma is the 9th byte of line 3: `  "b": [1,,2]`.
		{"an empty array element", "{\n  \"a\": 1,\n  \"b\": [1,,2]\n}\n", 3, 11, "invalid character ','"},
		{"a misspelt literal on line two", "{\"a\":\n {\"x\": tru}}", 2, 11, "in literal true"},
		{"a missing colon", "{\n\n  \"a\" 1\n}", 3, 7, "after object key"},
		{"a file cut short", "{\n  \"a\": 1", 2, 8, "unexpected end"},
		{"trailing data on a later line", "{\"a\": 1}\n\n  nonsense", 3, 3, "trailing data"},
		{"an array at the top level", "\n[1]", 2, 1, "not a JSON object"},
		// The duplicate is the key on line 3; its closing quote is byte 9.
		{"a duplicate key", "{\n  \"hooks\": {},\n  \"hooks\": {}\n}", 3, 9, `duplicate key "hooks"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseNamed("/home/u/.claude/settings.json", []byte(tc.in))
			e := errorOf(t, err)
			if e.Line != tc.line || e.Column != tc.col {
				t.Errorf("position %d:%d, want %d:%d (%v)", e.Line, e.Column, tc.line, tc.col, err)
			}
			if e.Path != "/home/u/.claude/settings.json" {
				t.Errorf("path %q, want the file's", e.Path)
			}
			want := "/home/u/.claude/settings.json:" + itoa(tc.line) + ":" + itoa(tc.col) + ": "
			if !strings.HasPrefix(err.Error(), want) || !strings.Contains(err.Error(), tc.wantInText) {
				t.Errorf("error %q, want it to start %q and say %q", err, want, tc.wantInText)
			}
		})
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// TestRubricErrors10_LoadNamesTheFile is the path half through the entry most
// callers use.
func TestRubricErrors10_LoadNamesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{\n  \"a\": 1,\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	e := errorOf(t, err)
	if e.Path != path || e.Line != 3 || e.Column != 1 {
		t.Errorf("Load error %q is at %s:%d:%d, want %s:3:1", err, e.Path, e.Line, e.Column, path)
	}
	if strings.Count(err.Error(), path) != 1 {
		t.Errorf("error %q names the file %d times, want once", err, strings.Count(err.Error(), path))
	}
}

// TestRubricErrors10_HookEventErrorsNameTheEventAndPlace covers the errors
// Parse cannot raise, because it holds hooks as raw bytes: they surface only
// when an edit looks inside, and then they must still say where.
func TestRubricErrors10_HookEventErrorsNameTheEventAndPlace(t *testing.T) {
	const path = "/p/settings.json"
	for _, tc := range []struct {
		name, in, event string
		line, col       int
		want            string
	}{
		// `    "PostToolUse": {"a": 1}` -- the value starts at byte 20.
		{"an event that is an object", "{\n  \"hooks\": {\n    \"PreToolUse\": [],\n    \"PostToolUse\": {\"a\": 1}\n  }\n}",
			"PostToolUse", 4, 20, `hook event "PostToolUse" is not an array`},
		{"hooks that is an array", "{\"x\": 1,\n \"hooks\": [1]}", "PreToolUse", 2, 11, "hooks: not a JSON object"},
		// The second PreToolUse key's closing quote is byte 14 of line 4.
		{"a duplicate event", "{\"hooks\": {\n  \"PreToolUse\": [],\n\n  \"PreToolUse\": []\n}}",
			"PreToolUse", 4, 14, `hooks: duplicate key "PreToolUse"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := ParseNamed(path, []byte(tc.in))
			if err != nil {
				t.Fatalf("ParseNamed: %v", err)
			}
			_, err = doc.HookEntries(tc.event)
			e := errorOf(t, err)
			if e.Path != path || e.Line != tc.line || e.Column != tc.col {
				t.Errorf("error %q is at %s:%d:%d, want %s:%d:%d", err, e.Path, e.Line, e.Column, path, tc.line, tc.col)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not say %q", err, tc.want)
			}
			for _, edit := range []func() error{
				func() error { return doc.SetHookEntries(tc.event, nil) },
				func() error { return doc.RemoveHookEvent(tc.event) },
			} {
				if err := edit(); tc.event != "PostToolUse" && errorOf(t, err).Path != path {
					t.Errorf("edit error %q does not name the file", err)
				}
			}
		})
	}
}

// TestRubricErrors10_AnEditedHooksObjectHasNoPlace pins the rule that a
// position is never a wrong one. Once SetHookEntries rewrites the hooks object,
// the events it carried over sit at offsets of their own in the rewritten
// bytes, not in the file; an error about one of them names the file and the
// event, and no line or column at all.
func TestRubricErrors10_AnEditedHooksObjectHasNoPlace(t *testing.T) {
	// The permissions member after hooks keeps the file longer than the
	// rewritten hooks object, so a stale offset would land inside the file and
	// read as a real place rather than fall off its end.
	in := "{\"hooks\": {\n  \"PreToolUse\": [],\n  \"Stop\": 7\n},\n\"permissions\": {\"allow\": [\n" +
		strings.Repeat("  \"Bash(ls:*)\",\n", 20) + "  \"Read\"\n]}}"
	doc, err := ParseNamed("/p/settings.json", []byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.SetHookEntries("PreToolUse", []json.RawMessage{json.RawMessage(`{"hooks":[]}`)}); err != nil {
		t.Fatal(err)
	}
	_, err = doc.HookEntries("Stop")
	e := errorOf(t, err)
	if e.Line != 0 || e.Column != 0 {
		t.Errorf("error %q claims %d:%d in a hooks object the file does not hold", err, e.Line, e.Column)
	}
	if want := `/p/settings.json: hook event "Stop" is not an array`; err.Error() != want {
		t.Errorf("error %q, want %q", err, want)
	}
}

// TestRubricErrors10_UnnamedDocumentsStillSaySettings keeps Parse, which has
// no file name, from producing a bare "3:1: ..." that reads as nothing.
func TestRubricErrors10_UnnamedDocumentsStillSaySettings(t *testing.T) {
	_, err := Parse([]byte("{\n  \"a\": 1,\n}"))
	if e := errorOf(t, err); e.Path != "" || !strings.HasPrefix(err.Error(), "settings:3:1: ") {
		t.Errorf("Parse error %q, want it to start %q", err, "settings:3:1: ")
	}
}
