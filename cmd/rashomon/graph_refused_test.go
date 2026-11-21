package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// TestGraphIndex_ARefusedFileIsNamedAndTheIndexWritten (note (a) e1, e2):
// `graph index` over a tree whose Python file the extractor refuses
// warns once for that file with its path, line and reason, writes the
// index with the refusal in its header, and counts it in the summary; and
// `graph brief` over that index withholds the absence the refused file
// makes false. Break: fail the index on the refusal, as before.
func TestGraphIndex_ARefusedFileIsNamedAndTheIndexWritten(t *testing.T) {
	dir := t.TempDir()
	for rel, text := range map[string]string{
		".rashomon/graph.json": `{"units":[{"path":"host","modules":["host"],"lang":"py"}]}`,
		"host/host.py":         "def canonical_name(user):\n    return user.strip()\n",
		"host/billing.py":      "def refund_ledger(entry):\n    return entry\n\n\ndef void_invoices(rows:\n    return rows\n",
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
	if want := "warning: host/billing.py:5 not indexed: a bracket opened at this line is not closed\n"; strings.Count(out.String(), want) != 1 {
		t.Errorf("graph index printed:\n%s\nwant the warning once: %s", out.String(), want)
	}
	if !strings.Contains(out.String(), "indexed 2 file(s), 1 refused: ") {
		t.Errorf("graph index printed:\n%s\nwant the summary to count the refused file", out.String())
	}
	_, h, err := graph.LoadFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h.Refused != 1 || len(h.RefusedFiles) != 1 || h.RefusedFiles[0].Path != "host/billing.py" || h.RefusedFiles[0].Line != 5 {
		t.Errorf("header refused %d %+v; want host/billing.py at line 5", h.Refused, h.RefusedFiles)
	}
	var brief strings.Builder
	if err := graphBriefCmd([]string{dir, "--prompt", "fix host/billing.py so refunds round down"}, &brief); err != nil {
		t.Fatal(err)
	}
	if b := brief.String(); strings.Contains(b, "host/billing.py is not in the index") || !strings.Contains(b, "withheld: the index refused 1 file(s)") {
		t.Errorf("graph brief over the refused file printed:\n%s\nwant the absence withheld", b)
	}
}
