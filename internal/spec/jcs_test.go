package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// The expected strings below come from RFC 8785 itself (section 3.2.3's
// sorting example) and from a second implementation, a standalone script
// that sorts by UTF-16 code unit and writes the RFC's string escapes.

func canonOf(t *testing.T, in string) string {
	t.Helper()
	n, err := lex([]byte(in))
	if err != nil {
		t.Fatalf("lex(%q): %v", in, err)
	}
	return string(appendJCS(nil, n))
}

// TestSP11_JCSSortsByUTF16: keys sort by UTF-16 code unit, so U+1F600
// (surrogates D83D DE00) comes before U+FB33, where a code point sort would
// put it after.
func TestSP11_JCSSortsByUTF16(t *testing.T) {
	in := `{"\u20ac":"Euro Sign","\r":"Carriage Return","\ufb33":"Hebrew Letter Dalet With Dagesh","1":"One","\ud83d\ude00":"Emoji: Grinning Face","\u0080":"Control","\u00f6":"Latin Small Letter O With Diaeresis"}`
	want := "{\"\\r\":\"Carriage Return\",\"1\":\"One\",\"\u0080\":\"Control\",\"\u00f6\":\"Latin Small Letter O With Diaeresis\"," +
		"\"\u20ac\":\"Euro Sign\",\"\U0001F600\":\"Emoji: Grinning Face\",\"\ufb33\":\"Hebrew Letter Dalet With Dagesh\"}"
	got := canonOf(t, in)
	if got != want {
		t.Errorf("canonical form\n got %q\nwant %q", got, want)
	}
	sum := sha256.Sum256([]byte(got))
	if h := hex.EncodeToString(sum[:]); h != "5e321556d22018a9656991a9e94f77ec175fa193e52a2429d312f8419ec8b08c" {
		t.Errorf("sha256 = %s", h)
	}
	// Nested objects sort at every level; arrays keep their order.
	if got, want := canonOf(t, `{"b":[{"z":1,"a":2},{"y":[3,2,1]}],"a":{}}`), `{"a":{},"b":[{"a":2,"z":1},{"y":[3,2,1]}]}`; got != want {
		t.Errorf("nested: got %s, want %s", got, want)
	}
}

// TestSP11_JCSStrings: RFC 8785 section 3.2.2.2. Only quote, backslash and
// the C0 controls are escaped, the short forms where JSON has them and
// lower-case \u00xx otherwise; DEL, U+2028, the HTML characters and every
// non-ASCII rune are written as themselves. That is what separates it from
// the standard marshaller, which escapes <, >, & and U+2028.
func TestSP11_JCSStrings(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"plain", `"plain"`},
		{"a b", `"a b"`},
		{"", `""`},
		{"a\"b", `"a\"b"`},
		{"a\\b", `"a\\b"`},
		{"\b\f\n\r\t", `"\b\f\n\r\t"`},
		{"\x00\x01\x0b\x1f", `"\u0000\u0001\u000b\u001f"`},
		{"\x7f", "\"\x7f\""},
		{"</>&", `"</>&"`},
		{"\u2028\u2029", "\"\u2028\u2029\""},
		{"é😀", "\"é😀\""},
	} {
		if got := string(appendJCSString(nil, tc.in)); got != tc.want {
			t.Errorf("appendJCSString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Through the lexer: an escaped spelling and a raw spelling are one string.
	if a, b := canonOf(t, `["\u00e9\u003c"]`), canonOf(t, "[\"é<\"]"); a != b || a != "[\"é<\"]" {
		t.Errorf("escaped %q, raw %q", a, b)
	}
}

// TestSP11_JCSScalars: integers print as their decimal digits ("-0" is "0"),
// literals as themselves, empty containers as themselves.
func TestSP11_JCSScalars(t *testing.T) {
	got := canonOf(t, ` [ 0 , -0 , 1 , -17 , 9007199254740991 , -9007199254740991 , true , false , null , [ ] , { } ] `)
	want := `[0,0,1,-17,9007199254740991,-9007199254740991,true,false,null,[],{}]`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
