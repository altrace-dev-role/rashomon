package editmarks

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestEM11_TheTableFitsTheStoresCaps: every id the table mints, and the basis
// word, is within the caps CapExecution enforces on an execution's marks, and
// the table's ids fit a map of at most MaxEditMarkIDs, so a mark of this
// package is never withheld as oversize.
func TestEM11_TheTableFitsTheStoresCaps(t *testing.T) {
	if len(table) > store.MaxEditMarkIDs {
		t.Errorf("the table holds %d ids, over the store's cap of %d", len(table), store.MaxEditMarkIDs)
	}
	m := &store.EditMarks{Added: map[string]int{}, Removed: map[string]int{}, Basis: Basis, Version: Version}
	for _, r := range table {
		if len(r.id) > store.MaxEditMarkIDBytes {
			t.Errorf("id %s is %d bytes, over the store's cap of %d", r.id, len(r.id), store.MaxEditMarkIDBytes)
		}
		m.Added[r.id], m.Removed[r.id] = 1<<30, 1<<30
	}
	x := store.Execution{}
	x.EditMarks = m
	if store.CapExecution(&x) || x.EditMarks == nil {
		t.Error("the whole table's marks at once were withheld by CapExecution")
	}
}
