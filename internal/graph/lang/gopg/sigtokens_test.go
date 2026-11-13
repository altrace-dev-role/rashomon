package gopg

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// sigDecl parses one function declaration the way Overlay does and returns
// its spelling and go/printer's print of its type, the sig before
// sigtokens.go (go/printer is imported by this test only).
func sigDecl(t *testing.T, decl string) (spelled, printed string) {
	t.Helper()
	content := []byte("package p\n\n" + decl + " {}\n")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", content, parser.SkipObjectResolution|parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %q: %v", decl, err)
	}
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			var b bytes.Buffer
			if err := printer.Fprint(&b, fset, fd.Type); err != nil {
				t.Fatalf("print %q: %v", decl, err)
			}
			return sigSpelling(fset, content, fd.Type), b.String()
		}
	}
	t.Fatalf("no function in %q", decl)
	return "", ""
}

// TestGopgSig_PrinterEqualIsSpellingEqual: over pairs of one declaration
// before and after a change, the spelling and go/printer agree on equal or
// unequal, except the layout rows: a list reflowed across lines, which
// go/printer printed with its line breaks (its parameters reads each field's
// line from the FileSet) and the spelling does not see. No pair go/printer
// printed equal spells unequal, so no change gofmt alone makes reads as a
// signature change.
func TestGopgSig_PrinterEqualIsSpellingEqual(t *testing.T) {
	for _, c := range []struct {
		name, a, b string
		equal      bool // the spelling's relation
		layout     bool // go/printer's differs: unequal, by line breaks only
	}{
		{"spacing in a list", "func F(a,b int)", "func F(a, b int)", true, false},
		{"trailing comma on one line", "func F(a int,)", "func F(a int)", true, false},
		{"multi-line list", "func F(\n\ta int,\n\tb string,\n)", "func F(a int, b string)", true, true},
		{"multi-line results", "func F() (\n\tn int,\n\terr error,\n)", "func F() (n int, err error)", true, true},
		{"block comment in a list", "func F(a int /* n */, b string)", "func F(a int, b string)", true, false},
		{"line comment in a multi-line list", "func F(\n\ta int, // n\n\tb string,\n)", "func F(\n\ta int,\n\tb string,\n)", true, false},
		{"single result in parens", "func F() (int)", "func F() int", true, false},
		{"single result in parens with a comma", "func F() (int,)", "func F() int", true, false},
		{"nested function result in parens", "func F(f func() (int))", "func F(f func() int)", true, false},
		{"doubled parens", "func F(a ((int)))", "func F(a (int))", true, false},
		{"line-end semicolons gofmt removes", "func F(x interface {\n\tM();\n\tN()\n})", "func F(x interface {\n\tM()\n\tN()\n})", true, false},
		{"embedded fields against a field", "func F(x struct{ T; U })", "func F(x struct{ T U })", false, false},
		{"receiver and name are not the type", "func (s *S) M(a int)", "func (s S) N(a int)", true, false},
		{"generic, spacing", "func F[T any, U comparable](x T) U", "func F[T any,U comparable] (x T) U", true, false},
		{"generic constraint changed", "func F[T any](x T)", "func F[T comparable](x T)", false, false},
		{"parameter type changed", "func F(a int)", "func F(a int64)", false, false},
		{"result added", "func F(a int) int", "func F(a int) (int, error)", false, false},
		{"variadic against a slice", "func F(a ...int)", "func F(a []int)", false, false},
		{"named result renamed", "func F() (n int)", "func F() (m int)", false, false},
		{"grouping kept", "func F(a, b int)", "func F(a int, b int)", false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			sa, pa := sigDecl(t, c.a)
			sb, pb := sigDecl(t, c.b)
			if sa == "" || sb == "" {
				t.Fatalf("empty spelling: %q, %q", sa, sb)
			}
			if got := sa == sb; got != c.equal {
				t.Errorf("spellings equal = %v, want %v: %q and %q", got, c.equal, sa, sb)
			}
			if got, want := pa == pb, c.equal && !c.layout; got != want {
				t.Errorf("go/printer equal = %v, want %v: %q and %q", got, want, pa, pb)
			}
			if pa == pb && sa != sb {
				t.Errorf("go/printer printed %q for both and the spellings differ: %q and %q", pa, sa, sb)
			}
		})
	}
}

// TestGopgSig_Spellings pins the spelling itself: the type only, tokens
// joined by one space, gofmt's parentheses and trailing commas gone.
func TestGopgSig_Spellings(t *testing.T) {
	for decl, want := range map[string]string{
		"func F[T any, U comparable](x T) U":             "func [ T any , U comparable ] ( x T ) U",
		"func (s *S) M() (int)":                          "func ( ) int",
		"func F(f func() (int))":                         "func ( f func ( ) int )",
		"func F() (\n\tn int, // first\n\terr error,\n)": "func ( ) ( n int , err error )",
		"func F(x struct {\n\ta int\n\tb string\n})":     "func ( x struct { a int ; b string } )",
		"func F(x interface{ M() (int) }) ([]byte)":      "func ( x interface { M ( ) int } ) [ ] byte",
	} {
		if got, _ := sigDecl(t, decl); got != want {
			t.Errorf("%q spelled %q, want %q", decl, got, want)
		}
	}
}

// TestGopgOverlay_LinksNoPrinter: the binary's import graph holds no
// go/printer. go/build/constraint stays in it: go/parser imports it to read a
// file's //go:build line into its Go version, so only the half go/printer
// reached (fixGoBuildLines and the +build conversion) leaves the binary, a
// claim about linked symbols that an import list cannot make. The control:
// go/parser is listed, so an empty list does not pass.
func TestGopgOverlay_LinksNoPrinter(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/altrace-dev-role/rashomon/cmd/rashomon").Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	pkgs := strings.Fields(string(out))
	if !slices.Contains(pkgs, "go/parser") {
		t.Fatalf("go/parser not in the binary's %d packages: the list proves nothing", len(pkgs))
	}
	if slices.Contains(pkgs, "go/printer") {
		t.Errorf("the binary links go/printer")
	}
}
