package pylite

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// TestExtract_GoesOnPastARefusedFile (note (a) e1): a file the reduced
// extractor cannot read is a gap, not the end of the index. Extract
// returns the records of every file it read and, as its error, the
// refused files by path, each with the line it stopped at and why: an
// opened bracket that never closes (the line it opened at), a closed one
// that never opened, a def with no colon, and a file that cannot be read
// (no line; the read error's words, not its path). Break: return on the
// first refusal as before, and a.py's records are lost with the error.
func TestExtract_GoesOnPastARefusedFile(t *testing.T) {
	root := t.TempDir()
	for rel, text := range map[string]string{
		"u/a.py": "def ok(x):\n    return x\n",
		"u/b.py": "import os\n\ndef broken(a, b:\n    return a\n",
		"u/c.py": "def nocolon(a)\n    return a\n",
		"u/e.py": "x = 1\ny = (2))\n",
	} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := Resolver{Units: []Unit{{Path: "u"}}, Modules: map[string]string{}}
	rev := graph.RevCommit("c1")
	recs, err := r.Extract(root, []string{"u/a.py", "u/b.py", "u/c.py", "u/d.py", "u/e.py"}, rev)
	var refused graph.Refusals
	if !errors.As(err, &refused) {
		t.Fatalf("Extract error = %v; want graph.Refusals naming the four refused files", err)
	}
	want := graph.Refusals{
		{Path: "u/b.py", Line: 3, Reason: "a bracket opened at this line is not closed"},
		{Path: "u/c.py", Line: 1, Reason: "a def with no colon after its parameters"},
		{Path: "u/d.py", Reason: "no such file or directory"},
		{Path: "u/e.py", Line: 2, Reason: "a bracket closed at this line was not opened"},
	}
	if !reflect.DeepEqual(refused, want) {
		t.Errorf("refused =\n%+v\nwant\n%+v", refused, want)
	}
	files := map[graph.ID]bool{}
	for _, rec := range recs {
		if rec.Node != nil && rec.Node.Kind == "file" {
			files[rec.Node.ID] = true
		}
	}
	if !reflect.DeepEqual(files, map[graph.ID]bool{"file:u/a.py": true}) {
		t.Errorf("file nodes = %v; want a.py's alone", files)
	}
}

// TestOverlay_ARefusalIsStillErrParse: the line a parse refusal carries
// does not change what it is; the hook's overlay writer and its tests
// match it with errors.Is(err, ErrParse), for an unclosed bracket and a
// def with no colon alike. Break: a refusal error that does not match
// ErrParse.
func TestOverlay_ARefusalIsStillErrParse(t *testing.T) {
	r := Resolver{Units: []Unit{{Path: "u"}}, Modules: map[string]string{}}
	for _, src := range []string{"def broken(a, b:\n    return a\n", "def nocolon(a)\n    return a\n"} {
		if _, err := r.Overlay("u/x.py", []byte(src), graph.RevCommit("c1"), 1); !errors.Is(err, ErrParse) {
			t.Errorf("Overlay(%q) = %v; want ErrParse", src, err)
		}
	}
}
