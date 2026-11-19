package index

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// TestBuild_ARefusedFileIsAGapNotAFailure (note (a) e1, e2): a Python
// file the extractor refuses no longer fails the index. Build writes the
// store without it, with every other file of both languages; the result
// lists the refused file with its line and reason; and the header counts
// and names it, so a reader of the index knows an absence in it is not
// proven. Break: Build returns the extractor's error, as before.
func TestBuild_ARefusedFileIsAGapNotAFailure(t *testing.T) {
	root := shapes(t)
	put(t, filepath.Join(root, "py", "bad.py"), "def f(a:\n    return a\n")
	r, err := Build(root, "c1", twoLangs)
	if err != nil {
		t.Fatalf("Build = %v; want the index written without py/bad.py", err)
	}
	want := []graph.Refusal{{Path: "py/bad.py", Line: 1, Reason: "a bracket opened at this line is not closed"}}
	if !reflect.DeepEqual(r.Refused, want) {
		t.Errorf("Refused = %+v; want %+v", r.Refused, want)
	}
	if r.Header.Refused != 1 || !reflect.DeepEqual(r.Header.RefusedFiles, want) {
		t.Errorf("header refused %d %+v; want 1 %+v", r.Header.Refused, r.Header.RefusedFiles, want)
	}
	rev := graph.RevCommit("c1")
	for id, in := range map[graph.ID]bool{"file:py/app.py": true, "sym:go:shape:func:Sum": true, "file:py/bad.py": false} {
		if _, ok := r.Store.Node(id, rev); ok != in {
			t.Errorf("node %s in the store = %v; want %v", id, ok, in)
		}
	}
	if r.Files != 4 {
		t.Errorf("files = %d; want the four handed to the resolvers, the refused one with them", r.Files)
	}
}

// TestBuild_AnUnreadableFileIsAGap (e1): a claimed file that cannot be
// read (here a link to nothing) is a gap of the same kind, its reason the
// read error's words. Break: a read error fails the index.
func TestBuild_AnUnreadableFileIsAGap(t *testing.T) {
	root := shapes(t)
	if err := os.Symlink(filepath.Join(root, "py", "nowhere.py"), filepath.Join(root, "py", "gone.py")); err != nil {
		t.Fatal(err)
	}
	r, err := Build(root, "c1", twoLangs)
	if err != nil {
		t.Fatalf("Build = %v; want the index written without py/gone.py", err)
	}
	want := []graph.Refusal{{Path: "py/gone.py", Reason: "no such file or directory"}}
	if !reflect.DeepEqual(r.Refused, want) || r.Header.Refused != 1 {
		t.Errorf("Refused = %+v, header %d; want %+v, 1", r.Refused, r.Header.Refused, want)
	}
}

// TestBuild_TheHeaderNamesTheFirstRefusedFiles (e2): the header counts
// every refused file and names the first graph.MaxRefusedNamed by path,
// so its line stays bounded; the result keeps them all. Break: name every
// refused file in the header.
func TestBuild_TheHeaderNamesTheFirstRefusedFiles(t *testing.T) {
	root := shapes(t)
	n := graph.MaxRefusedNamed + 1
	for i := range n {
		put(t, filepath.Join(root, "py", fmt.Sprintf("bad%02d.py", i)), "def f(a:\n")
	}
	r, err := Build(root, "c1", twoLangs)
	if err != nil {
		t.Fatal(err)
	}
	h := r.Header
	if h.Refused != n || len(h.RefusedFiles) != graph.MaxRefusedNamed || len(r.Refused) != n {
		t.Fatalf("header refused %d naming %d, result %d; want %d naming %d, result %d", h.Refused, len(h.RefusedFiles), len(r.Refused), n, graph.MaxRefusedNamed, n)
	}
	if last := h.RefusedFiles[graph.MaxRefusedNamed-1].Path; last != fmt.Sprintf("py/bad%02d.py", graph.MaxRefusedNamed-1) {
		t.Errorf("the header's last named file is %s; want the first %d by path", last, graph.MaxRefusedNamed)
	}
}
