package disclose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestTRU23_TheCheckpointsHaveARowOfTheirOwn (INDEX-POINTER-FILE1): the
// project's last and prior checkpoints (session id, point seq, when
// recorded) are a file of their own beside the project index, so one row
// covers that file and says so in its Holds, and the index's row no longer
// claims them; the written bytes are the proof of where each lives. Break:
// drop the checkpoints from their row's Holds.
func TestTRU23_TheCheckpointsHaveARowOfTheirOwn(t *testing.T) {
	root := t.TempDir()
	const key = "/tru23/checkpoint-project"
	var c verify.Checkpoints
	c.NoteCheckpoint("s1", 4, 100)
	c.NoteCheckpoint("s2", 9, 200)
	path := verify.CheckpointsPath(root, key)
	if err := verify.WriteCheckpoints(path, &c); err != nil {
		t.Fatalf("verify.WriteCheckpoints: %v", err)
	}
	index := verify.IndexPath(root, key)
	if _, err := verify.WriteIndex(index, &verify.ProjectIndex{ProjectKey: key}); err != nil {
		t.Fatalf("verify.WriteIndex: %v", err)
	}
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	holds := func(p string) (string, string) {
		t.Helper()
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(root, p)
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
		return string(raw), rows[0].Holds
	}
	raw, h := holds(path)
	for _, want := range []string{`"last":`, `"prior":`, `"s1"`, `"s2"`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("premise: the checkpoint file lacks %s: %s", want, raw)
		}
	}
	if !strings.Contains(h, "checkpoint") || !strings.Contains(h, "session id") || !strings.Contains(h, "point seq") {
		t.Errorf("the checkpoint file's row does not name what it holds: %q", h)
	}
	raw, h = holds(index)
	if strings.Contains(raw, `"last"`) || strings.Contains(h, "checkpoint") {
		t.Errorf("the index holds or claims the checkpoints: %s / %q", raw, h)
	}
}
