package pylite

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// writeSafeLink makes link a symlink to target, or skips the test: Windows
// creates one only with the privilege or developer mode.
func writeSafeLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("no symlink here: %v", err)
	}
}

// writeSafeRoot is a root with one unit, app, and the resolver for it.
func writeSafeRoot(t *testing.T) (string, Resolver) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, Resolver{Units: []Unit{{Path: "app"}}}
}

// writeSafePyOf is Python source of exactly n bytes, n at least 7.
func writeSafePyOf(n int) []byte {
	return []byte("x = 1\n" + strings.Repeat("#", n-7) + "\n")
}

// TestWS1_J10_ExtractRefusesASourceLinkedOutsideTheRoot: a listed .py that
// is a link to a file outside the root is refused with an error naming it,
// and nothing of the outside file is parsed. Break: read the file by path.
func TestWS1_J10_ExtractRefusesASourceLinkedOutsideTheRoot(t *testing.T) {
	root, r := writeSafeRoot(t)
	out := filepath.Join(t.TempDir(), "outside.py")
	if err := os.WriteFile(out, []byte("def outside_only():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSafeLink(t, out, filepath.Join(root, "app", "x.py"))
	recs, err := r.Extract(root, []string{"app/x.py"}, graph.RevCommit("C1"))
	if err == nil || !strings.Contains(err.Error(), "x.py") {
		t.Errorf("Extract of a source linked outside the root = %d record(s), %v; want an error naming app/x.py", len(recs), err)
	}
}

// TestWS1_J12_ExtractRefusesASourceOverTheBound: a listed .py one byte over
// the bound is refused with an error naming it and the bound; its control,
// exactly at the bound, is parsed. Break: read a source whole, whatever its
// size.
func TestWS1_J12_ExtractRefusesASourceOverTheBound(t *testing.T) {
	root, r := writeSafeRoot(t)
	if err := os.WriteFile(filepath.Join(root, "app", "big.py"), writeSafePyOf(maxSourceBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "at.py"), writeSafePyOf(maxSourceBytes), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := r.Extract(root, []string{"app/big.py"}, graph.RevCommit("C1"))
	if err == nil || !strings.Contains(err.Error(), "big.py") || !strings.Contains(err.Error(), "1048576") {
		t.Errorf("Extract of a source one byte over the bound = %v; want an error naming app/big.py and 1048576", err)
	}
	if _, err := r.Extract(root, []string{"app/at.py"}, graph.RevCommit("C1")); err != nil {
		t.Errorf("control, a source exactly at the bound: Extract = %v; want it parsed", err)
	}
}

// TestWS1_J13_ExtractFollowsARelativeLinkInsideTheRoot: a listed .py that is
// a relative link to a file inside the root is read and parsed. The limit of
// the refusal. Break: refuse every link.
func TestWS1_J13_ExtractFollowsARelativeLinkInsideTheRoot(t *testing.T) {
	root, r := writeSafeRoot(t)
	if err := os.WriteFile(filepath.Join(root, "app", "real.py"), []byte("def inside():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSafeLink(t, "real.py", filepath.Join(root, "app", "x.py"))
	recs, err := r.Extract(root, []string{"app/x.py"}, graph.RevCommit("C1"))
	if err != nil || len(recs) == 0 {
		t.Errorf("Extract through a link inside the root = %d record(s), %v; want it parsed", len(recs), err)
	}
}
