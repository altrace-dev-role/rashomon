package perf

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// parsePkgs parses fixture packages: import path -> file name -> source.
func parsePkgs(t *testing.T, fset *token.FileSet, srcs map[string]map[string]string) []SrcPackage {
	t.Helper()
	var pkgs []SrcPackage
	for path, files := range srcs {
		p := SrcPackage{Path: path}
		for name, src := range files {
			f, err := parser.ParseFile(fset, path+"/"+name, src, 0)
			if err != nil {
				t.Fatalf("%s/%s: %v", path, name, err)
			}
			p.Files = append(p.Files, f)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

// TestRES2c_InitScanFollowsEveryPathToACompile: RES-2's static check finds
// a regexp compile however package initialisation reaches it: under an
// aliased or dot import, through a package-local helper a var initialiser
// or init calls (in another file, transitively, as a method), and through a
// function of another scanned package; and it does not count a compile
// inside a function sync.OnceFunc, OnceValue or OnceValues defers to first
// use, nor one in a function initialisation never calls.
func TestRES2c_InitScanFollowsEveryPathToACompile(t *testing.T) {
	one := func(src string) map[string]map[string]string {
		return map[string]map[string]string{"m/a": {"a.go": "package a\n" + src}}
	}
	for _, c := range []struct {
		name string
		pkgs map[string]map[string]string
		want int
	}{
		{"plain", one(`import "regexp"; var re = regexp.MustCompile("a")`), 1},
		{"posix", one(`import "regexp"; var re, _ = regexp.CompilePOSIX("a")`), 1},
		{"MatchString compiles too", one(`import "regexp"; var ok, _ = regexp.MatchString("a", "a")`), 1},
		{"aliased import", one(`import re "regexp"; var x = re.MustCompile("a")`), 1},
		{"dot import", one(`import . "regexp"; var x = MustCompile("a")`), 1},
		{"var calls a helper (the spec's mutant)", one(`import "regexp"; var rules = compileRules()
func compileRules() *regexp.Regexp { return regexp.MustCompile("a") }`), 1},
		{"init calls a helper", one(`import "regexp"; var r *regexp.Regexp
func init() { r = build() }
func build() *regexp.Regexp { return regexp.MustCompile("a") }`), 1},
		{"helper calls a helper", one(`import "regexp"; var r = outer()
func outer() *regexp.Regexp { return inner() }
func inner() *regexp.Regexp { return regexp.MustCompile("a") }`), 1},
		{"generic helper", one(`import "regexp"; var r = build[int]()
func build[T any]() *regexp.Regexp { return regexp.MustCompile("a") }`), 1},
		{"method", one(`import "regexp"; type T struct{}; var r = T{}.compile()
func (T) compile() *regexp.Regexp { return regexp.MustCompile("a") }`), 1},
		{"helper in another file", map[string]map[string]string{"m/a": {
			"a.go": "package a\nvar rules = compileRules()\n",
			"b.go": "package a\nimport \"regexp\"\nfunc compileRules() *regexp.Regexp { return regexp.MustCompile(\"a\") }\n",
		}}, 1},
		{"another scanned package", map[string]map[string]string{
			"m/a": {"a.go": "package a\nimport mb \"m/b\"\nvar rules = mb.Rules()\n"},
			"m/b": {"b.go": "package b\nimport \"regexp\"\nfunc Rules() *regexp.Regexp { return regexp.MustCompile(\"a\") }\n"},
		}, 1},
		{"immediately called literal", one(`import "regexp"; var r = func() *regexp.Regexp { return regexp.MustCompile("a") }()`), 1},
		{"sync.OnceValue defers it", one(`import ("regexp"; "sync"); var r = sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile("a") })`), 0},
		{"aliased sync.OnceFunc defers it", one(`import ("regexp"; s "sync"); var f = s.OnceFunc(func() { regexp.MustCompile("a") })`), 0},
		{"sync.OnceValues defers a helper", one(`import ("regexp"; "sync"); var r = sync.OnceValues(build)
func build() (*regexp.Regexp, error) { return regexp.Compile("a") }`), 0},
		{"compiled on use only", one(`import "regexp"; func match(s string) bool { return regexp.MustCompile("a").MatchString(s) }`), 0},
		{"recursion terminates", one(`var n = f(3)
func f(i int) int { if i == 0 { return 0 }; return f(i - 1) }`), 0},
		{"a local MustCompile is not regexp's", one(`func MustCompile(string) int { return 0 }; var n = MustCompile("a")`), 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			fset := token.NewFileSet()
			got := RegexpCompilesAtInit(parsePkgs(t, fset, c.pkgs))
			if len(got) != c.want {
				var where []string
				for _, g := range got {
					where = append(where, fset.Position(g.Pos).String()+" via "+g.Via)
				}
				t.Errorf("%d compiles at init, want %d: %s", len(got), c.want, strings.Join(where, "; "))
			}
		})
	}
}
