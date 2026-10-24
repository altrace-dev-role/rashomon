package corpus

import (
	"bytes"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/secretshape"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/test/tools/hygiene"
)

// shellClasses are the verb classes a shell line can take; agent, mcp and
// unknown name tools, so a shell row carrying one is a labelling error.
var shellClasses = []string{shape.VerbRead, shape.VerbWrite, shape.VerbNetwork, shape.VerbExecute, shape.VerbVCS, shape.VerbPackage, shape.VerbTest}

func TestLoadedRowsUseTheClassifiersVocabulary(t *testing.T) {
	repos, err := LoadRepos()
	if err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, r := range repos {
		known[r.ID] = true
	}
	paths, err := LoadPaths()
	if err != nil {
		t.Fatal(err)
	}
	pathLabels := slices.DeleteFunc(shape.Labels(), func(l string) bool { return l == shape.LabelUnknown })
	seen := map[string]bool{}
	for _, p := range paths {
		if !slices.Contains(pathLabels, p.Label) {
			t.Errorf("%s: label %q is not in shape.Labels()", p.ID, p.Label)
		}
		if !known[p.Repo] || seen[p.ID] || p.Path == "" || p.Rule == "" {
			t.Errorf("%s: bad repo, duplicate id, empty path or empty rule", p.ID)
		}
		seen[p.ID] = true
	}
	lines, err := LoadShell()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		if !slices.Contains(shellClasses, l.Label) {
			t.Errorf("%s: label %q is not a shell verb class", l.ID, l.Label)
		}
		if !known[l.Repo] || seen[l.ID] || l.Line == "" || l.Count < 1 {
			t.Errorf("%s: bad repo, duplicate id, empty line or count < 1", l.ID)
		}
		seen[l.ID] = true
	}
	if len(paths) < 300 || len(lines) < 300 {
		t.Errorf("corpora too small: %d paths, %d lines", len(paths), len(lines))
	}
}

// TestShellRowsAreScrubbed is the standing guard on the shell corpus: no row
// may hold a secret shape, and none may be a finding of the repository's
// fixture scanner, whatever tool produced the file.
func TestShellRowsAreScrubbed(t *testing.T) {
	lines, err := LoadShell()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lines {
		if secretshape.Secretish(l.Line) {
			t.Errorf("%s: secretshape flags the line", l.ID)
		}
		if f := hygiene.Scan("shell.jsonl", []byte(l.Line)); len(f) != 0 {
			t.Errorf("%s: hygiene.Scan flags the line", l.ID)
		}
	}
}

// TestReportIsDeterministic runs the whole report twice and compares the bytes;
// the report is a function of the data files and the classifier alone.
func TestReportIsDeterministic(t *testing.T) {
	a, err := Build("test-commit")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Build("test-commit")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"paths.txt", "paths.json", "shell.txt", "shell.json"} {
		if len(a[name]) == 0 || !bytes.Equal(a[name], b[name]) {
			t.Errorf("%s: empty or differs between two runs", name)
		}
	}
}

// TestDisagreementIsCountedBothWays pins the arithmetic on a hand-made table:
// two rows agree and one disagrees, so the rate is 2/3 and the one row is listed.
func TestDisagreementIsCountedBothWays(t *testing.T) {
	rows := []Judged{
		{ID: "a", Want: "none", Got: "none"},
		{ID: "b", Want: "test-file", Got: "test-file"},
		{ID: "c", Want: "test-file", Got: "none"},
	}
	s := Summarize(rows)
	if s.Total != 3 || s.Agree != 2 || len(s.Disagree) != 1 || s.Disagree[0].ID != "c" {
		t.Fatalf("summary = %+v", s)
	}
	if s.Pairs[0].Want != "test-file" || s.Pairs[0].Got != "none" || s.Pairs[0].N != 1 {
		t.Fatalf("pairs = %+v", s.Pairs)
	}
}
