package graph

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestFindIndex_AWalkWithNoIndexAllocatesTwoPerDirectory: a walk that finds
// no index costs, per directory it probes, the index path's join and the
// NUL-terminated copy the stat needs, nothing more: no FileInfo, no
// *PathError (indexAt). The hook pays this walk on every Go edit outside an
// index, so it is pinned at its floor. The control: the walk finds an index
// put at the top directory it probes. Break: probe with os.Stat.
func TestFindIndex_AWalkWithNoIndexAllocatesTwoPerDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("indexAt is os.Stat where there is no unix stat")
	}
	top := t.TempDir()
	dir := filepath.Join(top, "a", "b", "c", "d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := FindIndex(dir); got != "" {
		t.Fatalf("FindIndex(%s) = %q with no index under %s", dir, got, top)
	}
	levels := MaxIndexDepth + 1
	if n := testing.AllocsPerRun(20, func() { FindIndex(dir) }); n > float64(2*levels) {
		t.Errorf("a walk of %d directories with no index allocates %.0f, want at most %d (two per directory)", levels, n, 2*levels)
	}
	idx := filepath.Join(top, filepath.FromSlash(GraphFile))
	if err := os.MkdirAll(filepath.Dir(idx), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(idx, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := FindIndex(dir); got != top {
		t.Errorf("control: FindIndex(%s) = %q, want %s, the top directory the walk probes", dir, got, top)
	}
}
