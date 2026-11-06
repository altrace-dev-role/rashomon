package knowledge

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

const truthTableText = `{"prd": "autosteer-prd-2026-10-21", "tree": "acb1c0d5", "rows": [
 {"id": "PF-PL11", "title": "Unknown, never zero", "status": "none",
  "implementation": "No reflection or cgo fan-in exists. The ledger verb reads a written Go package unknown when there is no pgraph log.",
  "missing": ["fan-in itself (PF-PL7)", "a reflection and cgo call-boundary detector"],
  "evidence": [{"file": "internal/platform/read.go", "symbol": "Read"}, {"file": "test/acceptance/h156_platform_wire_test.go", "symbol": "TestH156c"}],
  "integration": [{"event": "verb", "handler": "cmd/rashomon/report_ledger.go"}], "e2e_verified": false, "planned_pr": "", "session_proof": ""},
 {"id": "PF-PL7", "title": "Blast radius", "status": "none",
  "implementation": "pgraph's package doc names blast radius as a later slice.",
  "missing": [], "evidence": [{"file": "pgraph/doc.go"}]}
]}
`

// platformIndexed is a store with the platform read's file and symbol,
// the symbol declared by the file, and the pgraph doc file.
func platformIndexed() *graph.Store {
	s := graph.NewStore()
	for _, id := range []graph.ID{"file:internal/platform/read.go", "file:pgraph/doc.go"} {
		s.AssertNode(graph.Node{ID: id, Kind: "file", Tier: graph.Proven, Conf: 1, Rev: rev})
	}
	s.AssertNode(graph.Node{ID: "sym:go:internal/platform:func:Read", Kind: "sym", Tier: graph.Proven, Conf: 1, Rev: rev})
	s.AssertEdge(graph.Edge{Src: "file:internal/platform/read.go", Pred: graph.Declares, Dst: "sym:go:internal/platform:func:Read", Tier: graph.Proven, Conf: 1, Rev: rev})
	return s
}

// TestImport_TruthTableRow (W-M1.12): a truth table's row imports as a
// declared, approved requirement keyed by the row's id, with the title,
// status and first missing item as its clause, the implementation as its
// text, and its evidence files and symbols the index holds as applies_to
// (a symbol by its declaring file and name); an evidence file the index
// lacks is counted dropped; a JSON file that is not a truth table yields
// no rows and no error.
func TestImport_TruthTableRow(t *testing.T) {
	root := t.TempDir()
	src := put(t, filepath.Join(root, "test", "prd", "truth_table.json"), truthTableText)
	rep, err := Import(root, []string{src}, platformIndexed(), rid, rev)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(rep)
	rows, err := ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rep.Sources[0].Rows != 2 || rep.Sources[0].Clauses != 2 || rep.Sources[0].Dropped != 1 {
		t.Fatalf("rows %d, report %+v", len(rows), rep.Sources[0])
	}
	r := rows[0]
	wantClause := "Unknown, never zero; status none; missing: fan-in itself (PF-PL7)"
	if r.ID != "PF-PL11" || r.Kind != KindRequirement || r.Status != StatusApproved || r.Tier != TierDeclared || r.Clause != wantClause ||
		!strings.HasPrefix(r.Text, "No reflection") || r.Source.File != "truth_table.json" || r.Source.Row != "PF-PL11" || r.Source.Digest == "" {
		t.Errorf("row = %+v", r)
	}
	want := []string{string(graph.GlobalID(rid, "file:internal/platform/read.go")), string(graph.GlobalID(rid, "sym:go:internal/platform:func:Read"))}
	if strings.Join(r.AppliesTo, " ") != strings.Join(want, " ") {
		t.Errorf("applies_to %v, want %v", r.AppliesTo, want)
	}
	if got := rows[1].AppliesTo; len(got) != 1 || got[0] != string(graph.GlobalID(rid, "file:pgraph/doc.go")) {
		t.Errorf("PF-PL7 applies_to %v", got)
	}
	recs := Records(rows, rid, rev)
	var node, edges int
	for _, rec := range recs {
		if rec.Node != nil && rec.Node.ID == "req:PF-PL11" && rec.Node.Kind == KindRequirement && rec.Node.Tier == graph.Declared {
			node++
		}
		if rec.Edge != nil && rec.Edge.Src == "req:PF-PL11" && rec.Edge.Pred == graph.AppliesTo {
			edges++
		}
	}
	if node != 1 || edges != 2 {
		t.Errorf("records: %d node, %d applies_to edges, want 1 and 2", node, edges)
	}
	other := put(t, filepath.Join(root, "notes", "other.json"), `{"settings": {"rows": 3}}`+"\n")
	rep, err = Import(root, []string{other}, platformIndexed(), rid, rev)
	if err != nil || rep.Sources[0].Rows != 0 {
		t.Errorf("a JSON file that is no truth table: %v %+v", err, rep.Sources[0])
	}
}
