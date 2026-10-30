package disclose

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/learn"
	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// TestTRU23_LearnPathsAreInTheInventory: no hook writes the lesson ledger, so
// TestTRU23_EveryWrittenPathIsInTheInventory never reaches it. Here a lesson
// is imported, approved and labelled, the temporary files a crash in each
// write leaves are put beside them, and every file under learn/ is
// described by Default().Files (card P6).
func TestTRU23_LearnPathsAreInTheInventory(t *testing.T) {
	const project = "work_proj-0123456789ab"
	root := t.TempDir()
	l, err := learn.Open(root, project, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.Import(strings.NewReader(`{"lessons":[{"id":"a","text":"b"}]}`), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := l.Approve("a", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := l.AppendLabel(learn.Label{LessonID: "a", SessionID: "s", Label: steer.LabelConfirmed}); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(learn.DirLearn, project)
	written := []string{filepath.Join(learn.DirLearn, learn.FileKey)}
	for _, name := range []string{learn.FileLessons, learn.FileLabels, learn.FileActive, learn.FileSum} {
		written = append(written, filepath.Join(dir, name))
	}
	crashed := []string{filepath.Join(learn.DirLearn, "."+learn.FileKey+".tmp-1")}
	for _, name := range []string{learn.FileLessons, learn.FileActive, learn.FileSum} {
		crashed = append(crashed, filepath.Join(dir, "."+name+".tmp-1"))
	}
	for _, rel := range crashed {
		if err := os.WriteFile(filepath.Join(root, rel), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range append(written, crashed...) {
		if _, err := os.Lstat(filepath.Join(root, rel)); err != nil {
			t.Errorf("%s was not written: %v", rel, err)
		}
	}
	n := 0
	err = filepath.WalkDir(filepath.Join(root, learn.DirLearn), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		n++
		rel, err := filepath.Rel(root, p)
		if err == nil && !matchesAFileRule(rel, inv.Files) {
			t.Errorf("learn wrote %q, which Default().Files does not cover", rel)
		}
		if info, ierr := d.Info(); ierr == nil && info.Mode().Perm() != 0o600 {
			t.Errorf("%s has mode %v, want 0600", rel, info.Mode().Perm())
		}
		return err
	})
	if err != nil || n != len(written)+len(crashed) {
		t.Errorf("walked %d files (%v), want %d", n, err, len(written)+len(crashed))
	}
}
