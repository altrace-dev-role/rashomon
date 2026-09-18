package report

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/wire"
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

// TestFamilies_TransitIsMeasuredNotAssumed is the positive case, and the
// wording matters: the claim is about THIS session's calls, not about whether
// the program honours proxy variables in general.
func TestFamilies_TransitIsMeasuredNotAssumed(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		decl("pip", "pypi.org"),
	}}
	observed := map[string]bool{"pypi.org": true}

	fc := buildFamilies(run, observed, true, "")

	f, ok := familyByName(fc, "python")
	if !ok {
		t.Fatalf("no python family: %+v", fc.Families)
	}
	if f.Status != FamilyTransit {
		t.Errorf("status = %q, want %q", f.Status, FamilyTransit)
	}
	if f.HostsObserved != 1 || f.HostsDeclared != 1 {
		t.Errorf("observed/declared = %d/%d, want 1/1", f.HostsObserved, f.HostsDeclared)
	}
}

// TestFamilies_DeclaredButUnobservedIsItsOwnStatus is the negative case. It is
// deliberately not called "does not transit": the calls may not have run, or a
// response may have been cached, and the line does not pick between them.
func TestFamilies_DeclaredButUnobservedIsItsOwnStatus(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		decl("curl", "never-reached.example"),
	}}

	fc := buildFamilies(run, map[string]bool{}, true, "")

	f, _ := familyByName(fc, "curl")
	if f.Status != FamilyNotObserved {
		t.Errorf("status = %q, want %q", f.Status, FamilyNotObserved)
	}
}

// TestFamilies_ExercisedWithNoHostsIsNeither covers `go build` and `git
// status`: the family ran, named nothing, and there is nothing to join on.
// Claiming transit or its absence would be inventing a measurement.
func TestFamilies_ExercisedWithNoHostsIsNeither(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{decl("go")}}

	fc := buildFamilies(run, map[string]bool{}, true, "")

	f, _ := familyByName(fc, "go")
	if f.Status != FamilyNoHosts {
		t.Errorf("status = %q, want %q", f.Status, FamilyNoHosts)
	}
	if f.Calls != 1 {
		t.Errorf("calls = %d, want 1", f.Calls)
	}
}

// TestFamilies_UnexercisedFamiliesAreListedNotOmitted. A reader shown only the
// families that ran cannot tell an unused family from one that was not checked.
func TestFamilies_UnexercisedFamiliesAreListedNotOmitted(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{decl("curl", "pypi.org")}}

	fc := buildFamilies(run, map[string]bool{"pypi.org": true}, true, "")

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

	fc := buildFamilies(run, map[string]bool{"pypi.org": true}, true, "")

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
	if f.HostsDeclared != 2 || f.HostsObserved != 1 {
		t.Errorf("observed/declared = %d/%d, want 1/2", f.HostsObserved, f.HostsDeclared)
	}
	if f.Status != FamilyTransit {
		t.Errorf("status = %q, want transit: one observed host is proof for the family",
			f.Status)
	}
}

// TestFamilies_UnknownProgramIsNotAFamily. The table is a named set; an
// unrecognised program would otherwise create a family per command a session
// happened to run.
func TestFamilies_UnknownProgramIsNotAFamily(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{
		decl("some-internal-tool", "internal.example"),
	}}

	fc := buildFamilies(run, map[string]bool{}, true, "")

	for _, f := range fc.Families {
		if f.Name == "some-internal-tool" {
			t.Error("an unrecognised program became a family")
		}
	}
}

// TestFamilies_UnknownWhenTheStoreCouldNotBeRead is the degradation, and the
// most important row here. Without it every family would read "declared hosts
// not observed" -- blaming the families for the reader's own blindness.
func TestFamilies_UnknownWhenTheStoreCouldNotBeRead(t *testing.T) {
	run := &store.Run{Declarations: []store.Declaration{decl("curl", "pypi.org")}}

	fc := buildFamilies(run, nil, false, wire.NotObservedNoStore)

	if fc.Available {
		t.Error("available = true with no store")
	}
	if fc.Reason != wire.NotObservedNoStore {
		t.Errorf("reason = %q, want %q", fc.Reason, wire.NotObservedNoStore)
	}
	f, _ := familyByName(fc, "curl")
	if f.Status == FamilyNotObserved {
		t.Error("a family reads as \"declared hosts not observed\" when the store could " +
			"not be read, which blames the family for the reader's blindness")
	}
	if f.Status != unknown {
		t.Errorf("status = %q, want %q", f.Status, unknown)
	}
}

// TestFamilies_InheritedRowsDoNotSatisfyTheJoin: another session's traffic must
// not be able to report transit this session never had.
func TestFamilies_InheritedRowsDoNotSatisfyTheJoin(t *testing.T) {
	d := Destinations{Hosts: []wire.Destination{
		{Host: "pypi.org", InheritedAttempts: 3, Inherited: true},
		{Host: "mine.example", Attempts: 1},
	}}

	got := observedHostSet(d)

	if got["pypi.org"] {
		t.Error("an inherited host satisfied the join; it is another session's traffic")
	}
	if !got["mine.example"] {
		t.Error("this session's own host is missing from the join set")
	}
}

// TestFamilies_NotObservableIsAlwaysPresent. These are properties of the
// instrument, not of the session, so they print on a completely healthy run
// too: a reader told only what WAS observed reads the rest as absence of
// traffic rather than absence of observation.
func TestFamilies_NotObservableIsAlwaysPresent(t *testing.T) {
	healthy := buildFamilies(
		&store.Run{Declarations: []store.Declaration{decl("curl", "pypi.org")}},
		map[string]bool{"pypi.org": true}, true, "")

	if len(healthy.NotObservable) == 0 {
		t.Fatal("the not-observable list is empty on a healthy run")
	}
	var sawFetch, sawSSH bool
	for _, n := range healthy.NotObservable {
		if contains(n, "fetch") {
			sawFetch = true
		}
		if contains(n, "ssh") {
			sawSSH = true
		}
	}
	if !sawFetch || !sawSSH {
		t.Errorf("not_observable = %v, want it to name Node's built-in fetch and ssh",
			healthy.NotObservable)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
