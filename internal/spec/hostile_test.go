package spec

import (
	"strings"
	"testing"
)

// TestVER37d_HostileInputs: bytes an untrusted repository can put in spec.json.
// Each is refused with a closed reason and a bounded pointer, none panics, and
// the text that gets through the lexical checks is exactly the text the author
// wrote (archive 20.2.8, Trust: the spec is attack surface).
func TestVER37d_HostileInputs(t *testing.T) {
	why := func(s string) string { return withRule(kv{"why", s}) }
	const wp = "/rules/0/why"
	deepArr := func(n int) string { return top(`"x": ` + strings.Repeat("[", n-1) + strings.Repeat("]", n-1)) }
	rows := []row{
		// NUL.
		bad("raw NUL inside a string", why("\"a\x00b\""), ReasonSyntax, anyPointer),
		bad("raw NUL between tokens", "{\"rashomon_spec\":\x001}", ReasonSyntax, anyPointer),
		bad("raw NUL as the whole file", "\x00", ReasonSyntax, anyPointer),
		bad("NUL escape in text", why(`"a\u0000b"`), ReasonControl, wp),
		bad("NUL escape in an id", withRule(kv{"id", `"a\u0000b"`}), ReasonID, "/rules/0/id"),
		bad("NUL escape in a key is not echoed", top(`"a\u0000b": 1`), ReasonUnknownKey, "/?"),
		bad("NUL escape in a pattern", withRule(kv{"pattern", `"a\u0000"`}), ReasonControl, "/rules/0/pattern"),

		// Invalid UTF-8, inside a string (the value is where it matters).
		bad("lone continuation byte", why("\"a\xffb\""), ReasonNotUTF8, wp),
		bad("overlong NUL", why("\"a\xc0\x80b\""), ReasonNotUTF8, wp),
		bad("truncated three-byte sequence", why("\"a\xe2\x82\""), ReasonNotUTF8, wp),
		bad("encoded surrogate", why("\"a\xed\xa0\x80b\""), ReasonNotUTF8, wp),
		bad("beyond U+10FFFF", why("\"a\xf4\x90\x80\x80b\""), ReasonNotUTF8, wp),
		bad("invalid UTF-8 in a key", top("\"a\xff\": 1"), ReasonNotUTF8, ""),
		bad("invalid UTF-8 in a pattern", withRule(kv{"pattern", "\"a\xff\""}), ReasonNotUTF8, "/rules/0/pattern"),
		bad("invalid UTF-8 outside a string", "{\"rashomon_spec\":1}\xff", ReasonSyntax, anyPointer),
		bad("byte order mark", "\xef\xbb\xbf"+top(""), ReasonSyntax, anyPointer),
		ok("valid multi-byte text", why("\"café \U0001F600\"")),
		ok("a literal U+FFFD is legal", why("\"a�b\"")),
		ok("an escaped U+FFFD is legal", why(`"a�b"`)),

		// Unpaired surrogate escapes: not text, and encoding/json would
		// quietly turn each into U+FFFD, so two files would be one spec.
		bad("lone high surrogate", why(`"a\ud800b"`), ReasonSurrogate, wp),
		bad("lone low surrogate", why(`"a\udc00b"`), ReasonSurrogate, wp),
		bad("lone surrogate at the end of the string", why(`"a\ud800"`), ReasonSurrogate, wp),
		bad("high surrogate then a plain escape", why(`"\ud800A"`), ReasonSurrogate, wp),
		bad("high surrogate then a plain character", why(`"\ud800A"`), ReasonSurrogate, wp),
		bad("high surrogate then a code above the low range", why(`"\ud800\ue000"`), ReasonSurrogate, wp),
		bad("pair in the wrong order", why(`"\udc00\ud800"`), ReasonSurrogate, wp),
		bad("two high surrogates", why(`"\ud800\ud800"`), ReasonSurrogate, wp),
		bad("surrogate after an escaped backslash", why(`"\\\ud800"`), ReasonSurrogate, wp),
		bad("surrogate in a key", top(`"\ud800": 1`), ReasonSurrogate, ""),
		bad("surrogate in a pattern", withRule(kv{"pattern", `"\udfff"`}), ReasonSurrogate, "/rules/0/pattern"),
		bad("lower-case and upper-case hex both count", why(`"\uDBFF"`), ReasonSurrogate, wp),
		ok("a valid pair", why(`"😀"`)),
		ok("a pair at the top of the range", why(`"\udbff\udfff"`)),
		ok("a valid pair in upper-case hex", why(`"😀"`)),
		ok("the text \\ud800 after an escaped backslash is text", why(`"\\ud800"`)),
		ok("U+D7FF and U+E000 border the surrogates", why(`"퟿"`)),

		// Depth. Level 1 is the file's own object.
		over("33 levels is too deep", deepArr(33), ReasonTooDeep, anyPointer, MaxDepth),
		over("34 levels is too deep", deepArr(34), ReasonTooDeep, anyPointer, MaxDepth),
		over("unclosed deep arrays", strings.Repeat("[", 100000), ReasonTooDeep, anyPointer, MaxDepth),
		over("unclosed deep objects", strings.Repeat(`{"a":`, 50000), ReasonTooDeep, anyPointer, MaxDepth),
		over("deep arrays inside the schema", top(`"gates": `+strings.Repeat("[", 5000)+strings.Repeat("]", 5000)), ReasonTooDeep, anyPointer, MaxDepth),
		bad("32 levels reaches the schema, which refuses the key", deepArr(32), ReasonUnknownKey, "/x"),

		// Size.
		over("a megabyte of nothing", strings.Repeat(" ", 1<<20), ReasonTooLarge, "", MaxBytes),
		over("four megabytes of arrays", strings.Repeat("[", 4<<20), ReasonTooLarge, "", MaxBytes),
		over("one long string is bounded by the file cap", why(`"`+strings.Repeat("x", MaxBytes)+`"`), ReasonTooLarge, "", MaxBytes),
	}
	runRows(t, rows)

	// A hostile pointer is bounded: a key per level, at the depth cap.
	deepKeys := strings.Repeat(`{"`+strings.Repeat("k", 48)+`":`, MaxDepth+5)
	_, _, err := Parse([]byte(deepKeys))
	if e, isErr := err.(*Error); !isErr || len(e.Pointer) > MaxDepth*(MaxIDBytes+1) {
		t.Errorf("pointer of a %d-level document: %v", MaxDepth+5, err)
	}
}
