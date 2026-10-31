package disclose

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestSV1j_FiredMarkersAreInTheInventory: each hook event's fired marker in
// a run directory is described by one 0600 store row that says purge and
// eviction remove it; TestTRU23_EveryWrittenPathIsInTheInventory drives the
// writer itself. Break: drop the row, and the screen lists a file kind the
// store writes as never written.
func TestSV1j_FiredMarkersAreInTheInventory(t *testing.T) {
	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range store.FiredEvents() {
		rel := store.DirRuns + "/s1/" + store.FileFiredPrefix + e
		if !matchesAFileRule(rel, inv.Files) {
			t.Errorf("Default().Files does not cover %s", rel)
		}
	}
	rows := 0
	for _, r := range inv.Files {
		if r.Pattern != store.FileFiredPrefix+"*" {
			continue
		}
		rows++
		if r.Base != store.DirRuns+"/<session>" || r.Mode != "0600" || r.Owner != "store" || !strings.Contains(r.RemovedBy, "purge") || !strings.Contains(r.RemovedBy, "eviction") {
			t.Errorf("fired row = %+v, want a 0600 store row under the run, removed by eviction and purge", r)
		}
	}
	if rows != 1 {
		t.Errorf("%d rows describe the fired markers, want 1", rows)
	}
}
