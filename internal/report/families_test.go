package report

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// decl builds a declaration with a program and the hosts it named.
func decl(program string, hosts ...string) store.Declaration {
	p := program
	return store.Declaration{
		ToolUseID: "t-" + program,
		ToolName:  "Bash",
		SessionID: "sess-1",
		Shape:     shape.Shape{Program: &p},
		Hosts:     hosts,
	}
}

func familyByName(fc FamilyCoverage, name string) (Family, bool) {
	for _, f := range fc.Families {
		if f.Name == name {
			return f, true
		}
	}
	return Family{}, false
}

// TestFamilies_UnexercisedFamiliesAreListedNotOmitted. A reader shown only the
// families that ran cannot tell an unused family from one that was not checked.
func TestFamilies_UnexercisedFamiliesAreListedNotOmitted(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{decl("curl", "pypi.org")}}

	fc := buildFamilies(run)

	if len(fc.NotExercised) == 0 {
		t.Fatal("no families listed as unexercised, although only curl ran")
	}
	for _, name := range fc.NotExercised {
		if name == "curl" {
			t.Error("curl is listed as unexercised although it ran")
		}
	}
	var sawGo bool
	for _, name := range fc.NotExercised {
		if name == "go" {
			sawGo = true
		}
	}
	if !sawGo {
		t.Errorf("not_exercised = %v, want it to include go", fc.NotExercised)
	}
}

// TestFamilies_GroupedByTool: pip and python3 are one story, and a report that
// split them would ask the reader to reassemble it.
func TestFamilies_GroupedByTool(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		decl("pip", "pypi.org"),
		decl("python3", "files.pythonhosted.org"),
	}}

	fc := buildFamilies(run)

	f, ok := familyByName(fc, "python")
	if !ok {
		t.Fatal("no python family")
	}
	if f.Calls != 2 {
		t.Errorf("calls = %d, want 2", f.Calls)
	}
	if len(f.Programs) != 2 {
		t.Errorf("programs = %v, want both pip and python3", f.Programs)
	}
	if f.HostsDeclared != 2 {
		t.Errorf("hosts_declared = %d, want 2: pypi.org and files.pythonhosted.org", f.HostsDeclared)
	}
}

// TestFamilies_UnknownProgramIsNotAFamily. The table is a named set; an
// unrecognised program would otherwise create a family per command a session
// happened to run.
func TestFamilies_UnknownProgramIsNotAFamily(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		decl("some-internal-tool", "internal.example"),
	}}

	fc := buildFamilies(run)

	for _, f := range fc.Families {
		if f.Name == "some-internal-tool" {
			t.Error("an unrecognised program became a family")
		}
	}
}
