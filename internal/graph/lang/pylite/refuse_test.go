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

// TestBalanced_ACommentLineCountsTowardTheRefusalLine: a comment runs to
// the end of its line and stops before the newline, so that newline is
// still counted and a bracket refused after a comment, a whole-line one
// or a trailing one, is named at its own line. Break: let the comment
// loop stop on the newline instead of before it, and the scan's next step
// passes that newline uncounted: a refusal after a comment names an
// earlier line.
func TestBalanced_ACommentLineCountsTowardTheRefusalLine(t *testing.T) {
	for _, c := range []struct {
		src  string
		line int
		why  string
	}{
		{"# a comment\nx = (1\n", 2, "a bracket opened at this line is not closed"},
		{"x = 1  # a trailing comment\ny = 2)\n", 2, "a bracket closed at this line was not opened"},
		{"# one\n# two\nz = [\n", 3, "a bracket opened at this line is not closed"},
	} {
		var pe parseError
		if err := balanced(c.src); !errors.As(err, &pe) || pe.line != c.line || pe.why != c.why {
			t.Errorf("balanced(%q) = %v; want line %d: %s", c.src, err, c.line, c.why)
		}
	}
}

// TestBalanced_AnUnclosedBracketIsReportedWhereTheOutermostOpened: a
// bracket that never closes is refused at the line the outermost open
// bracket opened at, where the unfinished statement starts: not the line
// of a bracket opened inside it, closed or not, and not a bracket that
// opened and closed on an earlier line. Break: record the line at every
// opening bracket, and the refusal names the last bracket opened.
func TestBalanced_AnUnclosedBracketIsReportedWhereTheOutermostOpened(t *testing.T) {
	const why = "a bracket opened at this line is not closed"
	for _, c := range []struct {
		src  string
		line int
	}{
		{"x = (1,\n     [2, 3],\n", 1},
		{"call(a,\n  f(b,\n", 1},
		{"x = (1)\ny = [2,\n  (3),\n", 2},
	} {
		var pe parseError
		if err := balanced(c.src); !errors.As(err, &pe) || pe.line != c.line || pe.why != why {
			t.Errorf("balanced(%q) = %v; want line %d: %s", c.src, err, c.line, why)
		}
	}
}
