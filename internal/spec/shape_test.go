package spec

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// specExample is archive 20.2.8's generic example, verbatim.
const specExample = `{
  "rashomon_spec": 1,
  "gates": [
    {"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]},
    {"id": "lint", "kind": "lint", "argv": ["golangci-lint", "run"], "success_line": ["^0 issues\\.$"]},
    {"id": "verify", "kind": "custom", "argv": ["python", "tools/verify.py"], "aliases": [["make", "verify"]],
     "applies_to": ["jobs/**/*.py"]}
  ],
  "ci_commands": [["go", "vet", "./..."]],
  "rules": [
    {"id": "no-shell-true", "kind": "forbid_code", "languages": ["python"],
     "pattern": "subprocess\\.(Popen|run|call|check_output)\\([^)]*shell\\s*=\\s*True",
     "why": "The platform SAST gate rejects shell=True.", "hard_control": "CI scanner job",
     "alternative": "pass an argument list; for AWS calls use boto3"},
    {"id": "frozen-migrations", "kind": "protect_path", "paths": ["db/migrations/**"], "modes": ["modify", "delete"],
     "why": "Applied migrations are immutable.", "hard_control": "CODEOWNERS on db/migrations",
     "alternative": "add a new migration file"},
    {"id": "scan-before-push", "kind": "require_evidence", "at": ["push"], "evidence": ["kind:scan", "unit"],
     "why": "CI rejects pushes with scanner findings.", "hard_control": "CI scanner job"},
    {"id": "export-after-verify", "kind": "order",
     "then": {"argv": ["python", "jobs/export.py"]}, "after_gate": "verify", "same_run_key": ["--mode"],
     "invalidated_by": ["jobs/**"],
     "why": "Exports must follow a verification of the same mode.", "hard_control": "none; review only"},
    {"id": "no-truncate", "kind": "forbid_command", "command": {"argv": ["cqlsh"], "sql": ["TRUNCATE"]},
     "why": "Shared keyspaces are never truncated.", "hard_control": "database role grants"}
  ]
}`

func ptr[T any](v T) *T { return &v }

// TestVER37b_DefaultsApplied: Match "prefix", Required true, and the three
// Modes of a protect_path rule are filled in when absent and kept when given.
func TestVER37b_DefaultsApplied(t *testing.T) {
	parse := func(in string) *File {
		t.Helper()
		f, _, err := Parse([]byte(in))
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	g := parse(withGate()).Gates[0]
	if g.Match != "prefix" || g.Required == nil || !*g.Required {
		t.Errorf("gate defaults: match %q, required %v", g.Match, g.Required)
	}
	if g.Dir != "" || g.AppliesTo != nil {
		t.Errorf("dir %q and applies_to %v are the evaluator's defaults, not Parse's", g.Dir, g.AppliesTo)
	}
	g = parse(withGate(kv{"match", `"exact"`}, kv{"required", `false`})).Gates[0]
	if g.Match != "exact" || g.Required == nil || *g.Required {
		t.Errorf("explicit gate values were overwritten: match %q, required %v", g.Match, g.Required)
	}
	r := parse(withRule(kv{"kind", `"protect_path"`})).Rules[0]
	if want := []string{"modify", "delete", "create"}; !reflect.DeepEqual(r.Modes, want) {
		t.Errorf("protect_path modes %v, want %v", r.Modes, want)
	}
	r = parse(withRule(kv{"kind", `"protect_path"`}, kv{"modes", `["delete"]`})).Rules[0]
	if want := []string{"delete"}; !reflect.DeepEqual(r.Modes, want) {
		t.Errorf("explicit modes %v, want %v", r.Modes, want)
	}
	if r := parse(withRule()).Rules[0]; r.Modes != nil {
		t.Errorf("a forbid_code rule has no modes, got %v", r.Modes)
	}
	// Defaults must not alias: editing one gate's Required leaves the next.
	f := parse(top(`"gates": [` + gate() + `, ` + gate(kv{"id", `"b"`}) + `]`))
	*f.Gates[0].Required = false
	if !*f.Gates[1].Required {
		t.Error("two gates share one default Required")
	}
	f = parse(top(`"rules": [` + rule(kv{"kind", `"protect_path"`}) + `, ` + rule(kv{"id", `"b"`}, kv{"kind", `"protect_path"`}) + `]`))
	f.Rules[0].Modes[0] = "x"
	if f.Rules[1].Modes[0] != "modify" {
		t.Error("two rules share one default Modes")
	}
}

// TestVER37c_SpecExampleParses: the archive's own example reads into exactly
// the values it spells, so a field mapped to the wrong key shows here.
func TestVER37c_SpecExampleParses(t *testing.T) {
	f, digest, err := Parse([]byte(specExample))
	if err != nil {
		t.Fatal(err)
	}
	want := &File{
		RashomonSpec: 1,
		Gates: []Gate{
			{ID: "unit", Kind: "test", Argv: []string{"go", "test", "./..."}, Match: "prefix", Required: ptr(true)},
			{ID: "lint", Kind: "lint", Argv: []string{"golangci-lint", "run"}, Match: "prefix", Required: ptr(true), SuccessLine: []string{`^0 issues\.$`}},
			{ID: "verify", Kind: "custom", Argv: []string{"python", "tools/verify.py"}, Match: "prefix", Required: ptr(true),
				Aliases: [][]string{{"make", "verify"}}, AppliesTo: []string{"jobs/**/*.py"}},
		},
		CICommands: [][]string{{"go", "vet", "./..."}},
		Rules: []Rule{
			{ID: "no-shell-true", Kind: "forbid_code", Languages: []string{"python"},
				Pattern: `subprocess\.(Popen|run|call|check_output)\([^)]*shell\s*=\s*True`,
				Why:     "The platform SAST gate rejects shell=True.", HardControl: "CI scanner job",
				Alternative: "pass an argument list; for AWS calls use boto3"},
			{ID: "frozen-migrations", Kind: "protect_path", Paths: []string{"db/migrations/**"}, Modes: []string{"modify", "delete"},
				Why: "Applied migrations are immutable.", HardControl: "CODEOWNERS on db/migrations", Alternative: "add a new migration file"},
			{ID: "scan-before-push", Kind: "require_evidence", At: []string{"push"}, Evidence: []string{"kind:scan", "unit"},
				Why: "CI rejects pushes with scanner findings.", HardControl: "CI scanner job"},
			{ID: "export-after-verify", Kind: "order", Then: &CommandMatch{Argv: []string{"python", "jobs/export.py"}},
				AfterGate: "verify", SameRunKey: []string{"--mode"}, InvalidatedBy: []string{"jobs/**"},
				Why: "Exports must follow a verification of the same mode.", HardControl: "none; review only"},
			{ID: "no-truncate", Kind: "forbid_command", Command: &CommandMatch{Argv: []string{"cqlsh"}, SQL: []string{"TRUNCATE"}},
				Why: "Shared keyspaces are never truncated.", HardControl: "database role grants"},
		},
	}
	if !reflect.DeepEqual(f, want) {
		t.Errorf("parsed\n%+v\nwant\n%+v", f, want)
	}
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		t.Errorf("digest %q", digest)
	}
	// A fact with every field, and a target.
	full := withFact(kv{"target", `{"kind": "aws_account", "id": "123"}`}, kv{"why", `"user only"`})
	ff, _, err := Parse([]byte(full))
	if err != nil {
		t.Fatal(err)
	}
	wantFact := Fact{ID: "f1", Kind: "aws_profile", Programs: []string{"aws"}, Target: &FactTarget{Kind: "aws_account", ID: "123"},
		Classes: []string{"cred.sso_missing"}, Value: "dev", Mechanism: "profile", Why: "user only"}
	if !reflect.DeepEqual(ff.Facts[0], wantFact) {
		t.Errorf("fact %+v, want %+v", ff.Facts[0], wantFact)
	}
}

// TestVER37e_ParseIsDeterministic: the same bytes give the same File, digest
// and error on every call, and a reordered document gives an equal File. No
// map iteration reaches the result: validation walks keys in file order.
func TestVER37e_ParseIsDeterministic(t *testing.T) {
	var firstF *File
	var firstD string
	for i := 0; i < 20; i++ {
		f, d, err := Parse([]byte(specExample))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstF, firstD = f, d
			continue
		}
		if !reflect.DeepEqual(f, firstF) || d != firstD {
			t.Fatalf("run %d differs", i)
		}
	}
	// Two faults in one object: the first in file order is the one reported,
	// whichever way round they are written.
	a := top(`"zeta": 1, "alpha": 2`)
	b := top(`"alpha": 2, "zeta": 1`)
	for want, in := range map[string]string{"/zeta": a, "/alpha": b} {
		for i := 0; i < 20; i++ {
			_, _, err := Parse([]byte(in))
			if e, ok := err.(*Error); !ok || e.Pointer != want {
				t.Fatalf("run %d of %s: %v, want pointer %s", i, in, err, want)
			}
		}
	}
	// A reordered gate is the same Gate.
	g1 := withGate()
	g2 := `{"gates": [{"argv": ["go", "test"], "kind": "test", "id": "unit"}], "rashomon_spec": 1}`
	f1, _, _ := Parse([]byte(g1))
	f2, _, _ := Parse([]byte(g2))
	if !reflect.DeepEqual(f1, f2) {
		t.Errorf("key order changed the File: %+v vs %+v", f1, f2)
	}
}

// TestVER37f_StandardLibraryOnly: package spec links nothing outside the
// standard library, so the hook path that will call it adds no dependency
// (the standard's rule on dependencies).
func TestVER37f_StandardLibraryOnly(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no source files: %v", err)
	}
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, name, src, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			first, _, _ := strings.Cut(path, "/")
			if strings.Contains(first, ".") {
				t.Errorf("%s imports %s, which is not the standard library", name, path)
			}
		}
	}
}
