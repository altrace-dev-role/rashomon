package intent_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/intent"
)

// plannedRoot is a project with internal/graph/g.go, an empty directory
// docs, and a link internal/link that points at docs.
func plannedRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"internal/graph", "docs"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "internal/graph/g.go"), []byte("package g\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "internal/link")); err != nil {
		t.Skip("no symlinks here:", err)
	}
	return root
}

// TestResolvePlan_PlannedIsMissingUnderAnExistingDirectory: pins (1)-(6) of
// the hook half's rulings. A missing parent gives nothing; an existing parent
// gives the file as planned; an existing file is existing, as Resolve keeps
// it; a name at the root is planned; a ".." escape, an absolute path outside
// the root and a file under a link give nothing.
func TestResolvePlan_PlannedIsMissingUnderAnExistingDirectory(t *testing.T) {
	root := plannedRoot(t)
	outside := t.TempDir()
	named := intent.NamedPaths([]byte("edit internal/graph/g.go, add internal/graph/new.go and internal/nodir/x.go and top.go, " +
		"also ../escape.go and " + filepath.Join(outside, "o.go") + " and internal/link/z.go and docs/sub/deep.go"))
	existing, planned := intent.ResolvePlan(named, intent.Project{Root: root, Cwd: root})
	if want := []string{"internal/graph/g.go"}; !slices.Equal(existing, want) {
		t.Errorf("existing = %v, want %v", existing, want)
	}
	if want := []string{"internal/graph/new.go", "top.go"}; !slices.Equal(planned, want) {
		t.Errorf("planned = %v, want %v", planned, want)
	}
}

// TestResolvePlan_ExistingIsWhatResolveKeeps: the existing half is what
// Resolve keeps for the same names, and a prompt naming a missing file under
// an existing directory resolves as at base (Resolve keeps nothing for it).
func TestResolvePlan_ExistingIsWhatResolveKeeps(t *testing.T) {
	root := plannedRoot(t)
	text := []byte("look at internal/graph/g.go and docs/ and internal/graph/new.go")
	p := intent.Project{Root: root, Cwd: root}
	existing, _ := intent.ResolvePlan(intent.NamedPaths(text), p)
	if got := intent.Resolve(text, false, p).Paths; !slices.Equal(existing, got) || !slices.Equal(got, []string{"internal/graph/g.go", "docs"}) {
		t.Errorf("Resolve.Paths = %v, ResolvePlan existing = %v, want both [internal/graph/g.go docs]", got, existing)
	}
}

// TestResolvePlan_FortyNamesCostAtMostThirtyTwoLookups: pin (3): the parent
// lookups spend what the existing paths left of the one budget, and a repeated
// lookup is answered once. Here the 32 names tried first spend all of it, so
// no parent is looked up and nothing is planned: the bound is the pin.
func TestResolvePlan_FortyNamesCostAtMostThirtyTwoLookups(t *testing.T) {
	root := plannedRoot(t)
	var text []byte
	for i := 0; i < 40; i++ {
		text = fmt.Appendf(text, "internal/graph/n%02d.go internal/dir%02d/x.go ", i, i)
	}
	text = fmt.Appendf(text, "internal/graph/n00.go")
	calls := 0
	p := intent.Project{Root: root, Cwd: root, Lstat: func(rel string) (fs.FileInfo, error) {
		calls++
		return os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	}}
	named := intent.NamedPaths(text)
	if len(named) < 40 {
		t.Fatalf("named %d paths, want at least 40", len(named))
	}
	_, planned := intent.ResolvePlan(named, p)
	if calls > intent.MaxLstat {
		t.Errorf("lookups = %d, want at most %d", calls, intent.MaxLstat)
	}
	if len(planned) > intent.MaxPaths {
		t.Errorf("planned = %d paths, want at most %d", len(planned), intent.MaxPaths)
	}
	t.Logf("ResolvePlan: %d names, %d lookups, %d planned", len(named), calls, len(planned))
}

// TestResolvePlan_NothingNamedNothingLooked: no names cost no lookup.
func TestResolvePlan_NothingNamedNothingLooked(t *testing.T) {
	p := intent.Project{Root: t.TempDir(), Lstat: func(string) (fs.FileInfo, error) {
		t.Error("a lookup with nothing named")
		return nil, fs.ErrNotExist
	}}
	if e, pl := intent.ResolvePlan(nil, p); e != nil || pl != nil {
		t.Errorf("got %v %v", e, pl)
	}
}
