package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzParse holds Parse to its contract on any bytes: it never panics; it
// returns a Compiled or an *Error, never both and never neither; an error's
// text is bounded and carries a digest; an accepted spec obeys every count
// cap; the parsed value keeps no byte of the input; the canonical form is a
// fixed point (re-reading it gives the same digest); and the lexer agrees
// with encoding/json on syntax, accepting only what it accepts.
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		exampleSpec, `{"rashomon_spec":1}`, ``, `{}`, `[]`, `null`, `{"rashomon_spec":2}`, `{"rashomon_spec":1.5}`,
		`{"rashomon_spec":1,"bogus":1}`, `{"rashomon_spec":1,"rashomon_spec":1}`,
		withRules(ruleWith("forbid_code", `"pattern":"(?:a{2}){3}"`)),
		withRules(ruleWith("forbid_code", `"pattern":"`+strings.Repeat("a{100}", 20)+`"`)),
		withFacts(validFact), withGates(gateWith(`"exit_codes":{"0":"pass"},"required":false`)),
		"{\"rashomon_spec\":1,\"x\":\"\xff\"}", `{"rashomon_spec":1,"x":"\ud800"}`, "\x00", "{\"a\":" + strings.Repeat("[", 40),
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		in := bytes.Clone(data)
		c, err := Parse("p", in, Untrusted)
		if (c == nil) == (err == nil) {
			t.Fatalf("Compiled %v and error %v: exactly one must be set", c != nil, err)
		}
		if err != nil {
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("error is %T", err)
			}
			if len(e.Error()) > 512 || len(e.Pointer) > 1024 {
				t.Fatalf("error text is %d bytes, pointer %d", len(e.Error()), len(e.Pointer))
			}
			if !strings.HasPrefix(e.Digest, DigestPrefix) || len(e.Digest) != len(DigestPrefix)+64 || e.Reason == "" {
				t.Fatalf("error without a digest or reason: %+v", e)
			}
			if !utf8.ValidString(e.Error()) {
				t.Fatalf("error text is not UTF-8: %q", e.Error())
			}
			if e.Reason == ReasonSyntax && utf8.Valid(data) && json.Valid(data) {
				t.Fatalf("lexer calls valid JSON a syntax error: %q", data)
			}
		} else {
			if !json.Valid(data) {
				t.Fatalf("accepted what encoding/json rejects: %q", data)
			}
			if len(c.Gates)+len(c.CICommands) > MaxGates || len(c.Rules) > MaxRules || len(c.Facts) > MaxFacts {
				t.Fatalf("accepted over a count cap: %d gates %d ci %d rules %d facts", len(c.Gates), len(c.CICommands), len(c.Rules), len(c.Facts))
			}
			if c.Digest != c.Sources[0].Digest || len(c.Sources) != 1 {
				t.Fatalf("digest %s, sources %+v", c.Digest, c.Sources)
			}
			// Nothing of the input is kept: scribble over a copy and compare.
			before := fmt.Sprintf("%+v", *c)
			for i := range in {
				in[i] = 0xff
			}
			if after := fmt.Sprintf("%+v", *c); after != before {
				t.Fatal("the parsed spec aliases the input")
			}
		}
		// The canonical form is a fixed point.
		if n, lerr := lex(data); lerr == nil {
			canon := appendJCS(nil, n)
			n2, lerr2 := lex(canon)
			if lerr2 != nil {
				t.Fatalf("canonical form does not lex: %v: %q", lerr2, canon)
			}
			if again := appendJCS(nil, n2); !bytes.Equal(canon, again) {
				t.Fatalf("canonical form is not a fixed point:\n%q\n%q", canon, again)
			}
		}
	})
}
