package verify

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestTF10_AnAgentWriteToTheStoreIsAnIntegrityEntry (card 3.2, D131): an
// agent's write whose cleaned path carries a .claude/rashomon/ segment is an
// unresolved store_touched entry with the write's seq and path, one per path
// in seq then path order; it reaches the snapshot and holds RecordGreen false
// as every unresolved entry does. A write under .claude/ but not the store,
// under a name that only contains the segment's words, or outside the window
// is none.
func TestTF10_AnAgentWriteToTheStoreIsAnIntegrityEntry(t *testing.T) {
	task := ".claude/rashomon/task/current.json"
	r := runOf(t, edit(1, task), gateCall(2, "go-test", store.ExecOK))
	wantIntegrity(t, integrityOf(r, 0, 4), IntegrityCounts{}, IntegrityEntry{Check: StoreTouched, Seq: 1, Path: task})
	snap := evalAt(r, 4)
	if len(snap.Integrity.Unresolved) != 1 || snap.Integrity.Unresolved[0].Path != task {
		t.Errorf("snapshot integrity %+v; want the store_touched entry", snap.Integrity.Unresolved)
	}
	wantNoGreen(t, snap)
	wantGreenAt(t, evalAt(runOf(t, edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)), 4), 3)

	for _, p := range []string{".claude/rashomon/spec.json", ".claude/rashomon", "sub/.claude/rashomon/task/abc.json", "/repo/.claude/rashomon/x"} {
		wantIntegrity(t, integrityOf(runOf(t, edit(1, p)), 0, 2), IntegrityCounts{}, IntegrityEntry{Check: StoreTouched, Seq: 1, Path: p})
	}
	wantIntegrity(t, integrityOf(runOf(t, bashWrite(1, ".claude/rashomon/b", ".claude/rashomon/a")), 0, 2), IntegrityCounts{},
		IntegrityEntry{Check: StoreTouched, Seq: 1, Path: ".claude/rashomon/a"}, IntegrityEntry{Check: StoreTouched, Seq: 1, Path: ".claude/rashomon/b"})
	for _, p := range []string{".claude/settings.json", ".claude/rashomonx/y", "x.claude/rashomon/y", "docs/rashomon/task/current.json", "current.json"} {
		wantIntegrity(t, integrityOf(runOf(t, edit(1, p)), 0, 2), IntegrityCounts{})
	}
	wantIntegrity(t, integrityOf(r, 1, 4), IntegrityCounts{})
}
