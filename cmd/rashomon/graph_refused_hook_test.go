package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestGraphBrief_TheHookPathWithholdsOverARefusedFile (note (a) e2, the
// hook caller): the hook path hands the engine the refused count from the
// index's header, as graph brief does, so the evidence file it leaves
// under a v5 store prints the withheld mark in place of an absence over a
// refused file. The tree and the prompt are the claim-reason case of
// TestBrief_AnAbsenceOverARefusedFileIsWithheld, whose precondition shows
// that the reason reaches the evidence file when no count is given. A
// prompt that names the refused file would not do: that reason reaches
// Explain's names line alone, which the evidence file does not print. The
// scorer is pinned to the hook's default, the one that precondition read.
// Break: drop the count at the hook caller.
func TestGraphBrief_TheHookPathWithholdsOverARefusedFile(t *testing.T) {
	t.Setenv("RASHOMON_GRAPH_SCORER", "")
	dir := t.TempDir()
	for rel, text := range map[string]string{
		".rashomon/graph.json": `{"units":[{"path":"host","modules":["host"],"lang":"py"},{"path":"spend","modules":["spend"],"lang":"py"}]}`,
		"host/host.py":         "def canonical_name(user):\n    return user.strip()\n\n\ndef load_rows(db):\n    return db.execute(\"SELECT id, total FROM invoices WHERE paid = 0\")\n",
		"host/billing.py":      "def refund_ledger(entry):\n    return entry\n\n\ndef void_invoices(rows:\n    return rows\n\n\ndef faster_checkout(cart):\n    return cart\n",
		"spend/mode.py":        "def fast_mode(x):\n    return x\n\n\ndef fast_mode_pricing(x):\n    return fast_mode(x)\n",
	} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out strings.Builder
	if err := graphIndex([]string{dir}, &out); err != nil {
		t.Fatalf("graph index = %v; want the index written without host/billing.py\n%s", err, out.String())
	}
	storeRoot := filepath.Join(t.TempDir(), "store")
	if err := os.MkdirAll(storeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteRecordScopeMarker(storeRoot, store.RecordScopeV5); err != nil {
		t.Fatal(err)
	}
	ib, _ := graphBrief(storeRoot, dir, "s1", "p1", []byte("speed up the refund ledger"))
	if ib.Refused != 1 {
		t.Errorf("the hook path's brief holds refused %d; want the header's 1", ib.Refused)
	}
	gdir := filepath.Join(storeRoot, graph.StoreDir)
	entries, err := os.ReadDir(gdir)
	if err != nil {
		t.Fatal(err)
	}
	var evidence []string
	for _, e := range entries {
		if !e.IsDir() && graph.IsEvidenceFileName(e.Name()) {
			evidence = append(evidence, e.Name())
		}
	}
	if len(evidence) != 1 {
		t.Fatalf("evidence files under %s: %v; want one", gdir, evidence)
	}
	b, err := os.ReadFile(filepath.Join(gdir, evidence[0]))
	if err != nil {
		t.Fatal(err)
	}
	const mark = "withheld: the index refused 1 file(s)"
	if ev := string(b); strings.Contains(ev, "no unit names the target") || !strings.Contains(ev, mark) {
		t.Errorf("the hook path's evidence over a refused file carries the absence or lacks %q:\n%s", mark, ev)
	}
}
