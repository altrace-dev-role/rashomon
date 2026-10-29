package purge

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/learn"
)

// TestP6_PurgeLeavesNoLearnerFile: the learner's key, each project's state
// files and manifest, and the temporary files their writes leave are
// rashomon's and gone after purge; a user's file beside them is foreign and
// stays (card P6: the consent screen keeps lessons under <root>/learn, and
// purge removes the store).
func TestP6_PurgeLeavesNoLearnerFile(t *testing.T) {
	root := storeAt(t)
	proj := "learn/work_a-0123456789ab/"
	ours := []string{"learn/" + learn.FileKey, "learn/." + learn.FileKey + ".tmp-1",
		proj + learn.FileLessons, proj + learn.FileLabels, proj + learn.FileActive, proj + learn.FileSum,
		proj + "." + learn.FileActive + ".tmp-2"}
	mine := []string{"learn/notes.txt", proj + "notes.txt"}
	for _, f := range append(slices.Clone(ours), mine...) {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := Build(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ours {
		if !slices.Contains(p.Files, filepath.Join(root, filepath.FromSlash(f))) {
			t.Errorf("the plan does not list %s: %q", f, p.Files)
		}
	}
	for _, f := range mine {
		if abs := filepath.Join(root, filepath.FromSlash(f)); !slices.Contains(p.Foreign, abs) || slices.Contains(p.Files, abs) {
			t.Errorf("the plan does not name %s as foreign, or lists it as rashomon's: foreign %q", f, p.Foreign)
		}
	}
	p.Execute()
	for _, f := range ours {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(f))); err == nil {
			t.Errorf("purge left %s", f)
		}
	}
	for _, f := range mine {
		if b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))); err != nil || string(b) != "x" {
			t.Errorf("purge removed %s (%v)", f, err)
		}
	}
}
