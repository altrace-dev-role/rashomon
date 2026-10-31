package disclose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestTRU23_IndexRowNamesTheCheckpoints: the project index the Stop writes
// holds the last and prior checkpoints (session id, point seq, when recorded)
// beside its command families, so the one row that covers the file says so in
// its Holds; the written bytes are the proof it holds them.
func TestTRU23_IndexRowNamesTheCheckpoints(t *testing.T) {
	root := t.TempDir()
	const key = "/tru23/checkpoint-project"
	idx := &verify.ProjectIndex{ProjectKey: key}
	idx.NoteCheckpoint("s1", 4, 100)
	idx.NoteCheckpoint("s2", 9, 200)
	path := verify.IndexPath(root, key)
	if _, err := verify.WriteIndex(path, idx); err != nil {
		t.Fatalf("verify.WriteIndex: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"last":`, `"prior":`, `"s1"`, `"s2"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("premise: the index file lacks %s: %s", want, raw)
		}
	}
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	var rows []FileRule
	for _, r := range inv.Files {
		if matchesAFileRule(rel, []FileRule{r}) {
			rows = append(rows, r)
		}
	}
	if len(rows) != 1 {
		t.Fatalf("%d rows cover %s, want one", len(rows), rel)
	}
	if h := rows[0].Holds; !strings.Contains(h, "checkpoint") || !strings.Contains(h, "session id") || !strings.Contains(h, "point seq") {
		t.Errorf("the index row's Holds does not name the checkpoints it holds: %q", h)
	}
}
