package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

// TestRubricTEST10_AnEventNameThatIsNotUTF8IsRefused: JSON cannot carry a key
// that is not UTF-8, so writing one would put U+FFFD in the file and read back
// as a different event. Both edits refuse it with ErrEventNotUTF8, and the
// document is left as it was.
func TestRubricTEST10_AnEventNameThatIsNotUTF8IsRefused(t *testing.T) {
	in := []byte(`{"hooks": {"Stop": []}}`)
	entries := []json.RawMessage{json.RawMessage(`{"matcher":"*"}`)}
	for name, edit := range map[string]func(*Document, string) error{
		"SetHookEntries":  func(d *Document, ev string) error { return d.SetHookEntries(ev, entries) },
		"RemoveHookEvent": func(d *Document, ev string) error { return d.RemoveHookEvent(ev) },
	} {
		for _, ev := range []string{"\xbd", "Stop\xff", "\xed\xa0\x80"} {
			doc, err := ParseNamed("/u/.claude/settings.json", in)
			if err != nil {
				t.Fatal(err)
			}
			before := doc.Bytes()
			err = edit(doc, ev)
			if !errors.Is(err, ErrEventNotUTF8) {
				t.Errorf("%s(%q) = %v, want ErrEventNotUTF8", name, ev, err)
			}
			var e *Error
			if !errors.As(err, &e) || e.Path != "/u/.claude/settings.json" {
				t.Errorf("%s(%q) = %v, want a *settings.Error naming the file", name, ev, err)
			}
			if got := doc.Bytes(); !bytes.Equal(got, before) {
				t.Errorf("%s(%q) refused and still changed the document:\n%q\n%q", name, ev, before, got)
			}
		}
	}
}

// TestRubricERRORS10_ParseNamedKeepsItsOwnCopy: a caller that reuses its
// buffer after parsing changes neither what the document renders nor where a
// later error places itself.
func TestRubricERRORS10_ParseNamedKeepsItsOwnCopy(t *testing.T) {
	in := []byte("{\n  \"keep\": 1,\n  \"hooks\": {\"Stop\": {}}\n}\n")
	buf := bytes.Clone(in)
	doc, err := ParseNamed("settings.json", buf)
	if err != nil {
		t.Fatal(err)
	}
	for i := range buf {
		buf[i] = '\n'
	}
	if got, want := doc.Bytes(), mustParse(t, in).Bytes(); !bytes.Equal(got, want) {
		t.Errorf("after the caller reused its buffer the document renders\n%q\nwant\n%q", got, want)
	}
	_, err = doc.HookEntries("Stop")
	_, fresh := mustParse(t, in).HookEntries("Stop")
	e, want := errorOf(t, err), errorOf(t, fresh)
	if want.Line != 3 {
		t.Fatalf("premise: a fresh parse places the error at %d:%d, not on line 3", want.Line, want.Column)
	}
	if e.Line != want.Line || e.Column != want.Column {
		t.Errorf("after the caller reused its buffer the error is at %d:%d, want %d:%d (%v)",
			e.Line, e.Column, want.Line, want.Column, err)
	}
}

func mustParse(t *testing.T, in []byte) *Document {
	t.Helper()
	d, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestRubricERRORS10_AnErrorWithNoCauseStillRenders: Error is exported, so a
// caller can build one with no Err, or hold a nil *Error; neither panics.
func TestRubricERRORS10_AnErrorWithNoCauseStillRenders(t *testing.T) {
	for _, tc := range []struct {
		e    *Error
		want string
	}{
		{&Error{Path: "a.json", Line: 2, Column: 3}, "a.json:2:3: invalid settings"},
		{&Error{}, "settings: invalid settings"},
		{nil, "settings: <nil>"},
	} {
		if got := tc.e.Error(); got != tc.want {
			t.Errorf("Error() = %q, want %q", got, tc.want)
		}
		if err := tc.e.Unwrap(); err != nil {
			t.Errorf("Unwrap() = %v, want nil", err)
		}
	}
}
