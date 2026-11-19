package index

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// TestBuild_GoFilesDeclareNoModuleID (control for the Python module id):
// in the two-language build the Go files declare their symbols and no
// module id, and a Go import ends at a unit node, as before Python files
// declared their modules.
func TestBuild_GoFilesDeclareNoModuleID(t *testing.T) {
	r, err := Build(shapes(t), "c1", twoLangs)
	if err != nil {
		t.Fatal(err)
	}
	declares, imports := 0, 0
	for _, e := range r.Store.Edges(graph.RevCommit("c1")) {
		if !strings.HasPrefix(string(e.Src), "file:") || !strings.HasSuffix(string(e.Src), ".go") {
			continue
		}
		switch e.Pred {
		case graph.Declares:
			declares++
			if strings.Contains(string(e.Dst), ":mod:") {
				t.Errorf("Go file %s declares %s", e.Src, e.Dst)
			}
		case graph.Imports:
			imports++
			if !strings.HasPrefix(string(e.Dst), "unit:go:") {
				t.Errorf("Go import %s -> %s ends at no unit", e.Src, e.Dst)
			}
		}
	}
	if declares == 0 || imports == 0 {
		t.Errorf("Go files declare %d and import %d; want some of each", declares, imports)
	}
}
