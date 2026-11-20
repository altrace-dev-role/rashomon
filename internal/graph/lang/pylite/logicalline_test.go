package pylite_test

import (
	"slices"
	"sort"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// TestLogicalLine_BracketsJoinLinesAndACommentEndsItsLine: Python reads a
// statement as one logical line, its physical lines joined by a backslash
// outside a comment or by a bracket still open, and a comment ends where its
// physical line ends. So a from-import whose names sit in brackets over
// several lines (the form black writes) is one from-import, a comment on an
// import line is no part of the import, and a backslash inside a comment
// joins nothing. Each form is checked beside its one-line twin, and both
// must give the edges written out here. The last row is a guard: a bracket
// opened on an import-like line inside a string joins no line past the
// string's closing line, so the import after the string is still read.
//
// What the scan still cannot read: an import inside a def's body (only
// calls are read there); a plain import of several modules on one line
// (import a, b); two statements on one line joined by a semicolon; and an
// import-like line inside a string, which is read as an import, as before.
func TestLogicalLine_BracketsJoinLinesAndACommentEndsItsLine(t *testing.T) {
	res := pylite.Resolver{
		Units:   []pylite.Unit{{Path: "src/pkg", ModRoot: "src"}, {Path: "top", ModRoot: "top"}},
		Modules: map[string]string{"pkg": "src/pkg", "xx": "top", "yy": "top"},
	}
	const rel = "src/pkg/a.py"
	const (
		modMM = "sym:py:src/pkg:mod:pkg.mm"
		modNN = "sym:py:src/pkg:mod:pkg.nn"
		modXX = "sym:py:top:mod:xx"
		modYY = "sym:py:top:mod:yy"
		mmXX  = "sym:py:src/pkg:func:pkg.mm.xx"
		mmYY  = "sym:py:src/pkg:func:pkg.mm.yy"
		nnXX  = "sym:py:src/pkg:func:pkg.nn.xx"
	)
	for _, c := range []struct {
		name, form, twin string
		imports, refs    []string // destinations from file:src/pkg/a.py
	}{
		{"a bracket with one name per line", "from .mm import (\n    xx,\n    yy\n)\n", "from .mm import (xx, yy)\n",
			[]string{modMM}, []string{mmXX, mmYY}},
		{"a bracket with a trailing comma", "from .mm import (\n    xx,\n    yy,\n)\n", "from .mm import (xx, yy,)\n",
			[]string{modMM}, []string{mmXX, mmYY}},
		{"a bracket with a comment on a name line", "from .mm import (\n    xx,  # the first\n    yy,\n)\n", "from .mm import (xx, yy)  # the first\n",
			[]string{modMM}, []string{mmXX, mmYY}},
		{"a bracket with an alias", "from .mm import (\n    xx as aa,\n    yy,\n)\n", "from .mm import (xx as aa, yy)\n",
			[]string{modMM}, []string{mmXX, mmYY}},
		{"a bracket with a name on its opening line", "from .mm import (xx,\n    yy)\n", "from .mm import (xx, yy)\n",
			[]string{modMM}, []string{mmXX, mmYY}},
		{"a comment on a from line", "from .mm import xx  # noqa\n", "from .mm import xx\n",
			[]string{modMM}, []string{mmXX}},
		{"a comment on a plain import line", "import xx  # noqa\n", "import xx\n",
			[]string{modXX}, nil},
		{"a comment that ends in a backslash joins nothing", "from .mm import xx  # see \\\nimport yy\n", "from .mm import xx\nimport yy\n",
			[]string{modMM, modYY}, []string{mmXX}},
		{"pinned: a bracket inside a string joins nothing past the string", "\"\"\"Usage:\nfrom .mm import (\n\"\"\"\nfrom .nn import xx\nYY = \")\"\n", "from .nn import xx\nYY = \")\"\n",
			[]string{modNN}, []string{nnXX}},
	} {
		t.Run(c.name, func(t *testing.T) {
			want := func(pred graph.Pred, dsts []string) []string {
				var out []string
				for _, d := range dsts {
					out = append(out, "file:src/pkg/a.py -"+string(pred)+"-> "+d)
				}
				sort.Strings(out)
				return out
			}
			for _, v := range []struct{ label, src string }{{"form", c.form}, {"twin", c.twin}} {
				recs, _ := extractFiles(t, res, map[string]string{rel: v.src})
				for _, chk := range []struct {
					pred graph.Pred
					want []string
				}{{graph.Imports, want(graph.Imports, c.imports)}, {graph.References, want(graph.References, c.refs)}} {
					if got := edgesOf(recs, chk.pred); !slices.Equal(got, chk.want) {
						t.Errorf("%s: %s edges %q; want %q", v.label, chk.pred, got, chk.want)
					}
				}
				ov, err := res.Overlay(rel, []byte(v.src), relRev, 1)
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range []graph.Pred{graph.Imports, graph.References} {
					if got, want := edgesOf(ov, p), edgesOf(recs, p); !slices.Equal(got, want) {
						t.Errorf("%s: overlay %s edges %q; extract %q", v.label, p, got, want)
					}
				}
			}
		})
	}
}
