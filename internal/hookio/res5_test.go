package hookio

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// res5Dirs are the hook-path packages, relative to the module root. A listed
// package that does not exist on this tree yet is skipped by name.
var res5Dirs = []string{
	"internal/hook", "internal/shape", "internal/verify", "internal/steer",
	"internal/harness", "internal/learn", "cmd/rashomon",
}

// res5Allowed is the one decode a hook path may still make: canonical's
// `var v any` over tool_input, kept for digest compatibility until a
// streaming canonicaliser passes RES-48. Named by package and function.
var res5Allowed = map[string]bool{"internal/shape.canonical": true}

// TestRES5_PayloadScannedOnce: each hook process reads its payload once, in
// hookio.Intake or IntakeStream, and locates fields rather than decoding
// them. No hook-path package calls
// json.Unmarshal or builds a json.Decoder, so neither the raw payload nor a
// tool_input span is decoded again into any, map[string]json.RawMessage or
// another map; the one exception is named in res5Allowed. Break: restore the
// second decode of the payload for backgroundTaskId, and this names it.
func TestRES5_PayloadScannedOnce(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	scanned := 0
	for _, dir := range res5Dirs {
		if _, err := os.Stat(filepath.Join(root, dir)); os.IsNotExist(err) {
			t.Logf("%s: not on this tree", dir)
			continue
		}
		err := filepath.WalkDir(filepath.Join(root, dir), func(f string, e os.DirEntry, err error) error {
			if err != nil || e.IsDir() || !strings.HasSuffix(f, ".go") || strings.HasSuffix(f, "_test.go") {
				return err
			}
			rel, err := filepath.Rel(root, f)
			if err != nil {
				return err
			}
			scanned++
			for _, d := range decodesIn(t, f) {
				if !res5Allowed[filepath.ToSlash(filepath.Dir(rel))+"."+d.fn] {
					t.Errorf("%s:%d: %s in %s decodes JSON on a hook path; read spans from hookio instead", rel, d.line, d.call, d.fn)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if scanned < 20 {
		t.Fatalf("scanned %d files: the package list no longer matches the tree", scanned)
	}
}

type decode struct {
	fn, call string
	line     int
}

// decodesIn lists the calls to encoding/json's Unmarshal and NewDecoder in
// one file, under whatever name the file imports the package as.
func decodesIn(t *testing.T, path string) []decode {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	name := ""
	for _, imp := range f.Imports {
		if p, err := strconv.Unquote(imp.Path.Value); err == nil && p == "encoding/json" {
			name = "json"
			if imp.Name != nil {
				name = imp.Name.Name
			}
		}
	}
	if name == "" {
		return nil
	}
	var out []decode
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == name && (sel.Sel.Name == "Unmarshal" || sel.Sel.Name == "NewDecoder") {
				out = append(out, decode{fn: fn.Name.Name, call: name + "." + sel.Sel.Name, line: fset.Position(call.Pos()).Line})
			}
			return true
		})
	}
	return out
}
