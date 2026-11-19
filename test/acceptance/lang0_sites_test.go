package acceptance

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The ratchet of the standing rule LANG-0: no new non-test line in cmd/ or
// internal/ may name a language, outside internal/graph/lang,
// internal/platform and the wiring functions of the index. A line names
// a language when it holds one of these shapes, printed by the test as it
// runs:
//
//	import    an import of a package under internal/graph/lang/ (gopg, pylite)
//	pkgsel    a selector on such an import (gopg.Resolver, pylite.Lang)
//	suffix    a string literal ending in a source suffix (".go", "_test.py", "Test.java")
//	manifest  a string literal equal to a row's manifest, or ending in "/" and one
//	bare      a string literal equal to a row's bare extension ("go", "py")
//	tool      a string literal equal to a row's toolchain command, or that word and a space
//
// Limit: a language with no resolver is watched by the import and suffix shapes only.
// It closes when the language registers: the wiring check then fails until the
// language has a row, and from then on its manifest, bare and tool words count.
//
// Comments are not code: the parser drops them, so a comment that names a
// language is never counted. pgraph is a module of its own and outside the
// scope; test/ and scripts/ are tooling of this repository, outside LANG-0.

// lang0Row is the pattern row of one language the index registers a
// resolver for, keyed in lang0Rows by the name of its package under
// internal/graph/lang.
type lang0Row struct {
	suffix, bare, manifest, tool []string
}

var lang0Rows = map[string]lang0Row{
	"gopg": {
		suffix:   []string{".go"},
		bare:     []string{"go"},
		manifest: []string{"go.mod", "go.sum", "go.work"},
		tool:     []string{"go", "gofmt"},
	},
	"pylite": {
		suffix:   []string{".py"},
		bare:     []string{"py"},
		manifest: []string{"pyproject.toml", "setup.cfg", "requirements.txt", "Pipfile"},
		tool:     []string{"python", "python3", "pytest", "pip", "pip3"},
	},
}

// lang0OtherSuffixes are the source suffixes of languages with no resolver:
// the suffix shape is all that watches them (the limit above).
var lang0OtherSuffixes = []string{
	".java", ".kt", ".kts", ".cs", ".swift", ".scala", ".rb", ".ex", ".exs", ".cc", ".cpp",
	".rs", ".ts", ".tsx", ".js", ".jsx", ".php",
}

const lang0Pkg = "github.com/altrace-dev-role/rashomon/internal/graph/lang/"

// lang0Wirings are the wiring functions of the index, keyed as the ledger
// keys a function: Wire, the registry a hook reads a file through, and
// resolverFor, the resolver Build wires for a unit's language. Every language
// package one of them names must have a row in lang0Rows, so a language
// cannot join either without joining this ratchet. Each must be found and
// hold its exempt row. The change that has Build read through Wire removes
// resolverFor from this list together with its exempt row.
var lang0Wirings = []string{
	"internal/graph/index/index.go:Wire",
	"internal/graph/index/index.go:resolverFor",
}

// lang0ExemptDirs are the places LANG-0 allows outright, each with its reason.
var lang0ExemptDirs = []struct{ dir, reason string }{
	{"internal/graph/lang/", "the home of each language: its resolver and its suffix, manifest and toolchain words belong here"},
	{"internal/platform/", "the reader of pgraph's fact log, where the pgraph module (outside LANG-0) crosses into this one: " +
		"the packages it reads are the Go packages pgraph records, so the language is its input format, not a branch of behaviour"},
}

// lang0Act is what a contributor does when a line that names a language is
// new or a count rises.
const lang0Act = "move the line into the package of a registered language under internal/graph/lang and ask the registry for it, " +
	"or register the language there; raising the ledger without the maintainers' decision does not fix a rise"

// lang0Exempt are the named exempt rows, each with its reason. A row that
// matches no line fails the test, so an exemption cannot outlive its site.
var lang0Exempt = map[string]string{
	"internal/graph/index/index.go:Wire":              "a wiring function: the registry a hook reads a file through",
	"internal/graph/index/index.go:resolverFor":       "a wiring function: the resolver Build wires for a unit's language, until Build reads through Wire",
	"internal/graph/index/index.go:python":            "builds the pylite resolver for resolverFor and Wire and nothing else",
	"internal/graph/index/index.go:import":            "the gopg and pylite imports of resolverFor and Wire",
	"internal/graph/graphtest/graphtest.go:import":    "test support: graph tests build their fake resolver from pylite",
	"internal/graph/graphtest/graphtest.go:Resolver":  "test support: the fake resolver graph tests register",
	"internal/graph/graphtest/graphtest.go:Specimen":  "test support: the fake resolver's fixture type",
	"internal/graph/graphtest/graphtest.go:Sym":       "test support: symbol ids in the fake resolver's language",
	"internal/graph/graphtest/graphtest.go:Const":     "test support: symbol ids in the fake resolver's language",
	"internal/graph/graphtest/synthetic.go:Synthetic": "test support: synthetic trees of the fake resolver's language",
}

// The ledger, in three classes, each key a file and the top-level declaration
// that holds the lines (a function as Name or Recv.Name, a var, const or type
// by its name, the imports as "import"), each value its count of lines. It is
// a ratchet, not a pardon: a count that rises fails, a key not listed fails,
// and a count that falls fails until the row is lowered or removed.

// lang0Finding are the sites where the hook path and the commands name Go in
// place of asking the registry. LANG-REG1 routes each through the wiring
// function and lowers this class to nothing.
var lang0Finding = map[string]int{
	"cmd/rashomon/plan.go:import":             1,
	"cmd/rashomon/plan.go:planNamesOf":        1,
	"internal/hook/blastradius.go:Post.blast": 1,
	"internal/verify/platform.go:isGoWrite":   1,
}

// lang0SingleLanguage is behaviour written for one or two languages by name,
// left for a later card to make neutral.
var lang0SingleLanguage = map[string]int{
	"internal/brief/brief.go:Brief.buildFact":      1,
	"internal/brief/brief.go:Brief.packagesFact":   2,
	"internal/brief/walk.go:readGoDir":             1,
	"internal/recap/platform.go:platformLine":      2,
	"internal/shape/verb.go:refusesRun":            1,
	"internal/shape/writetargets.go:ruleOf":        1,
	"internal/verify/pathclass.go:isTestName":      1,
	"internal/verify/platform.go:goTestKey":        1,
	"internal/verify/platform.go:writeTouches":     1,
	"internal/verify/platform_cover.go:coverLines": 1,
}

// lang0Tables are tables that list several languages as data (a table built
// on first use sits in a function, under RES-2), left for a later card to
// move to one home package.
var lang0Tables = map[string]int{
	"internal/brief/brief.go:manifests":                2,
	"internal/gateinfer/gateinfer.go:commands":         2,
	"internal/gateinfer/gateinfer.go:manifests":        7,
	"internal/gateout/tables.go:tables":                2,
	"internal/intent/scan.go:extensions":               2,
	"internal/report/families.go:families":             3,
	"internal/report/timeline.go:subcommandPrograms":   3,
	"internal/shape/label.go:labelTable":               8,
	"internal/shape/masked.go:buildCommands":           1,
	"internal/shape/masked.go:lintCommands":            1,
	"internal/shape/masked.go:typecheckCommands":       1,
	"internal/shape/masked_name.go:sameWork":           3,
	"internal/shape/runner.go:leadingFlags":            2,
	"internal/shape/runner.go:testCommands":            5,
	"internal/shape/verb.go:inPlaceEditors":            2,
	"internal/shape/verb.go:notARun":                   4,
	"internal/shape/verb.go:programVerb":               2,
	"internal/verify/pathclass.go:camelTestExtensions": 1,
	"internal/verify/pathclass.go:configBasenames":     2,
	"internal/verify/pathclass.go:generatedSuffixes":   1,
	"internal/verify/pathclass.go:testSuffixes":        2,
}

// lang0Shape names the shape a string literal's value matches, or "".
func lang0Shape(v string) string {
	suffixes := append([]string(nil), lang0OtherSuffixes...)
	for _, r := range lang0Rows {
		suffixes = append(suffixes, r.suffix...)
	}
	for _, s := range suffixes {
		if strings.HasSuffix(v, s) {
			return "suffix"
		}
	}
	for _, r := range lang0Rows {
		for _, m := range r.manifest {
			if v == m || strings.HasSuffix(v, "/"+m) {
				return "manifest"
			}
		}
		for _, b := range r.bare {
			if v == b {
				return "bare"
			}
		}
		for _, c := range r.tool {
			if v == c || strings.HasPrefix(v, c+" ") {
				return "tool"
			}
		}
	}
	return ""
}

// lang0Scan returns, for every key in scope, the set of its lines that name
// a language; every declaration key it saw, matched or not; and, for each
// wiring function, the language packages it names.
func lang0Scan(t *testing.T) (sites map[string]map[int]bool, seen map[string]bool, wired map[string]map[string]bool) {
	t.Helper()
	sites, seen, wired = map[string]map[int]bool{}, map[string]bool{}, map[string]map[string]bool{}
	fset := token.NewFileSet()
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(moduleRoot, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			rel, err := filepath.Rel(moduleRoot, p)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			for _, ex := range lang0ExemptDirs {
				if strings.HasPrefix(rel, ex.dir) {
					return nil
				}
			}
			body, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			f, err := parser.ParseFile(fset, p, body, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			lang0File(fset, rel, f, sites, seen, wired)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return sites, seen, wired
}

// lang0File adds one parsed file's matches to sites, keyed by declaration.
func lang0File(fset *token.FileSet, rel string, f *ast.File, sites map[string]map[int]bool, seen map[string]bool, wired map[string]map[string]bool) {
	add := func(key string, pos token.Pos) {
		if sites[key] == nil {
			sites[key] = map[int]bool{}
		}
		sites[key][fset.Position(pos).Line] = true
	}
	local := map[string]string{}
	for _, im := range f.Imports {
		path, err := strconv.Unquote(im.Path.Value)
		if err != nil || !strings.HasPrefix(path, lang0Pkg) {
			continue
		}
		pkg := strings.TrimPrefix(path, lang0Pkg)
		name := pkg[strings.LastIndex(pkg, "/")+1:]
		if im.Name != nil {
			name = im.Name.Name
		}
		local[name] = pkg
		seen[rel+":import"] = true
		add(rel+":import", im.Pos())
	}
	inspect := func(key string, n ast.Node) {
		seen[key] = true
		ast.Inspect(n, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.SelectorExpr:
				if id, ok := x.X.(*ast.Ident); ok {
					if pkg, ok := local[id.Name]; ok {
						add(key, id.Pos())
						if slices.Contains(lang0Wirings, key) {
							if wired[key] == nil {
								wired[key] = map[string]bool{}
							}
							wired[key][pkg] = true
						}
					}
				}
			case *ast.BasicLit:
				if x.Kind != token.STRING {
					break
				}
				if v, err := strconv.Unquote(x.Value); err == nil && lang0Shape(v) != "" {
					add(key, x.Pos())
				}
			}
			return true
		})
	}
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			inspect(rel+":"+funcName(d), d)
		case *ast.GenDecl:
			if d.Tok == token.IMPORT {
				continue
			}
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.ValueSpec:
					inspect(rel+":"+s.Names[0].Name, s)
				case *ast.TypeSpec:
					inspect(rel+":"+s.Name.Name, s)
				}
			}
		}
	}
}

// lang0LogPatterns prints the shapes and rows the test counts by.
func lang0LogPatterns(t *testing.T) {
	t.Helper()
	t.Log("LANG-0 shapes: import and pkgsel (a package under internal/graph/lang/), suffix, manifest, bare, tool")
	pkgs := make([]string, 0, len(lang0Rows))
	for p := range lang0Rows {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	for _, p := range pkgs {
		r := lang0Rows[p]
		t.Logf("row %s: suffix %q bare %q manifest %q tool %q", p, r.suffix, r.bare, r.manifest, r.tool)
	}
	t.Logf("suffix only (no resolver): %q", lang0OtherSuffixes)
	t.Log("Limit: a language with no resolver is watched by the import and suffix shapes only. " +
		"It closes when the language registers: the wiring check then fails until the language has a row, " +
		"and from then on its manifest, bare and tool words count.")
}

// lang0Classes are the ledger's three classes, each with what it is.
var lang0Classes = []struct {
	name, what string
	rows       map[string]int
}{
	{"A lang0Finding", "the hook path and the commands name Go in place of asking the registry; LANG-REG1 lowers it to 0", lang0Finding},
	{"B lang0SingleLanguage", "behaviour written for one or two languages by name", lang0SingleLanguage},
	{"C lang0Tables", "tables that already list languages with no resolver as data: a debt of placement, not of behaviour", lang0Tables},
}

// lang0LogLedger prints the exempt places and rows with their reasons, and
// each class of the ledger with the count of every key.
func lang0LogLedger(t *testing.T) {
	t.Helper()
	for _, ex := range lang0ExemptDirs {
		t.Logf("exempt directory %s: %s", ex.dir, ex.reason)
	}
	for _, key := range sortedKeys(lang0Exempt) {
		t.Logf("exempt row %s: %s", key, lang0Exempt[key])
	}
	for _, c := range lang0Classes {
		total := 0
		for _, n := range c.rows {
			total += n
		}
		t.Logf("class %s (%s): %d lines in %d keys", c.name, c.what, total, len(c.rows))
		for _, key := range sortedKeys(c.rows) {
			t.Logf("  %s %d", key, c.rows[key])
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestRubricLANG0_NoNewLanguageNamingLine holds cmd/ and internal/ to LANG-0:
// the lines that name a language may fall and never rise, and each class of
// the ledger names the card that lowers it.
func TestRubricLANG0_NoNewLanguageNamingLine(t *testing.T) {
	lang0LogPatterns(t)
	lang0LogLedger(t)
	sites, seen, wired := lang0Scan(t)

	for _, w := range lang0Wirings {
		if !seen[w] {
			t.Errorf("wiring function %s not found: if it moved, move its entry in lang0Wirings and its exempt row with it", w)
		}
		if _, ok := lang0Exempt[w]; !ok {
			t.Errorf("wiring function %s has no exempt row", w)
		}
		for _, pkg := range sortedKeys(wired[w]) {
			if _, ok := lang0Rows[pkg]; !ok {
				t.Errorf("wiring function %s names the resolver of %s, which has no row in lang0Rows: add its suffix, bare extension, manifests and toolchain commands", w, pkg)
			}
		}
	}

	var problems []string
	for key, reason := range lang0Exempt {
		if len(sites[key]) == 0 {
			problems = append(problems, fmt.Sprintf("exempt row %s (%s) matches no line: remove it", key, reason))
		}
		delete(sites, key)
	}

	ledger := map[string]int{}
	for _, c := range lang0Classes {
		for key, n := range c.rows {
			if _, dup := ledger[key]; dup {
				problems = append(problems, fmt.Sprintf("%s is in two classes of the ledger", key))
			}
			ledger[key] = n
		}
	}
	for key, want := range ledger {
		got := len(sites[key])
		switch {
		case got > want:
			problems = append(problems, fmt.Sprintf("%s: %d lines name a language, the ledger holds %d: LANG-0 bars a new one; %s", key, got, want, lang0Act))
		case got == 0:
			problems = append(problems, fmt.Sprintf("%s: no line names a language now, the ledger holds %d: remove the row", key, want))
		case got < want:
			problems = append(problems, fmt.Sprintf("%s: %d lines name a language, the ledger holds %d: lower the row", key, got, want))
		}
	}
	for key, lines := range sites {
		if _, ok := ledger[key]; !ok {
			problems = append(problems, fmt.Sprintf("%s: %d lines name a language (%v) and the ledger has no row: LANG-0 bars a new site; %s", key, len(lines), sortedLines(lines), lang0Act))
		}
	}
	sort.Strings(problems)
	for _, p := range problems {
		t.Error(p)
	}
}

func sortedLines(m map[int]bool) []int {
	out := make([]int, 0, len(m))
	for n := range m {
		out = append(out, n)
	}
	sort.Ints(out)
	return out
}
