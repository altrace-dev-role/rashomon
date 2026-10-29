package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The stored duration is store.Execution.DurationMS (duration_ms in the
// record). A replay omits it by ruling (docs/autosteer-prescan.md, revision 7:
// stored null, "never estimated from transcript timestamps"), because nothing
// reads it back. This test is what keeps that true: it fails if any non-test
// code outside the recorder's own write path reads the field, by its Go name
// or by its record key, so the day duration feeds a verdict, a steer note, a
// report line or a digest, the omission is reopened on purpose and not found
// out from a replayed verdict that differs from a live one.

// durationWritePath is the code allowed to name the field: where the payload
// key is declared and read (hookio, hook.PostPayload), where it is stored
// (hook.Post building the Execution, store.Execution declaring it), and the
// replay's own field table, which says why it is omitted.
var durationWritePath = map[string]string{
	"internal/hookio/payload.go": "declares the payload key",
	"internal/hook/post.go":      "reads the payload key and stores it on the execution",
	"internal/store/record.go":   "declares the stored field",
	"internal/replay/fields.go":  "the replay's field table, which records its omission",
}

// durationRequired are the directories the guard must have scanned: the
// consumers a verdict, a note, a report or a digest is built in. A guard that
// found no files in one (a moved directory) would pass on nothing.
var durationRequired = []string{
	"internal/verify", "internal/steer", "internal/recap", "internal/report", "internal/digest", "internal/share", "cmd/rashomon",
}

// durationUses lists the places a Go source reads or names the duration: a
// selector or a keyed field called DurationMS, and a string literal that is
// the record key.
func durationUses(fset *token.FileSet, f *ast.File) []string {
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if n.Sel.Name == "DurationMS" {
				out = append(out, fmt.Sprintf("%s: .DurationMS", fset.Position(n.Sel.Pos())))
			}
		case *ast.KeyValueExpr:
			if id, ok := n.Key.(*ast.Ident); ok && id.Name == "DurationMS" {
				out = append(out, fmt.Sprintf("%s: DurationMS:", fset.Position(id.Pos())))
			}
		case *ast.BasicLit:
			if n.Kind == token.STRING {
				if s, err := strconv.Unquote(n.Value); err == nil && s == "duration_ms" {
					out = append(out, fmt.Sprintf("%s: %q", fset.Position(n.Pos()), s))
				}
			}
		}
		return true
	})
	return out
}

// TestREPLAY11_NothingReadsTheStoredDuration is the guard of the ruling that
// a replay stores duration_ms null.
func TestREPLAY11_NothingReadsTheStoredDuration(t *testing.T) {
	// The scanner finds what it is for.
	for name, src := range map[string]string{
		"a selector":     "package x\nfunc f(e struct{ DurationMS *int64 }) int64 { return *e.DurationMS }\n",
		"a keyed field":  "package x\ntype E struct{ DurationMS *int64 }\nvar _ = E{DurationMS: nil}\n",
		"the record key": "package x\nvar k = \"duration_ms\"\n",
	} {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "x.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(durationUses(fset, f)) == 0 {
			t.Fatalf("the scanner finds nothing in %s", name)
		}
	}

	root := "../.."
	scanned := map[string]int{}
	var found []string
	fset := token.NewFileSet()
	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			scanned[filepath.ToSlash(filepath.Dir(rel))]++
			f, err := parser.ParseFile(fset, p, nil, 0)
			if err != nil {
				return err
			}
			if _, ok := durationWritePath[rel]; ok {
				return nil
			}
			found = append(found, durationUses(fset, f)...)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, dir := range durationRequired {
		if scanned[dir] == 0 {
			t.Errorf("the guard scanned no non-test Go file in %s", dir)
		}
	}
	for p := range durationWritePath {
		if _, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, p), nil, parser.PackageClauseOnly); err != nil {
			t.Errorf("the write-path file %s is gone: %v", p, err)
		}
	}
	if len(found) > 0 {
		sort.Strings(found)
		t.Fatalf("non-test code outside the recorder's write path reads the stored duration (duration_ms):\n  %s\n"+
			"A replay stores duration_ms null, by ruling, because nothing reads it back (docs/autosteer-prescan.md, revision 6, ruled in revision 7). "+
			"The duration_ms omission in replay must be revisited before duration can feed a verdict: "+
			"either a replayed record carries a duration the verdict can use, or the verdict must not use it.",
			strings.Join(found, "\n  "))
	}
}
