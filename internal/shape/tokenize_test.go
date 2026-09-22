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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, marks, quoted, err := tokenizeQuoted(tc.in)
			if tc.drop {
				// Through programToken, which owns this behaviour now: the
				// assignment prefix is skipped as part of finding the
				// program, not by a separate pass.
				if i, ok := programToken(got, marks, quoted); ok {
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
