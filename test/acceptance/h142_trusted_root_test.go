package acceptance

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// H-142 (SPEC-TRUST2): DiscoverSpec's trust root is a type, hook.TrustedRoot,
// whose one field is unexported, so a caller cannot hand it a raw store path.
// The compiler holds that only while nothing else makes one. This test fails
// the day a function or a method outside the two named makers returns a
// TrustedRoot, or the type gains an exported field. Who may call NoRun, the
// one maker that takes a raw root, is TestH142_NoRunHasOneProductionCaller.
func TestH142_TrustedRootHasTwoMakers(t *testing.T) {
	dir := filepath.Join(moduleRoot, "internal", "hook")
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var makers []string
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, e.Name(), body, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range f.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Name.IsExported() && d.Type.Results != nil && returnsTrustedRoot(d.Type.Results) {
					makers = append(makers, d.Name.Name)
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					ts, ok := s.(*ast.TypeSpec)
					if !ok || ts.Name.Name != "TrustedRoot" {
						continue
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						t.Fatalf("TrustedRoot is %T, want a struct", ts.Type)
					}
					for _, fl := range st.Fields.List {
						if len(fl.Names) == 0 {
							t.Errorf("TrustedRoot embeds a type; its field must be unexported")
						}
						for _, n := range fl.Names {
							if n.IsExported() {
								t.Errorf("TrustedRoot has the exported field %s: a caller could set it", n.Name)
							}
						}
					}
				}
			}
		}
	}
	slices.Sort(makers)
	if want := []string{"NoRun", "TrustRoot"}; !slices.Equal(makers, want) {
		t.Errorf("exported makers of hook.TrustedRoot %v, want %v", makers, want)
	}
}

func returnsTrustedRoot(r *ast.FieldList) bool {
	for _, f := range r.List {
		if id, ok := f.Type.(*ast.Ident); ok && id.Name == "TrustedRoot" {
			return true
		}
	}
	return false
}

// noRunCallers is where a production call to hook.NoRun is sound: the brief
// is user-only and prints at a run's first record, so no write by the session
// it describes can have happened (printBrief and cmdBrief). Add a file here
// only with that argument written down.
var noRunCallers = map[string]bool{
	"cmd/rashomon/brief.go": true,
}

// H-142 (SPEC-TRUST2): NoRun is itself a raw-root maker, and the maker test
// above cannot see its callers. This one parses every non-test .go file in
// the module and fails on a call to NoRun, qualified or not, outside
// noRunCallers.
func TestH142_NoRunHasOneProductionCaller(t *testing.T) {
	fset := token.NewFileSet()
	var calls int
	err := filepath.WalkDir(moduleRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(moduleRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, rel, body, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			c, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			switch fn := c.Fun.(type) {
			case *ast.Ident:
				name = fn.Name
			case *ast.SelectorExpr:
				name = fn.Sel.Name
			}
			if name != "NoRun" {
				return true
			}
			calls++
			if !noRunCallers[rel] {
				t.Errorf("%s: production call to NoRun; NoRun hands DiscoverSpec the store root "+
					"unchecked, which is sound only where no session write can have happened "+
					"(the brief, at a run's first record); a caller that reads a session's own "+
					"record uses hook.TrustRoot", fset.Position(c.Pos()))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls == 0 {
		t.Errorf("no production call to NoRun found: the allowlist %v is stale, or the walk missed cmd/rashomon/brief.go", noRunCallers)
	}
}
