package acceptance

import (
	"context"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/platform"
)

// TestH156d_SnapshotReadsWhatSyncWrote (W-M1.2, AC-M1.0): the sidecar set the
// real pgraph writes under a synced module reads whole through
// platform.ReadSnapshot, the Go producer's one read: a Place per declared
// symbol at the file and line pgraph placed it, the PROVEN call from b.G to
// a.F among the facts, and each package's files from the digests. Break:
// write the symbols file under another header, chain it to other digests or
// leave it out of the walk file, and the read is an error, never a part.
func TestH156d_SnapshotReadsWhatSyncWrote(t *testing.T) {
	dir := gi1Project(t, h156Files)
	h156Sync(t, dir)
	s, err := platform.ReadSnapshot(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	declared := 0
	for _, f := range s.Facts {
		if f.Predicate == platform.PredDeclares {
			declared++
		}
	}
	if declared == 0 || len(s.Places) != declared {
		t.Errorf("%d places for %d declared symbols; want one each", len(s.Places), declared)
	}
	want := map[string]platform.Place{
		"func:example.com/m/a.F": {ID: "func:example.com/m/a.F", Pkg: "example.com/m/a", File: "a/a.go", Line: 3},
		"func:example.com/m/b.G": {ID: "func:example.com/m/b.G", Pkg: "example.com/m/b", File: "b/b.go", Line: 5},
	}
	for id, w := range want {
		i := slices.IndexFunc(s.Places, func(p platform.Place) bool { return p.ID == id })
		if i < 0 || s.Places[i] != w {
			t.Errorf("%s placed as %+v; want %+v", id, s.Places[max(i, 0)], w)
		}
	}
	if !slices.Contains(s.Facts, platform.Fact{Subject: "func:example.com/m/b.G", Predicate: platform.PredCalls, Object: "func:example.com/m/a.F", Tier: platform.TierProven, Config: s.Facts[0].Config}) {
		t.Errorf("no PROVEN call from b.G to a.F among %d facts: %+v", len(s.Facts), s.Facts)
	}
	for pkg, file := range map[string]string{"example.com/m/a": "a/a.go", "example.com/m/b": "b/b.go"} {
		if !slices.Contains(s.Files[pkg], file) {
			t.Errorf("files of %s = %v; want %s among them", pkg, s.Files[pkg], file)
		}
	}
}
