package settings

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// TestParseRefuses covers the documents this package declines to edit. Each is
// refused rather than repaired: a file whose meaning depends on which of two
// duplicate keys a decoder happened to keep, or on what follows the object we
// read, is one this package can only guess at, and guessing here rewrites the
// user's configuration.
func TestParseRefuses(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
	}{
		{"duplicate top-level keys", `{"hooks": {}, "hooks": {}}`},
		{"a second object after the first", `{"a": 1} {"b": 2}`},
		{"trailing garbage after the object", `{"a": 1} nonsense`},
		{"an array at the top level", `[{"a": 1}]`},
		{"a string at the top level", `"settings"`},
		{"a number at the top level", `42`},
		{"null at the top level", `null`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := Parse([]byte(tc.in))
			if err == nil {
				t.Errorf("Parse(%s) returned a document of %d members, want an error", tc.in, len(doc.members))
			}
		})
	}
}

// TestHookEntriesRefusesDuplicateEventKeys is the same rule one level down,
// where Parse holds the value as raw bytes and never looks inside it. The
// refusal has to come from the level that does look.
func TestHookEntriesRefusesDuplicateEventKeys(t *testing.T) {
	doc, err := Parse([]byte(`{"hooks": {"PreToolUse": [], "PreToolUse": []}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if entries, err := doc.HookEntries("PreToolUse"); err == nil {
		t.Errorf("HookEntries returned %d entries, want an error: which PreToolUse did it read?", len(entries))
	}
}

// TestParseEmptyInputIsAnEmptyDocument is what lets watch run on a machine that
// has never run Claude Code. An absent file, an empty one and one holding only
// whitespace are all the same starting point.
func TestParseEmptyInputIsAnEmptyDocument(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t \r\n"} {
		doc, err := Parse([]byte(in))
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if len(doc.members) != 0 {
			t.Errorf("Parse(%q) returned %d members, want none", in, len(doc.members))
		}
		if got, want := string(doc.Bytes()), "{}\n"; got != want {
			t.Errorf("Parse(%q).Bytes() is %q, want %q", in, got, want)
		}
	}
}

// TestBytesPreservesForeignMembers is the byte-identity promise at the unit
// level. The values below are spaced the way a hand-edited settings.json is and
// ordered the way its writers happened to add them; anything that has been
// through an encoder comes back sorted and restyled.
func TestBytesPreservesForeignMembers(t *testing.T) {
	foreign := []struct{ key, raw string }{
		{"permissions", "{\"allow\": [ \"Bash(ls:*)\",\n      \"Read(//tmp/**)\" ],   \"deny\":[]}"},
		{"hooks", `{"PreToolUse": [{"matcher":"Bash","hooks":[{"type":"command","command":"echo foreign"}]}]}`},
		{"apiKeyHelper", `"~/bin/key.sh"`},
		{"env", `{ "ALTRACE_DEMO" : "1" }`},
	}

	var in strings.Builder
	in.WriteString("{")
	for i, m := range foreign {
		if i > 0 {
			in.WriteByte(',')
		}
		fmt.Fprintf(&in, "\n   %q  :  %s", m.key, m.raw)
	}
	in.WriteString("\n}\n")

	doc, err := Parse([]byte(in.String()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	out := doc.Bytes()

	at := -1
	for _, m := range foreign {
		raw, ok := doc.Get(m.key)
		if !ok {
			t.Fatalf("%q is missing from the document", m.key)
		}
		if string(raw) != m.raw {
			t.Errorf("%q reads back as %s, want %s", m.key, raw, m.raw)
		}
		i := bytes.Index(out, []byte(m.raw))
		if i < 0 {
			t.Errorf("%q is not in the output as the bytes it was read as:\n%s", m.key, out)
			continue
		}
		if i < at {
			t.Errorf("%q moved ahead of a member that preceded it:\n%s", m.key, out)
		}
		at = i
	}

	again, err := Parse(out)
	if err != nil {
		t.Fatalf("re-parsing our own output: %v", err)
	}
	if !bytes.Equal(again.Bytes(), out) {
		t.Errorf("a second round trip moved the file:\n%s\n%s", out, again.Bytes())
	}
}
