package shape

import (
	"slices"
	"testing"
)

// TestTokenize pins the split exactly, because the split is what the store
// records: argc is a count of these tokens and program is the first of them.
// An approximate case here is an approximate record there.
func TestTokenize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		drop bool
		want []string
		err  error
	}{
		{name: "empty line", in: ""},
		{name: "plain words", in: "ls -l /tmp", want: []string{"ls", "-l", "/tmp"}},
		{name: "runs of whitespace", in: "  ls \t  -l  ", want: []string{"ls", "-l"}},

		{name: "single quotes hold spaces", in: "echo 'one two'", want: []string{"echo", "one two"}},
		{name: "single quotes join the token", in: "pre'fix'post", want: []string{"prefixpost"}},
		{name: "empty single-quoted string is a token", in: "echo ''", want: []string{"echo", ""}},

		{name: "double quote escapes", in: `echo "a\"b\\c\$d"`, want: []string{"echo", `a"b\c$d`}},
		{name: "double quote keeps a backslash before an ordinary character", in: `echo "a\nb"`, want: []string{"echo", `a\nb`}},
		{name: "empty double-quoted string is a token", in: `echo ""`, want: []string{"echo", ""}},

		{name: "backslash outside quotes escapes a space", in: `echo a\ b`, want: []string{"echo", "a b"}},
		{name: "backslash-newline is a line continuation", in: "echo a\\\nb", want: []string{"echo", "ab"}},

		{name: "pipe splits tokens", in: "ls|wc", want: []string{"ls", "|", "wc"}},
		{name: "doubled ampersand is one token", in: "a&&b", want: []string{"a", "&&", "b"}},
		{name: "doubled redirect is one token", in: "x>>y", want: []string{"x", ">>", "y"}},
		{name: "parentheses split", in: "(sub)", want: []string{"(", "sub", ")"}},
		{name: "semicolon splits", in: "a;b", want: []string{"a", ";", "b"}},

		{name: "leading assignments are dropped", in: "FOO=bar BAZ=qux make -j4", drop: true, want: []string{"make", "-j4"}},
		{name: "an assignment after the program is an argument", in: "env FOO=bar", drop: true, want: []string{"env", "FOO=bar"}},
		{name: "a line of nothing but assignments names no program", in: "FOO=bar", drop: true},

		{name: "unterminated single quote", in: "echo 'oops", want: []string{"echo"}, err: errUnterminated},
		{name: "unterminated single quote inside a token", in: "ec'ho", err: errUnterminated},
		{name: "unterminated double quote", in: `echo "oops`, want: []string{"echo"}, err: errUnterminated},
		{name: "unterminated double quote at end of input", in: `echo "`, want: []string{"echo"}, err: errUnterminated},
		{name: "trailing backslash", in: `echo a\`, want: []string{"echo", "a"}, err: errUnterminated},
		// The program search's uncertain stop is its own: the count's
		// tokenizer reports an unterminated quote as it always has.
		{name: "unterminated quote after an expansion", in: `echo "$(x`, want: []string{"echo"}, err: errUnterminated},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shaped, err := tokenizeShape(tc.in)
			got := make([]string, len(shaped))
			for k, s := range shaped {
				got[k] = s.text
			}
			if tc.drop {
				// Through programToken, which owns this behaviour now: the
				// assignment prefix is skipped as part of finding the
				// program, not by a separate pass.
				if i, ok := programToken(shaped, false); ok {
					got = got[i:]
				} else {
					got = nil
				}
			}
			if err != tc.err {
				t.Errorf("error is %v, want %v", err, tc.err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("tokens are %q, want %q", got, tc.want)
			}
		})
	}
}

// TestTokenizeGlued pins the one bit the tokenizer used to throw away: whether
// anything separated a token from the one before it. `>|` is one operator and
// `> |` two, and `acme-<1-3>` is one word to zsh only because nothing divides
// its pieces; the token text is the same either way.
func TestTokenizeGlued(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []bool
	}{
		{">| f", []bool{false, true, false}},
		{"> | f", []bool{false, false, false}},
		{">\n| f", []bool{false, false, false}},
		{">\\\n| f", []bool{false, true, false}}, // a continuation is not a separator
		{"a<(b) c", []bool{false, true, true, true, true, false}},
		{"ls>out", []bool{false, true, true}},
		{"'a'b \"c\"", []bool{false, false}},
	} {
		ts, err := tokenizeShape(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		got := make([]bool, len(ts))
		for i, tok := range ts {
			got[i] = tok.glued
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("glued for %q is %v, want %v", tc.in, got, tc.want)
		}
	}
}

// TestTokenizeProgramReadsANSICQuotes: the program search reads $'...' to its
// first unescaped quote, as the shell does, while the count keeps the reading
// it has always had. Read to the first quote of any kind, a $'...' holding an
// escaped quote is an open quote, and the () after it -- which makes the line
// a function definition -- is never seen.
func TestTokenizeProgramReadsANSICQuotes(t *testing.T) {
	const line = "f $'\\'' () { ls; }"
	prog, err := tokenizeProgram(line)
	if err != nil {
		t.Fatalf("tokenizeProgram: %v", err)
	}
	got := make([]string, len(prog))
	for i, tok := range prog {
		got[i] = tok.text
	}
	if want := []string{"f", "$\\'", "(", ")", "{", "ls", ";", "}"}; !slices.Equal(got, want) {
		t.Errorf("program tokens are %q, want %q", got, want)
	}
	if _, err := tokenizeShape(line); err != errUnterminated {
		t.Errorf("counting tokens: error is %v, want the unterminated quote argc has always seen", err)
	}
	// Only an unquoted, unescaped $ opens one, and only when bash and zsh
	// agree that it does. Read as $'...', each of the first four would run to
	// the end of the line unterminated; read as the ordinary single quote it
	// is, '\' closes. An odd run of $ ends in a $' to both shells.
	for _, tc := range []struct {
		in   string
		want error
		why  string
	}{
		{`\$'\' x`, nil, "an escaped $ opens nothing"},
		{`"$"'\' x`, nil, "a quoted $ opens nothing"},
		{`$x '\' y`, nil, "a $ earlier in the line opens nothing"},
		{`'\' x`, nil, "a quote at the start of the line has no $ before it"},
		{`$$$'\'' x`, nil, "$$ and then $'...', to both shells"},
		{`$$'\' x'`, errUncertain, "bash reads $$ and a plain quote, zsh $ and $'...'"},
	} {
		if _, err := tokenizeProgram(tc.in); err != tc.want {
			t.Errorf("%q: error is %v, want %v: %s", tc.in, err, tc.want, tc.why)
		}
	}
}

// TestTokenizeProgramStopsWhereItsReadingIsUncertain: the program search's
// tokenizer stops, with errUncertain and the tokens completed before the stop,
// where it can no longer say how the shell reads on. The count's tokenizer
// reads the same lines as it always has.
func TestTokenizeProgramStopsWhereItsReadingIsUncertain(t *testing.T) {
	for _, tc := range []struct {
		in        string
		want      []string
		program   error
		count     error
		reasoning string
	}{
		{`f "$(echo '"')" () { ls; }`, []string{"f"}, errUncertain, errUnterminated,
			"the quote may be inside the $( ), where the shell reads it"},
		{`f "$(a)" 'b () { ls; }`, []string{"f", "$(a)"}, errUncertain, errUnterminated,
			"an expansion in an earlier word"},
		{`f "$(a)" \`, []string{"f", "$(a)"}, errUncertain, errUnterminated,
			"a trailing backslash after an expansion"},
		{`echo "oops`, []string{"echo"}, errUnterminated, errUnterminated,
			"with no expansion before it, an unterminated quote is one to the shell too"},
		{`ls; echo $$'x'`, []string{"ls", ";", "echo"}, errUncertain, nil,
			"bash and zsh read $$' differently"},
	} {
		toks, err := tokenizeProgram(tc.in)
		got := make([]string, len(toks))
		for i, tok := range toks {
			got[i] = tok.text
		}
		if err != tc.program || !slices.Equal(got, tc.want) {
			t.Errorf("%q: program search reads %q, %v; want %q, %v: %s", tc.in, got, err, tc.want, tc.program, tc.reasoning)
		}
		if _, err := tokenizeShape(tc.in); err != tc.count {
			t.Errorf("%q: counting tokens: error is %v, want %v", tc.in, err, tc.count)
		}
	}
}

// TestTokenizeTicks pins which backticks the tokenizer counts as opening or
// closing a command substitution: unquoted, or inside double quotes, and not
// escaped. A quoted or escaped backtick is in the word's text all the same,
// and opens nothing.
func TestTokenizeTicks(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want []int
	}{
		{"`a b`", []int{1, 1}},
		{"`a`", []int{2}},
		{"\"`a b`\"", []int{2}},
		{"\"`\"", []int{1}},
		{"'`'", []int{0}},
		{"\\`", []int{0}},
		{"\"\\`\"", []int{0}},
	} {
		ts, err := tokenizeShape(tc.in)
		if err != nil {
			t.Fatalf("%q: %v", tc.in, err)
		}
		got := make([]int, len(ts))
		for i, tok := range ts {
			got[i] = tok.ticks
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("ticks for %q are %v, want %v", tc.in, got, tc.want)
		}
	}
}
