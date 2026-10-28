package purge

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestTRU28_PurgeTakesTheProjectIndex: the project index and its replacement
// are rashomon's, and go with verify/projects and verify after them; a file of
// the user's in either directory is foreign and is left (card 2.4 R17).
func TestTRU28_PurgeTakesTheProjectIndex(t *testing.T) {
	root := storeAt(t)
	index := verify.IndexPath(root, "/work/repo")
	projects := filepath.Dir(index)
	temp := filepath.Join(projects, "."+filepath.Base(index)+".tmp-123")
	writeFiles(t, index, temp)

	p, err := Build(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{index, temp, projects, filepath.Dir(projects)} {
		if !slices.Contains(p.Files, want) {
			t.Errorf("the plan does not list %s: %q", want, p.Files)
		}
	}
	if i, j := slices.Index(p.Files, index), slices.Index(p.Files, projects); i > j {
		t.Errorf("%s is planned before %s, which holds it", projects, index)
	}

	root = storeAt(t)
	index = verify.IndexPath(root, "/work/repo")
	projects = filepath.Dir(index)
	mine := []string{filepath.Join(projects, "mine.txt"), filepath.Join(filepath.Dir(projects), "notes.txt")}
	writeFiles(t, append([]string{index}, mine...)...)
	p, err = Build(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range mine {
		if !slices.Contains(p.Foreign, f) || slices.Contains(p.Files, f) {
			t.Errorf("%s is not foreign: files %q, foreign %q", f, p.Files, p.Foreign)
		}
	}
	if !slices.Contains(p.Files, index) || slices.Contains(p.Files, projects) {
		t.Errorf("want %s planned and %s, which holds a user file, left: %q", index, projects, p.Files)
	}
}
