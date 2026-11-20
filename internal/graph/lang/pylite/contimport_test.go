package pylite_test

import (
	"slices"
	"sort"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// TestContImport_ABackslashContinuedImportReadsAsOneStatement: a backslash
// that ends a line joins the next line to it, and the joined lines are one
// statement (Python's explicit line joining). A from-import split by a
// backslash before its import keyword is one from-import: it imports the
// module its from part names, and the names after the keyword are names of
// that module. Read line by line, the indented second line alone reads as
// "import xx": an imports edge to the module xx where xx is a known
// module, nothing where it is not, and no edge to the module the from part
// names in either case. The split after the keyword, a split plain import,
// two backslashes in one statement and a CRLF line end are rows of the same
// rule. Pinned as they read without any join: the three plain forms, an
// indented import inside a try block (its own statement, so a rule that
// dropped indented imports fails it) and a backslash on the last line of a
// file, which has no line to join. Overlay of the same content writes the
// same two predicates.
func TestContImport_ABackslashContinuedImportReadsAsOneStatement(t *testing.T) {
	units := []pylite.Unit{{Path: "src/pkg", ModRoot: "src"}, {Path: "top", ModRoot: "top"}}
	known := pylite.Resolver{Units: units, Modules: map[string]string{"pkg": "src/pkg", "xx": "top"}}
	unknown := pylite.Resolver{Units: units, Modules: map[string]string{"pkg": "src/pkg"}}
	const rel = "src/pkg/a.py"
	file := string(graph.FileID(rel))
	modPkgMM := string(graph.SymID(pylite.Lang, "src/pkg", "mod", "pkg.mm"))
	refXX := string(graph.SymID(pylite.Lang, "src/pkg", "func", "pkg.mm.xx"))
	modXX := string(graph.SymID(pylite.Lang, "top", "mod", "xx"))
	for _, c := range []struct {
		name          string
		res           pylite.Resolver
		src           string
		imports, refs []string // destinations from the file
	}{
		{"from-import split before the keyword, xx known", known, "from .mm \\\n    import xx\n", []string{modPkgMM}, []string{refXX}},
		{"from-import split before the keyword, xx not known", unknown, "from .mm \\\n    import xx\n", []string{modPkgMM}, []string{refXX}},
		{"from-import split after the keyword", known, "from .mm import \\\n    xx\n", []string{modPkgMM}, []string{refXX}},
		{"plain import split after the keyword", known, "import \\\n    xx\n", []string{modXX}, nil},
		{"two backslashes in one statement", known, "from .mm \\\n    import \\\n        xx\n", []string{modPkgMM}, []string{refXX}},
		{"a CRLF line end", known, "from .mm \\\r\n    import xx\r\n", []string{modPkgMM}, []string{refXX}},
		{"pinned: plain import", known, "import xx\n", []string{modXX}, nil},
		{"pinned: plain relative from-import", known, "from .mm import xx\n", []string{modPkgMM}, []string{refXX}},
		{"pinned: plain absolute from-import", known, "from pkg.mm import xx\n", []string{modPkgMM}, []string{refXX}},
		{"pinned: an indented import in a try block", known, "try:\n    import xx\nexcept ImportError:\n    pass\n", []string{modXX}, nil},
		{"pinned: a backslash on the last line", known, "from .mm \\", nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			recs, _ := extractFiles(t, c.res, map[string]string{rel: c.src})
			want := func(pred graph.Pred, dsts []string) []string {
				var out []string
				for _, d := range dsts {
					out = append(out, file+" -"+string(pred)+"-> "+d)
				}
				sort.Strings(out)
				return out
			}
			for _, chk := range []struct {
				pred graph.Pred
				want []string
			}{{graph.Imports, want(graph.Imports, c.imports)}, {graph.References, want(graph.References, c.refs)}} {
				if got := edgesOf(recs, chk.pred); !slices.Equal(got, chk.want) {
					t.Errorf("%s edges %q; want %q", chk.pred, got, chk.want)
				}
			}
			ov, err := c.res.Overlay(rel, []byte(c.src), relRev, 1)
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range []graph.Pred{graph.Imports, graph.References} {
				if got, want := edgesOf(ov, p), edgesOf(recs, p); !slices.Equal(got, want) {
					t.Errorf("overlay %s edges %q; extract %q", p, got, want)
				}
			}
		})
	}
}
