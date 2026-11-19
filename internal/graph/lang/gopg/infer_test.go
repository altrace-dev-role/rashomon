package gopg

import (
	"slices"
	"testing"
)

// TestSETUP1_GO1_EachGoModOutsideTestdataIsAUnit: each go.mod is a unit at
// its directory, a nested one included, with mod_root its directory and an
// empty (not absent) module list; one under testdata or under a directory
// whose name starts with _ is not, and no other file makes a unit.
func TestSETUP1_GO1_EachGoModOutsideTestdataIsAUnit(t *testing.T) {
	files := []string{
		"_tools/go.mod",
		"go.mod",
		"go.sum",
		"internal/x/testdata/snap/go.mod",
		"main.go",
		"notes/go.mod.txt",
		"pgraph/go.mod",
		"pgraph/testdata/mod/go.mod",
		"pgraph/x.go",
		"tools/gen/go.mod",
	}
	got := Resolver{}.Infer(files)
	var paths []string
	for _, u := range got.Units {
		paths = append(paths, u.Path)
		if u.ModRoot != u.Path || u.Modules == nil || len(u.Modules) != 0 {
			t.Errorf("unit %q: mod_root %q, modules %#v; want mod_root its path and an empty list", u.Path, u.ModRoot, u.Modules)
		}
		want := u.Path + "/go.mod"
		if u.Path == "." {
			want = "go.mod"
		}
		if u.From != want || u.Rule == "" {
			t.Errorf("unit %q from %q by %q; want from %q and a rule named", u.Path, u.From, u.Rule, want)
		}
	}
	if want := []string{".", "pgraph", "tools/gen"}; !slices.Equal(paths, want) {
		t.Errorf("units %v; want %v", paths, want)
	}
	if len(got.Left) != 0 {
		t.Errorf("left out %v; want nothing named (the go tool skips those modules too)", got.Left)
	}
}
