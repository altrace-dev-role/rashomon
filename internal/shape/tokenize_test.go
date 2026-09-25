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
		{"f \"$(echo '\"' ) g", []string{"f"}, errUncertain, errUnterminated,
			"a quoted $( ) that never closes"},
		{`f "$(case x in a) :;; esac)" g`, []string{"f"}, errUncertain, nil,
			"a case pattern's ) inside a quoted $( ) closes nothing"},
		{"f \"$(: # )\n)\" g", []string{"f"}, errUncertain, nil,
			"a comment inside a quoted $( ) may hide its close"},
		{"f \"$(cat <<EOF)\" g", []string{"f"}, errUncertain, nil,
			"a here-document whose body never began"},
		{"f \"$(cat <<EOF\nx\nEOF)\" g", []string{"f"}, errUncertain, nil,
			"a delimiter line that is not only the delimiter"},
		{`f "$(echo ${x#)})" g`, []string{"f"}, errUncertain, nil,
			"a ${ } inside a quoted $( ) that may hide a paren"},
		{`f "$(: "$$(")")" g`, []string{"f"}, errUncertain, errUnterminated,
			"an even run of $ before ( inside double quotes is $$ and a (, to bash"},
		{`f "$$(: ")")" g`, []string{"f"}, errUncertain, nil,
			"the same in the outer string"},
		{`f "$(: $$(x))" g`, []string{"f"}, errUncertain, nil,
			"and inside the substitution"},
		{`f "$(: $[ ( ] ))" g`, []string{"f"}, errUncertain, nil,
			"a paren inside $[ ] is no group, and may not be one to every shell"},
		{"f \"$(: \"x\"; cat <<EOF 'a\\\nb'\nEOF\n)\" g", []string{"f"}, errUncertain, nil,
			"a backslash-newline joined inside what the shell reads as a single-quoted string"},
		{"f \"$(: \"'\"; ca\\\nse x in a) :;; esac)\" g", []string{"f"}, errUncertain, errUnterminated,
			"a backslash-newline the joining left, read as a single-quoted string"},
		{"f \"$(: \"'\" \"x\\\ny\")\" g", []string{"f"}, errUncertain, errUnterminated,
			"the same inside a nested double-quoted string"},
		{`f "$((a) (b))" g`, []string{"f"}, errUncertain, nil,
			"$(( whose first ) is not followed by another is a subshell, not arithmetic"},
		{`f "$((1;2))" g`, []string{"f"}, errUncertain, nil,
			"a ; inside $(( )) is a command, not arithmetic"},
		{`f "$(grep x; case x in a) :;; esac)" g`, []string{"f"}, errUncertain, nil,
			"case where a command begins"},
		{`f "$({ grep x } case x in a) :;; esac)" g`, []string{"f"}, errUncertain, nil,
			"case after a } that ends a group in zsh wherever it stands"},
		{`f "$([[ -n x ]] case x in a) :;; esac)" g`, []string{"f"}, errUncertain, nil,
			"case after the ]] of zsh's short if, where a command begins"},
		{`f "$(2>f case x in a) :;; esac)" g`, []string{"f"}, errUncertain, nil,
			"case after a redirect's fd and target"},
		{`f "$(( #))" g`, []string{"f"}, errUncertain, nil,
			"a # after a blank inside $(( )) would begin a comment in a $( ( )"},
		{`f "$((case x in a)) :;; esac)" g`, []string{"f"}, errUncertain, nil,
			"case inside $(( )) would begin a command in a $( ( )"},
		{"f \"$((1\n))\" g", []string{"f"}, errUncertain, nil,
			"a newline inside $(( ))"},
		{`f "$(("1"))" g`, []string{"f"}, errUncertain, nil,
			"a quote inside $(( ))"},
		{"f \"$((`echo 1`))\" g", []string{"f"}, errUncertain, nil,
			"a backtick inside $(( ))"},
		{`f "$((\1))" g`, []string{"f"}, errUncertain, nil,
			"a backslash inside $(( ))"},
		{`f "$(( ${x#)} ))" g`, []string{"f"}, errUncertain, nil,
			"a ${ } hiding a paren inside $(( ))"},
		{`f "$(: $[ { ] ))" g`, []string{"f"}, errUncertain, nil,
			"a brace inside $[ ]"},
		{`f "$(: $[a[1] ( ] ))" g`, []string{"f"}, errUncertain, nil,
			"a paren inside $[ ] after a nested [ ]"},
		{`f "$(( ${x#(} ) ))" g`, []string{"f"}, errUncertain, nil,
			"a ${ } hiding a ( inside $(( ))"},
		{`f "$(( $$(x ")") ))" g`, []string{"f"}, errUncertain, nil,
			"an even run of $ before ( inside $(( ))"},
		{`f "$(: "$[ " ]")" g`, []string{"f"}, errUncertain, errUnterminated,
			"a quote inside $[ ] inside a nested double-quoted string"},
		{"f \"$(cat <<'EOF'\nEOF \\\nEOF\n)\" g", []string{"f"}, errUncertain, nil,
			"a body line, as written, that begins with the delimiter and is not it"},
		{"f \"$(cat <<EOF\na \\\nb\nEOF\n)\" g", []string{"f"}, errUncertain, nil,
			"a backslash-newline inside an unquoted here-document's body"},
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

// TestTokenizeProgramNestsQuotesInsideDoubleQuotedSubstitution: for the
// program search, a $( ) inside double quotes is a context of its own, in which
// quotes nest and parens count by depth until the ) that closes it. So
// "$(echo "a;b c")" is one word, as it is to the shell, and not a string that
// ends at the first inner quote with a ; and a blank outside it. The count
// keeps the reading it has always had.
func TestTokenizeProgramNestsQuotesInsideDoubleQuotedSubstitution(t *testing.T) {
	for _, tc := range []struct {
		in    string
		want  []string
		count []string
	}{
		{`f "$(echo "a;b c")" z`,
			[]string{"f", `$(echo "a;b c")`, "z"},
			[]string{"f", "$(echo a", ";", "b", "c)", "z"}},
		{`f "$(echo '"')" () { ls; }`,
			[]string{"f", `$(echo '"')`, "(", ")", "{", "ls", ";", "}"}, nil},
		{`f "x$(a "$(b ")")" c)y" z`,
			[]string{"f", `x$(a "$(b ")")" c)y`, "z"}, nil},
		{`f "$(echo $((1+(2))) ")")" z`,
			[]string{"f", `$(echo $((1+(2))) ")")`, "z"}, nil},
		{"f \"$(echo `echo \")\"`)\" z",
			[]string{"f", "$(echo `echo \")\"`)", "z"}, nil},
		{`f "$(dirname "${BASH_SOURCE[0]}")" z`,
			[]string{"f", `$(dirname "${BASH_SOURCE[0]}")`, "z"}, nil},
		{"f \"$(cat <<'EOF'\nit's (\" EOF\nEOF\n)\" z",
			[]string{"f", "$(cat <<'EOF'\nit's (\" EOF\nEOF\n)", "z"}, nil},
		{"f \"$(cat <<-EOF\n\t)\n\tEOF\n)\" z",
			[]string{"f", "$(cat <<-EOF\n\t)\n\tEOF\n)", "z"}, nil},
		{`f "$((1<<3))" z`,
			[]string{"f", `$((1<<3))`, "z"}, nil},
		{`f "$(echo $((1<<(2))) ")")" z`,
			[]string{"f", `$(echo $((1<<(2))) ")")`, "z"}, nil},
		{`f "$(grep -i case file)" z`,
			[]string{"f", `$(grep -i case file)`, "z"}, nil},
		{`f "$(for x in case; do :; done)" z`,
			[]string{"f", `$(for x in case; do :; done)`, "z"}, nil},
		{`f "$(: "$$'")" z`,
			[]string{"f", `$(: "$$'")`, "z"}, nil},
		{`f "$(( a[1] + $[b[2]] ))" z`,
			[]string{"f", `$(( a[1] + $[b[2]] ))`, "z"}, nil},
		{`f "$(( 2#101 ))" z`,
			[]string{"f", `$(( 2#101 ))`, "z"}, nil},
		{`f "$(( $(date -d 'a)' +%s) - 0 ))" z`,
			[]string{"f", `$(( $(date -d 'a)' +%s) - 0 ))`, "z"}, nil},
		// As written, x\ and EOF are two lines, and the second ends the
		// quoted here-document; joined, xEOF would not have.
		{"f \"$(cat <<'EOF'\nx\\\nEOF\n)\n\"a;b\"\nEOF\n)\" g",
			[]string{"f", "$(cat <<'EOF'\nxEOF\n)\na", ";", "b\nEOF\n)", "g"}, nil},
		{"f \"$(cat <<'EOF'\na \\\nb\nEOF\n)\" z",
			[]string{"f", "$(cat <<'EOF'\na b\nEOF\n)", "z"}, nil},
	} {
		toks, err := tokenizeProgram(tc.in)
		if err != nil {
			t.Errorf("%q: program search: %v", tc.in, err)
		}
		got := make([]string, len(toks))
		for i, tok := range toks {
			got[i] = tok.text
		}
		if !slices.Equal(got, tc.want) {
			t.Errorf("%q: program search reads %q, want %q", tc.in, got, tc.want)
		}
		if len(toks) > 1 && (!toks[1].opaque || toks[1].ticks != 0) {
			t.Errorf("%q: the substitution's word is opaque %v with %d live backticks, want opaque and none",
				tc.in, toks[1].opaque, toks[1].ticks)
		}
		if tc.count == nil {
			continue
		}
		ct, _ := tokenizeShape(tc.in)
		cgot := make([]string, len(ct))
		for i, tok := range ct {
			cgot[i] = tok.text
		}
		if !slices.Equal(cgot, tc.count) {
			t.Errorf("%q: counting tokens reads %q, want %q as it always has", tc.in, cgot, tc.count)
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
