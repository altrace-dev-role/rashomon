package episode

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/trace"
)

func TestCommandSig(t *testing.T) {
	for cmd, want := range map[string][2]string{
		"pytest tests/test_api.py":           {"pytest", ""},
		"python -m pytest -q tests/":         {"python", "pytest"},
		"cd web && npx jest src/a.test.ts":   {"npx", "jest"},
		"npm run build":                      {"npm", "run build"},
		"FOO=1 CI=true go test ./...":        {"go", "test"},
		"timeout 30 cargo test --release":    {"cargo", "test"},
		"terraform plan -out=plan.bin":       {"terraform", "plan"},
		"/usr/local/bin/bazel build //app:x": {"bazel", "build"},
		"export X=1; make check | tail -20":  {"make", "check"},
		"sudo -E env PATH=/x mvn -q verify":  {"mvn", "verify"},
		`bash -c "echo hi"`:                  {"bash", ""},
		"":                                   {"", ""},
	} {
		p, v := commandSig(cmd)
		if p != want[0] || v != want[1] {
			t.Errorf("commandSig(%q) = %q, %q; want %q, %q", cmd, p, v, want[0], want[1])
		}
	}
}

func TestScopeOf(t *testing.T) {
	for _, c := range []struct {
		paths []string
		want  Scope
	}{
		{nil, Scope{}},
		{[]string{"a.go"}, Scope{Ext: ".go"}},
		{[]string{"src/api/client.ts", "src/api/client.test.ts"}, Scope{Dir: "src/api", Ext: ".ts"}},
		{[]string{"a/b/c/d/e/f.py"}, Scope{Dir: "a/b/c", Ext: ".py"}},
		{[]string{"a/b/c/d/f.py"}, Scope{Dir: "a/b/c", Ext: ".py"}},
		{[]string{"src/x.ts", "lib/y.ts"}, Scope{Ext: ".ts"}},
		{[]string{"tests"}, Scope{Dir: "tests"}},
	} {
		if got := scopeOf(c.paths); got != c.want {
			t.Errorf("scopeOf(%q) = %+v, want %+v", c.paths, got, c.want)
		}
	}
}

// TestFingerprint_StableAcrossWhatVaries: the same failure with other
// numbers, paths, quoted names and ids keys alike; another failure does not;
// the digest is pinned, so a change to the rule shows here and needs a new
// FingerprintVersion.
func TestFingerprint_StableAcrossWhatVaries(t *testing.T) {
	sig := ActSig{Class: trace.ActExec, Program: "tool"}
	fp := func(out string) string {
		f, _ := Fingerprint(sig, &trace.ResultStep{Failed: true, Output: out}, nil)
		return f
	}
	a := fp("ERROR at /src/a/b.py:12: cannot open 'config.yaml' (id deadbeefcafe)")
	b := fp("ERROR at /lib/zz.py:340: cannot open 'other.yml' (id facefeedbead)")
	c := fp("ERROR at /src/a/b.py:12: permission denied for 'config.yaml'")
	if a != b || a == c || a == "" {
		t.Errorf("fingerprints %q %q %q", a, b, c)
	}
	if a != "54449a23fac74de5" {
		t.Errorf("pinned digest moved: %q; bump FingerprintVersion if the rule changed", a)
	}
}

// TestFingerprint_BaselineAndTies: a line a passing run also prints is never
// picked; among the rest the most fixed words wins, ties to the later line;
// a named tool error is keyed by its name; an output with no candidate line
// has no fingerprint.
func TestFingerprint_BaselineAndTies(t *testing.T) {
	sig := ActSig{Class: trace.ActExec, Program: "tool"}
	b := Baseline{}
	b.AddPassing(sig, "running all the checks now\nall checks passed")
	_, tmpl := Fingerprint(sig, &trace.ResultStep{Failed: true, Output: "running all the checks now\nfirst bad thing\nsecond bad thing"}, b)
	if tmpl != "second bad thing" {
		t.Errorf("template %q", tmpl)
	}
	_, tmpl = Fingerprint(sig, &trace.ResultStep{Failed: true, Output: "running all the checks now\nfirst bad thing"}, nil)
	if tmpl != "running all the checks now" {
		t.Errorf("first-run template %q", tmpl)
	}
	if fp, tmpl := Fingerprint(sig, &trace.ResultStep{Failed: true, Error: trace.ErrorStaleWrite, Output: "anything"}, b); fp == "" || tmpl != "tool error stale-write" {
		t.Errorf("tool error %q %q", fp, tmpl)
	}
	if fp, _ := Fingerprint(sig, &trace.ResultStep{Failed: true, Output: "FAIL\n42\n"}, nil); fp != "" {
		t.Errorf("fingerprint of no candidate %q", fp)
	}
}

// ecosystemWords are tools and languages this package must never name
// outside its tests: what it matches is read from the session.
var ecosystemWords = []string{"pytest", "jest", "terraform", "npm", "cargo", "go test", "python", "node", "mvn",
	"gradle", "bazel", "make", "rspec", "phpunit", "dotnet", ".py", ".go", ".ts", ".tf", "Claude", "Bash"}

// TestNoEcosystemWords: no string in the package's non-test source holds a
// tool's, a language's or an agent's name.
func TestNoEcosystemWords(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			v, _ := strconv.Unquote(lit.Value)
			for _, w := range ecosystemWords {
				if strings.Contains(strings.ToLower(v), strings.ToLower(w)) {
					t.Errorf("%s:%d names %q in %s", name, fset.Position(lit.Pos()).Line, w, lit.Value)
				}
			}
			return true
		})
	}
	if checked < 4 {
		t.Fatalf("checked %d files", checked)
	}
}

// TestNothingImportsEpisode: no package's non-test code imports this one
// yet, so it reaches no hook (learning PR 2's promise; the miner is the first
// importer, in its own pull request).
func TestNothingImportsEpisode(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == ".git" || n == "testdata" || n == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}
		for _, im := range f.Imports {
			if path, _ := strconv.Unquote(im.Path.Value); strings.HasSuffix(path, "/internal/episode") {
				rel, _ := filepath.Rel(root, p)
				t.Errorf("%s imports %s", rel, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
