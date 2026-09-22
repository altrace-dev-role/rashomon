package shape

import (
	"encoding/json"
	"testing"
)

// TestProgramIsAProgram: the program field names a program, or is null.
//
// Measured on a real session before this held: three of twenty-six
// declarations recorded `&&`, `(` or nothing as the program -- about one in
// eight of the single field that says what ran. A value that cannot be a
// program is worse than no value, because null already means "we could not
// tell" and reads that way, while `&&` reads as a fact.
func TestProgramIsAProgram(t *testing.T) {
	for _, tc := range []struct {
		name string
		cmd  string
		want string // "" means the program must be null
		why  string
	}{
		{name: "subshell", cmd: "( cd /tmp && ls )", want: "cd",
			why: "recorded `(` before this"},
		{name: "brace group", cmd: "{ ls; }", want: "ls",
			why: "`{` is a shell keyword the tokenizer does not mark as a metacharacter"},
		{name: "leading operator", cmd: "&& go build", want: "go"},
		{name: "leading pipe", cmd: "| grep x", want: "grep"},
		{name: "leading semicolon", cmd: "; ls", want: "ls"},
		{name: "redirect first", cmd: "> out.txt ls", want: "ls",
			why: "the filename after a redirect is not the program"},

		// Operators the tokenizer emits as TWO tokens, because it doubles a
		// metacharacter only when the next byte is the same byte. Skipping
		// "the operator and one token" here skips the operator's second half
		// and keeps the filename -- which recorded customer-list.csv as the
		// program of the first line below. A regression against the
		// behaviour before this function, which recorded `>`: useless, but no
		// content.
		{name: "redirect >&", cmd: ">& /home/alice/customer-list.csv ls", want: "ls"},
		{name: "redirect >|", cmd: ">| /tmp/acme-merger-notes.txt ls", want: "ls"},
		{name: "redirect <>", cmd: "<> /var/db/prod.sqlite ls", want: "ls"},
		{name: "here-string", cmd: `<<< "hunter2-password" cat`, want: "cat",
			why: "a here-string's word is literal text, not even a filename"},
		{name: "fd duplication", cmd: ">&2 echo hi", want: "echo"},
		{name: "input fd duplication", cmd: "<&3 read x", want: "read"},
		{name: "redirect &>", cmd: "&> /tmp/out.txt ls", want: "ls"},
		{name: "redirect &>>", cmd: "&>> /tmp/out.txt ls", want: "ls"},
		{name: "heredoc", cmd: "<<EOF cat", want: "cat"},
		// A `(` after `<` opens a process substitution; it is not a
		// redirect's target and must not be consumed as one. Doing so skips
		// `cat` as the "target" and reads the next word -- the path -- as
		// the program. cat is the only thing on this line that runs.
		{name: "process substitution", cmd: "<(cat /home/alice/secret.csv) ls", want: "cat",
			why: "the path is cat's argument"},
		// A separator where a target should be is a syntax error, and the
		// word after it is in command position, not a target.
		{name: "redirect then separator", cmd: "> ; ls /home/alice/secret.csv", want: "ls"},
		// Nor is another operator: the second redirect owns the path.
		{name: "redirect then redirect", cmd: "> > /home/alice/secret.csv ls", want: "ls"},
		{name: "assignment then subshell", cmd: "FOO=1 ( ls )", want: "ls"},

		// Unchanged behaviour, asserted so the fix cannot quietly move it.
		{name: "ordinary command", cmd: "cd /tmp && go build", want: "cd"},
		{name: "assignments skipped", cmd: "FOO=bar BAZ=qux make -j4", want: "make"},
		{name: "assignment after the program is an argument", cmd: "env FOO=bar", want: "env"},
		{name: "path is reduced to its base", cmd: "/usr/local/bin/go build", want: "go"},

		// Null is the honest answer, and must stay null rather than become
		// a metacharacter.
		{name: "nothing but assignments", cmd: "FOO=bar"},
		{name: "nothing but operators", cmd: "&& ; |"},
		{name: "empty", cmd: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]string{"command": tc.cmd})
			if err != nil {
				t.Fatal(err)
			}
			got := Derive("Bash", raw, []byte("key"))

			if tc.want == "" {
				if got.Program != nil {
					t.Errorf("program for %q is %q, want null", tc.cmd, *got.Program)
				}
				return
			}
			if got.Program == nil {
				t.Fatalf("program for %q is null, want %q", tc.cmd, tc.want)
			}
			if *got.Program != tc.want {
				msg := ""
				if tc.why != "" {
					msg = "\n  " + tc.why
				}
				t.Errorf("program for %q is %q, want %q%s", tc.cmd, *got.Program, tc.want, msg)
			}
		})
	}
}

// TestProgramIONumberIsAKnownGap pins, rather than fixes, one place the
// invariant above does not hold: an fd number written against its redirect,
// `2> out.txt ls`, records "2" as the program.
//
// The tokenizer does not keep whether a word ended at a metacharacter or at
// whitespace, so `2> out.txt ls` (fd 2 redirected, program ls) and
// `2 > out.txt ls` (a program named 2) reach this package as the same tokens.
// Reading the first into both would take an argument for the program whenever
// the second was meant. Fixing it needs that bit from the tokenizer. It is not
// new: the first token was "2" before programToken existed too.
//
// This test exists so the gap is closed on purpose. The day it fails, correct
// TestProgramIsAProgram's claim as well as this assertion.
func TestProgramIONumberIsAKnownGap(t *testing.T) {
	raw, err := json.Marshal(map[string]string{"command": "2> out.txt ls"})
	if err != nil {
		t.Fatal(err)
	}
	got := Derive("Bash", raw, []byte("key"))
	if got.Program == nil || *got.Program != "2" {
		t.Errorf("program for an fd-prefixed redirect is %v; this test pins \"2\" -- if that changed on purpose, update the invariant", got.Program)
	}
}

// TestArgcExcludesLeadingAssignments pins what argc counts, which changed
// without notice once: finding the program by skipping a `FOO=bar` prefix
// stopped dropping that prefix from the tokens argc is taken over, and
// `FOO=bar BAZ=qux make -j4` went from 2 to 4 while its digest stayed the same.
// The same command then counts differently depending on which build recorded
// it, in a field the store schema publishes.
//
// argc is over the whole line after a leading assignment prefix: operators and
// every stage of a pipeline count, and an assignment after the program is an
// argument like any other.
func TestArgcExcludesLeadingAssignments(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		want int
	}{
		{"FOO=bar BAZ=qux make -j4", 2},
		{"FOO=1 ls", 1},
		{"FOO=bar", 0},
		{"env FOO=bar", 2},
		{"ls | wc -l", 4},
		{"( cd /tmp && ls )", 6},
	} {
		raw, err := json.Marshal(map[string]string{"command": tc.cmd})
		if err != nil {
			t.Fatal(err)
		}
		got := Derive("Bash", raw, []byte("key"))
		if got.Argc == nil {
			t.Errorf("argc for %q is null, want %d", tc.cmd, tc.want)
			continue
		}
		if *got.Argc != tc.want {
			t.Errorf("argc for %q is %d, want %d", tc.cmd, *got.Argc, tc.want)
		}
	}
}
