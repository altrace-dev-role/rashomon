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
// TrustedRoot, or the type gains an exported field.
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
