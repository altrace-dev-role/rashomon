package recap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// forbiddenWords is internal/report/vocabulary_test.go's own list -- claims
// about intent, which a fact about text must never make -- WIDENED with
// verdict words, because this package's whole non-goal is one report.go's
// vocabulary test cannot check: "No verdict from rashomon. A count is a
// fact; 'on task' is not, and rashomon cannot reach it" (spec, Non-goals).
// report's renderer never had a reason to write "stayed on task" or
// "authorized" in the first place; this package, summarising a whole turn in
// one line, is exactly where the temptation to reach for one would land.
//
// H-94's break: put a forbidden word in this package's source and this test
// must go red. It cannot check semantic correctness -- only that the
// literal words never appear -- and that bound is written into the original
// test this one is copied from, not fixed here.
var forbiddenWords = []string{
	// Claims about intent, unchanged from report's own list.
	"lied", "lie", "hid", "hiding", "gaslit", "gaslight", "deceived", "concealed", "dishonest",
	// Claims about verdict -- named explicitly in the spec's H-94.
	"stayed on task", "rogue", "safe", "authorized", "unauthorized", "violation",
	"malicious", "suspicious", "compromised", "off task", "on task",
}

// TestRecapRendererStatesNoVerdict is vocabulary_test.go's own AST scan
// (internal/report), COPIED rather than imported.
//
// filepath.Glob("*.go") resolves against the package under test at `go test`
// time, so report's original only ever walked report's own directory --
// widening its list would have left this package's renderer completely
// unchecked, which is the mechanism note in H-94 itself. A copy in THIS
// directory is what makes the glob look at recap.go and state.go instead.
func TestRecapRendererStatesNoVerdict(t *testing.T) {
	sources, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	fset := token.NewFileSet()
	var scanned, literals int
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		scanned++

		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			literals++
			lower := strings.ToLower(text)
			for _, w := range forbiddenWords {
				re := regexp.MustCompile(`\b` + regexp.QuoteMeta(w) + `\b`)
				if re.MatchString(lower) {
					t.Errorf("%s prints %q: %q\nThis package states records -- counts and the "+
						"store's own fixed reason codes -- and never a verdict about whether the "+
						"agent stayed on task, was authorized, or acted safely. rashomon cannot "+
						"reach that claim (spec, Non-goals) and must not sound like it did.",
						fset.Position(lit.Pos()), w, text)
				}
			}
			return true
		})
	}
	// Guard the premise twice, at thresholds sized to this package rather
	// than copied from report's larger one: a glob matching nothing, or a
	// walk finding no literals, would both pass in silence otherwise.
	if scanned < 2 {
		t.Fatalf("scanned only %d non-test files in this package; the glob is not looking at "+
			"the renderer any more", scanned)
	}
	if literals < 8 {
		t.Fatalf("found only %d string literals across %d files; the walk is not reaching the "+
			"rendered text", literals, scanned)
	}
}
