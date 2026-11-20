package pylite_test

import (
	"slices"
	"sort"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// TestStarImport_ImportsItsModuleAndBindsNoName: a star import (from m
// import *) imports its module and binds the public names of it, names the
// scan cannot list from the importing file alone. So it writes the imports
// edge to the module and no references edge, where a from-import of one
// name writes both. It binds no name called *, so two star imports in one
// file import both modules. Nor is it kept under its module's name, so a
// star import of the top-level module mm in a file that binds mm (from x
// import mm), before it or after it, keeps both imports edges and the
// references edge to x.mm. Each form runs beside a twin that imports one
// name from the same module, and both must give the edges written out here.
//
// What the scan still cannot read: a name a star import brings in, so a
// call through it resolves to nothing.
func TestStarImport_ImportsItsModuleAndBindsNoName(t *testing.T) {
	res := pylite.Resolver{
		Units:   []pylite.Unit{{Path: "src/pkg", ModRoot: "src"}, {Path: "top", ModRoot: "top"}},
		Modules: map[string]string{"pkg": "src/pkg", "x": "top", "mm": "top"},
	}
	const rel = "src/pkg/a.py"
	const (
		modMM    = "sym:py:src/pkg:mod:pkg.mm"
		modNN    = "sym:py:src/pkg:mod:pkg.nn"
		mmXX     = "sym:py:src/pkg:func:pkg.mm.xx"
		mmYY     = "sym:py:src/pkg:func:pkg.mm.yy"
		nnXX     = "sym:py:src/pkg:func:pkg.nn.xx"
		modX     = "sym:py:top:mod:x"
		modTopMM = "sym:py:top:mod:mm"
		xMM      = "sym:py:top:func:x.mm"
		topMMYY  = "sym:py:top:func:mm.yy"
	)
	type edges struct{ imports, refs []string } // destinations from file:src/pkg/a.py
	for _, c := range []struct {
		name, form, twin   string
		formWant, twinWant edges
	}{
		{"a relative star import", "from .mm import *\n", "from .mm import xx\n",
			edges{[]string{modMM}, nil}, edges{[]string{modMM}, []string{mmXX}}},
		{"an absolute star import", "from pkg.mm import *\n", "from pkg.mm import xx\n",
			edges{[]string{modMM}, nil}, edges{[]string{modMM}, []string{mmXX}}},
		{"a star import with a comment", "from .mm import *  # noqa: F403\n", "from .mm import xx  # noqa\n",
			edges{[]string{modMM}, nil}, edges{[]string{modMM}, []string{mmXX}}},
		{"two star imports in one file", "from .mm import *\nfrom .nn import *\n", "from .mm import yy\nfrom .nn import xx\n",
			edges{[]string{modMM, modNN}, nil}, edges{[]string{modMM, modNN}, []string{mmYY, nnXX}}},
		{"a star import of a module named like a name bound before it", "from x import mm\nfrom mm import *\n", "from x import mm\nfrom mm import yy\n",
			edges{[]string{modTopMM, modX}, []string{xMM}}, edges{[]string{modTopMM, modX}, []string{topMMYY, xMM}}},
		{"a star import of a module named like a name bound after it", "from mm import *\nfrom x import mm\n", "from mm import yy\nfrom x import mm\n",
			edges{[]string{modTopMM, modX}, []string{xMM}}, edges{[]string{modTopMM, modX}, []string{topMMYY, xMM}}},
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
			for _, v := range []struct {
				label, src string
				w          edges
			}{{"form", c.form, c.formWant}, {"twin", c.twin, c.twinWant}} {
				recs, _ := extractFiles(t, res, map[string]string{rel: v.src})
				for _, chk := range []struct {
					pred graph.Pred
					want []string
				}{{graph.Imports, want(graph.Imports, v.w.imports)}, {graph.References, want(graph.References, v.w.refs)}} {
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
