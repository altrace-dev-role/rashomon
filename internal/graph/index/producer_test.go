package index

import (
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/lang/pylite"
)

// TestBuild_PyProducerIsTheReadersOwn: the producer string Build writes
// for py, read from the provenance of the records pylite extracts
// (producerOf), equals the string pylite's resolver states (Producer),
// which a hook compares with the header's (lang.Registry, Unindexed). Two
// paths reach the one value. Break: change the string one path gives and
// not the other, and an index this build wrote reads as an older reader's,
// so every .py edit under it says the unindexed line.
func TestBuild_PyProducerIsTheReadersOwn(t *testing.T) {
	root := filepath.Join(t.TempDir(), "r")
	put(t, filepath.Join(root, "py", "app.py"), "def run():\n    return 1\n")
	r, err := Build(root, "c1", pyUnit)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := r.Header.Producers[pylite.Lang]
	if !ok {
		t.Fatalf("the header's producers %v name no %s producer; want the one its records carry", r.Header.Producers, pylite.Lang)
	}
	v, ok := any(pylite.Resolver{}).(interface{ Producer() string })
	if !ok {
		t.Fatal("pylite.Resolver states no producer string (no Producer method)")
	}
	if want := v.Producer(); got != want {
		t.Errorf("Build wrote py producer %q; the reader states %q", got, want)
	}
}
