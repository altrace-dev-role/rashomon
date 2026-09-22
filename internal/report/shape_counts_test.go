package report

import (
	"fmt"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

func bashCall(id, program, verb string) store.Declaration {
	d := store.Declaration{ToolUseID: id, ToolName: "Bash", SessionID: "s",
		Shape: shape.Shape{VerbClass: verb}}
	if program != "" {
		p := program
		d.Shape.Program = &p
	}
	return d
}

// TestByProgramAndVerb: the report says what ran, not just which door it came
// through.
//
// "by tool: Bash 23" answers nothing -- Bash is not a thing anyone did. The
// program and the verb class are on every record already; before this they
// were reachable only from --chain, one call at a time.
func TestByProgramAndVerb(t *testing.T) {
	sess := build(&store.Run{Declarations: []store.Declaration{
		bashCall("a", "git", "vcs"),
		bashCall("b", "git", "vcs"),
		bashCall("c", "go", "package"),
		{ToolUseID: "d", ToolName: "Read", SessionID: "s", Shape: shape.Shape{VerbClass: "read"}},
	}})

	d := sess.Declarations
	if d.ByProgram["git"] != 2 || d.ByProgram["go"] != 1 || len(d.ByProgram) != 2 {
		t.Errorf("by_program = %v, want {git: 2, go: 1}", d.ByProgram)
	}
	if d.ByVerbClass["vcs"] != 2 || d.ByVerbClass["package"] != 1 || d.ByVerbClass["read"] != 1 {
		t.Errorf("by_verb_class = %v, want {vcs: 2, package: 1, read: 1}", d.ByVerbClass)
	}
	if d.ProgramsUnknown != 0 {
		t.Errorf("programs_unknown = %d, want 0: every shell call named a program", d.ProgramsUnknown)
	}
}

// TestProgramsUnknownIsCountedApart: a shell call whose program could not be
// told is counted, and counted BESIDE the programs rather than among them.
//
// "unknown" is not a program. A reader scanning the list for what ran must
// not meet it sitting between git and go as though it were one more command.
func TestProgramsUnknownIsCountedApart(t *testing.T) {
	sess := build(&store.Run{Declarations: []store.Declaration{
		bashCall("a", "git", "vcs"),
		bashCall("b", "", "execute"), // would not tokenize
	}})

	d := sess.Declarations
	if d.ProgramsUnknown != 1 {
		t.Errorf("programs_unknown = %d, want 1", d.ProgramsUnknown)
	}
	for name := range d.ByProgram {
		if name == "" || name == "unknown" {
			t.Errorf("by_program carries %q, which is not a program", name)
		}
	}
	if got := programs(d.ByProgram, d.ProgramsUnknown); got != "git 1 (and 1 shell call(s) whose program could not be told)" {
		t.Errorf("rendered as %q", got)
	}
}

// TestNonShellToolsAreNotCountedAsMissingAProgram: only a shell tool has a
// program to miss. Counting Read or WebFetch as "unknown" would report a gap
// where there is nothing to know.
func TestNonShellToolsAreNotCountedAsMissingAProgram(t *testing.T) {
	sess := build(&store.Run{Declarations: []store.Declaration{
		{ToolUseID: "a", ToolName: "Read", SessionID: "s", Shape: shape.Shape{VerbClass: "read"}},
		{ToolUseID: "b", ToolName: "WebFetch", SessionID: "s", Shape: shape.Shape{VerbClass: "network"}},
	}})
	if got := sess.Declarations.ProgramsUnknown; got != 0 {
		t.Errorf("programs_unknown = %d, want 0: neither tool runs a program", got)
	}
	if got := programs(sess.Declarations.ByProgram, 0); got != none {
		t.Errorf("rendered as %q, want %q", got, none)
	}
}

// TestProgramsAllUntoldSaysSo: when no shell call's program could be told, the
// line says it could not be told -- not that the calls named none.
//
// programs_unknown counts every Bash call whose program could not be found,
// including one whose first word an unterminated quote cut off. "named none"
// is true of only some of those, and asserts a fact about a line we could not
// read.
func TestProgramsAllUntoldSaysSo(t *testing.T) {
	sess := build(&store.Run{Declarations: []store.Declaration{
		bashCall("a", "", "execute"),
		bashCall("b", "", "execute"),
	}})
	d := sess.Declarations
	want := "none (2 shell call(s) whose program could not be told)"
	if got := programs(d.ByProgram, d.ProgramsUnknown); got != want {
		t.Errorf("rendered as %q, want %q", got, want)
	}
}

// TestByProgramLineIsBounded: a session that ran many distinct programs does
// not rebuild the unreadable line the id lists once were. The count and the
// pointer to --json stay on the line; the JSON keeps every program.
func TestByProgramLineIsBounded(t *testing.T) {
	var decls []store.Declaration
	for i := 0; i < 60; i++ {
		decls = append(decls, bashCall(fmt.Sprintf("t%02d", i), fmt.Sprintf("program-%02d", i), "execute"))
	}
	sess := build(&store.Run{Declarations: decls})
	if got := len(sess.Declarations.ByProgram); got != 60 {
		t.Fatalf("premise broken: by_program has %d entries, want 60", got)
	}
	got := programs(sess.Declarations.ByProgram, 0)
	if len(got) > listWidth+len(" and 48 more of 60 (--json lists them all)") {
		t.Errorf("by program is %d characters, want it bounded near %d: %q", len(got), listWidth, got)
	}
	if !strings.Contains(got, "of 60 (--json lists them all)") {
		t.Errorf("a shortened line must say how many there were and where the rest are: %q", got)
	}
}
