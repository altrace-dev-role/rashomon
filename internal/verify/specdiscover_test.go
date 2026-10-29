package verify

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestSP13_EvaluateSeesDiscovered: a temp tree with .git and a repository
// spec at its root, discovered through OSFS from two levels below and
// handed to Evaluate: the snapshot's spec path is the repository spec's and
// its digest the Compiled's. A refused spec nearer than it reaches the
// snapshot as the Compiled's Errors line, with no path.
func TestSP13_EvaluateSeesDiscovered(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(base, "a", "b")
	for _, dir := range []string{filepath.Join(base, ".git"), cwd} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	root := baseline.Key(cwd)
	if root != base {
		t.Fatalf("baseline.Key(%s) = %s, want %s", cwd, root, base)
	}
	put := func(dir, doc string) string {
		t.Helper()
		path := filepath.Join(dir, ".claude", "rashomon", "spec.json")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	evaluate := func() (*spec.Compiled, *SpecInfo) {
		t.Helper()
		sp, err := spec.Discover(spec.OSFS{}, spec.Locations(cwd, root), "")
		if err != nil || sp == nil {
			t.Fatalf("Discover: %+v, %v", sp, err)
		}
		snap := Evaluate(&store.Run{Dir: "s1"}, &ProjectIndex{ProjectKey: "k"}, sp, Point{Kind: PointStop, Seq: 1, SessionID: "s1"}, time.Unix(0, 0))
		return sp, snap.Spec
	}

	path := put(base, `{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]}]}`)
	sp, info := evaluate()
	if info == nil || info.Path != path || info.Digest != sp.Digest || info.Trust != spec.Untrusted || info.Errors == nil || len(info.Errors) != 0 {
		t.Errorf("the snapshot's spec %+v, want path %s, digest %s, untrusted, no errors", info, path, sp.Digest)
	}

	refused := put(filepath.Join(base, "a"), `{"rashomon_spec": 2}`)
	sp, info = evaluate()
	if len(sp.Errors) != 1 || !strings.HasPrefix(sp.Errors[0], refused+": spec: ") || len(sp.Sources) != 0 {
		t.Fatalf("the refused spec: errors %q", sp.Errors)
	}
	if info == nil || info.Path != "" || info.Digest != sp.Digest || !slices.Equal(info.Errors, sp.Errors) {
		t.Errorf("the snapshot's spec %+v, want no path and the line %q", info, sp.Errors)
	}
}
