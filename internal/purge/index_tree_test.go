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

// TestTRU28b_PurgeTakesTheCheckpointFiles (INDEX-POINTER-FILE1): the
// project's checkpoint file and its replacement are rashomon's, and go with
// verify/checkpoints and verify after them, beside the index; a file of the
// user's there is foreign and is left. Break: leave verify/checkpoints out of
// verify's tree.
func TestTRU28b_PurgeTakesTheCheckpointFiles(t *testing.T) {
	root := storeAt(t)
	index, file := verify.IndexPath(root, "/work/repo"), verify.CheckpointsPath(root, "/work/repo")
	checkpoints := filepath.Dir(file)
	temp := filepath.Join(checkpoints, "."+filepath.Base(file)+".tmp-123")
	writeFiles(t, index, file, temp)
	p, err := Build(root, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{file, temp, checkpoints, filepath.Dir(checkpoints)} {
		if !slices.Contains(p.Files, want) {
			t.Errorf("the plan does not list %s: %q", want, p.Files)
		}
	}
	if i, j := slices.Index(p.Files, file), slices.Index(p.Files, checkpoints); i > j {
		t.Errorf("%s is planned before %s, which holds it", checkpoints, file)
	}

	root = storeAt(t)
	file = verify.CheckpointsPath(root, "/work/repo")
	mine := filepath.Join(filepath.Dir(file), "mine.txt")
	writeFiles(t, file, mine)
	if p, err = Build(root, nil, ""); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(p.Foreign, mine) || !slices.Contains(p.Files, file) || slices.Contains(p.Files, filepath.Dir(file)) {
		t.Errorf("want %s planned, %s foreign and its directory left: files %q, foreign %q", file, mine, p.Files, p.Foreign)
	}
}
