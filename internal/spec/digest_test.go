package spec

import (
	"strings"
	"testing"
)

// katDoc is a spec with every awkward string case in it: a quote, a backslash,
// a two-byte and a four-byte rune, U+2028, "/" written as an escape, and the
// three characters Go's encoding/json escapes and RFC 8785 does not. Its keys
// are deliberately out of order and its spacing is not canonical.
//
// katDigest was computed by a second route that shares no code with the
// package: the script below, an RFC 8785 serializer written for this note
// (escape only the quote, the backslash and C0; sort keys by UTF-16 code
// units; integers as decimal digits), run on katDoc saved as kat.json with
// "python3 -I jcs.py kat.json". It prints the canonical text and
//
//	sha256:7ae16c1d49796b24552e5e104be16095de8fa19473931619632feb826025fcec
//
// The minimal document {"rashomon_spec":1} was checked a third way:
// printf '%s' '{"rashomon_spec":1}' | sha256sum.
//
//	# A second route to the digest: a small RFC 8785 serializer, not json.dumps.
//	import hashlib, json, sys
//
//	def esc(s):
//	    out = ['"']
//	    for ch in s:
//	        o = ord(ch)
//	        if ch == '"': out.append('\\"')
//	        elif ch == '\\': out.append('\\\\')
//	        elif ch == '\b': out.append('\\b')
//	        elif ch == '\f': out.append('\\f')
//	        elif ch == '\n': out.append('\\n')
//	        elif ch == '\r': out.append('\\r')
//	        elif ch == '\t': out.append('\\t')
//	        elif o < 0x20: out.append('\\u%04x' % o)
//	        else: out.append(ch)
//	    out.append('"')
//	    return ''.join(out)
//
//	def u16(s):  # UTF-16 code units, the RFC's key order
//	    b = s.encode('utf-16-be')
//	    return [int.from_bytes(b[i:i+2], 'big') for i in range(0, len(b), 2)]
//
//	def jcs(v):
//	    if isinstance(v, dict):
//	        return '{' + ','.join(esc(k) + ':' + jcs(v[k]) for k in sorted(v, key=u16)) + '}'
//	    if isinstance(v, list): return '[' + ','.join(jcs(x) for x in v) + ']'
//	    if isinstance(v, str): return esc(v)
//	    if v is True: return 'true'
//	    if v is False: return 'false'
//	    if v is None: return 'null'
//	    assert isinstance(v, int)
//	    return str(v)
//
//	doc = json.load(open(sys.argv[1], encoding='utf-8'))
//	c = jcs(doc)
//	print(c)
//	print('sha256:' + hashlib.sha256(c.encode('utf-8')).hexdigest())
const (
	katDoc = `{
  "rules": [{"id": "no-x", "kind": "forbid_code", "pattern": "a\\.b<c>&d",
    "why": "quote \" backslash \\ \u00e9 \ud83d\ude00 < & > \u2028 \/",
    "hard_control": "CI"}],
  "rashomon_spec": 1,
  "gates": [{"argv": ["go", "test"], "kind": "test", "id": "unit", "required": false}]
}`
	katDigest = "sha256:7ae16c1d49796b24552e5e104be16095de8fa19473931619632feb826025fcec"
	minDigest = "sha256:e736acc287246d179f9da3baa6ba9fd05849e1ed4ca02ff5befca3c4899436ac"
)

func digestOf(t *testing.T, in string) string {
	t.Helper()
	_, d, err := Parse([]byte(in))
	if err != nil {
		t.Fatalf("Parse(%.60q): %v", in, err)
	}
	return d
}

// TestVER39_DigestCanonical: the digest is "sha256:" and the hex SHA-256 of
// the RFC 8785 form of the parsed document, so key order, whitespace and the
// spelling of an escape do not change it, and any change of content does
// (archive 20.2.8, Digest).
func TestVER39_DigestCanonical(t *testing.T) {
	t.Run("known answers from a second route", func(t *testing.T) {
		if got := digestOf(t, katDoc); got != katDigest {
			t.Errorf("digest %s, want %s", got, katDigest)
		}
		if got := digestOf(t, `{"rashomon_spec":1}`); got != minDigest {
			t.Errorf("digest %s, want %s", got, minDigest)
		}
	})
	t.Run("equal documents, equal digest", func(t *testing.T) {
		same := map[string]string{
			"whitespace removed": `{"rules":[{"id":"no-x","kind":"forbid_code","pattern":"a\\.b<c>&d","why":"quote \" backslash \\ é 😀 < & > ` + "\u2028" + ` /","hard_control":"CI"}],"rashomon_spec":1,"gates":[{"argv":["go","test"],"kind":"test","id":"unit","required":false}]}`,
			"tabs and CRLF": "{\r\n\t\"rashomon_spec\" :\t1 ,\r\n\t\"gates\"\t:\t[ {\"id\":\"unit\",\r\n\"kind\":\"test\",\"argv\":[\"go\",\t\"test\"],\"required\":false} ],\r\n" +
				"\t\"rules\":[{\"id\":\"no-x\",\"kind\":\"forbid_code\",\"pattern\":\"a\\\\.b<c>&d\",\"why\":\"quote \\\" backslash \\\\ é 😀 < & > \u2028 /\",\"hard_control\":\"CI\"}]\r\n}\r\n",
			"keys reversed": `{"gates": [{"required": false, "kind": "test", "id": "unit", "argv": ["go", "test"]}], "rashomon_spec": 1, "rules": [{"hard_control": "CI", "why": "quote \" backslash \\ é 😀 < & > ` + "\u2028" + ` /", "pattern": "a\\.b<c>&d", "kind": "forbid_code", "id": "no-x"}]}`,
		}
		for name, in := range same {
			if got := digestOf(t, in); got != katDigest {
				t.Errorf("%s: digest %s, want %s", name, got, katDigest)
			}
		}
	})
	t.Run("a different spelling of one string is the same string", func(t *testing.T) {
		a := digestOf(t, withRule(kv{"why", `"caf\u00e9 \/ x"`}))
		b := digestOf(t, withRule(kv{"why", `"café / x"`}))
		if a != b {
			t.Errorf("escaped %s, literal %s", a, b)
		}
	})
	t.Run("different content, different digest", func(t *testing.T) {
		seen := map[string]string{}
		for name, in := range map[string]string{
			"base":              withRule(),
			"why changed":       withRule(kv{"why", `"x"`}),
			"id changed":        withRule(kv{"id", `"r2"`}),
			"key added":         withRule(kv{"alternative", `"a"`}),
			"gate instead":      withGate(),
			"array order":       top(`"ci_commands": [["a"], ["b"]]`),
			"array order other": top(`"ci_commands": [["b"], ["a"]]`),
			"false vs true":     withGate(kv{"required", `false`}),
			"true":              withGate(kv{"required", `true`}),
		} {
			d := digestOf(t, in)
			if prev, dup := seen[d]; dup {
				t.Errorf("%s and %s share digest %s", name, prev, d)
			}
			seen[d] = name
		}
	})
	t.Run("an absent default is not a present one", func(t *testing.T) {
		// The digest is over the document as written, not over the File after
		// defaults, so "required": true and no "required" are two documents.
		if digestOf(t, withGate()) == digestOf(t, withGate(kv{"required", `true`})) {
			t.Error("the digest followed the defaults, not the document")
		}
	})
	t.Run("digest of a schema-invalid document", func(t *testing.T) {
		// One flag per digest needs a digest for a spec that is refused, and
		// it must be the same one the equivalent valid layout would have.
		bad := top(`"extra": 1`)
		f, d, err := Parse([]byte(bad))
		if err == nil || f != nil {
			t.Fatalf("want a refusal, got %v %v", f, err)
		}
		reordered := `{"extra": 1, "rashomon_spec": 1}`
		if _, d2, _ := Parse([]byte(reordered)); d == "" || d != d2 {
			t.Errorf("digests %q and %q", d, d2)
		}
	})
	t.Run("no digest when there is no document", func(t *testing.T) {
		for _, in := range []string{"", "{", `{"a":1,"a":2}`, sized(MaxBytes + 1), "[" + strings.Repeat("[", 100)} {
			if _, d, err := Parse([]byte(in)); err == nil || d != "" {
				t.Errorf("%.20q: digest %q, err %v; a document that did not read has no digest", in, d, err)
			}
		}
	})
}

// TestVER39b_JCSMatchesRFC8785 pins appendJCS to the examples RFC 8785
// section 3.2.3 gives: keys in UTF-16 code unit order (a four-byte rune sorts
// before U+E000 and above, which code point order would not do), the escapes
// section 3.2.2.2 fixes, and integers as digits.
func TestVER39b_JCSMatchesRFC8785(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"RFC 8785 3.2.3 key order",
			`{"\u20ac": "Euro Sign", "\r": "Carriage Return", "\ufb33": "Hebrew Letter Dalet With Dagesh", "1": "One", "\ud83d\ude00": "Emoji: Grinning Face", "\u0080": "Control", "\u00f6": "Latin Small Letter O With Diaeresis"}`,
			"{\"\\r\":\"Carriage Return\",\"1\":\"One\",\"\u0080\":\"Control\",\"\u00f6\":\"Latin Small Letter O With Diaeresis\",\"\u20ac\":\"Euro Sign\",\"\U0001F600\":\"Emoji: Grinning Face\",\"\ufb33\":\"Hebrew Letter Dalet With Dagesh\"}"},
		{"escapes", `"\u0001\b\f\n\r\t\"\\\u007f<>&\u2028\/"`, "\"\\u0001\\b\\f\\n\\r\\t\\\"\\\\\x7f<>&\u2028/\""},
		{"C0 in lower-case hex", `"\u001f\u0000"`, `"\u001f\u0000"`},
		{"integers", `[0, -0, 7, -7, 9007199254740991, -9007199254740991]`, `[0,0,7,-7,9007199254740991,-9007199254740991]`},
		{"no space, literals", ` { "a" : [ true , false , null ] , "b" : { } } `, `{"a":[true,false,null],"b":{}}`},
		{"nested keys sort at each level", `{"b": {"z": 1, "a": 2}, "a": []}`, `{"a":[],"b":{"a":2,"z":1}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n, err := readTree([]byte(c.in))
			if err != nil {
				t.Fatal(err)
			}
			if got := string(appendJCS(nil, n)); got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}
