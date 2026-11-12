package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestGraphEvidence_OnlyAtRecordScopeV5 (W-M2.11 1d): the evidence file names
// the brief's ids and paths in clear, which the steer line at record scope v4
// keeps only as keyed digests and counts (Brief.Record), so the prompt writes
// it only where the store's record_scope marker says v5: no marker, which
// reads as v4, and a v4 marker leave no file under the store's graph
// directory; v5 leaves the one file. The scope is read once a brief exists
// and not before: a cwd with no index never reads it. Break: drop the scope
// condition in writeGraphEvidence, and the v4 stores hold a file.
func TestGraphEvidence_OnlyAtRecordScopeV5(t *testing.T) {
	dir := t.TempDir()
	graphProject(t, dir)
	cwd := filepath.Join(dir, "services", "auth")
	const prompt = "purging the sessions"
	for _, tc := range []struct {
		name, marker string
		want         bool
	}{
		{"no marker", "", false},
		{"v4", store.RecordScopeV4, false},
		{"v5", store.RecordScopeV5, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.marker != "" {
				if err := store.WriteRecordScopeMarker(root, tc.marker); err != nil {
					t.Fatal(err)
				}
			}
			if _, ok := graphBrief(root, cwd, "s1", "p1", []byte(prompt)); !ok {
				t.Fatal("no brief")
			}
			_, err := os.Stat(filepath.Join(root, graph.StoreDir, graph.EvidenceFileName("s1", "p1", time.Now())))
			if got := err == nil; got != tc.want {
				t.Errorf("evidence file written = %v (%v), want %v", got, err, tc.want)
			}
		})
	}
	t.Run("read once a brief exists", func(t *testing.T) {
		reads := 0
		scopeOf := func() string { reads++; return store.RecordScopeV5 }
		root := t.TempDir()
		if _, ok := graphBriefWith(root, t.TempDir(), "s1", "p1", []byte(prompt), nil, scopeOf); ok || reads != 0 {
			t.Errorf("a cwd with no index: brief %v, scope read %d times; want none, and no read", ok, reads)
		}
		if _, ok := graphBriefWith(root, cwd, "s1", "p2", []byte(prompt), nil, scopeOf); !ok || reads != 1 {
			t.Errorf("an indexed cwd: brief %v, scope read %d times; want a brief and one read", ok, reads)
		}
	})
}
