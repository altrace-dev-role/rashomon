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
