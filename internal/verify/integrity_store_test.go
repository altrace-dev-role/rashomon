package verify

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
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

// storeTouchedAt reports whether integrity over r for the window (0, at],
// with storeRoot, names a store_touched entry: the flag StoreWritten's trust
// drop has to agree with.
func storeTouchedAt(r *store.Run, storeRoot string, at int64) bool {
	_, writes, _, _ := classify(r)
	for _, e := range integrity(r, pickExecutions(r), writes, 0, at, storeRoot).Unresolved {
		if e.Check == StoreTouched {
			return true
		}
	}
	return false
}

// TestST1s_StoreWrittenNamesTheStoreTouchedWrites (D173): StoreWritten, the
// trust drop's read, and integrity's store_touched over the store root agree
// on every write: a file tool's path or a shell call's named path under the
// root once cleaned, or the root itself, at or before at, read either way;
// not a sibling whose name only starts like the root's, a path that cleans
// out of it, an MCP call, a write after at, or a read with no root. A root
// inside the project, where the record names the path relative to the
// project, matches on the path as the call named it. A write under the
// project's .claude/rashomon/ is integrity's alone: the trust drop reads the
// store root. A clean path costs StoreWritten no allocation.
func TestST1s_StoreWrittenNamesTheStoreTouchedWrites(t *testing.T) {
	const root = "/home/u/.local/state/rashomon"
	mcp := runOf(t, writeCall(1, root+"/spec-trust.json"))
	mcp.Declarations[0].Shape.VerbClass = shape.VerbMCP
	inProject := runOf(t, writeCall(1, "/repo/.state/rashomon/spec-trust.json"))
	project := "/repo"
	inProject.Declarations[0].Project = &project
	later := runOf(t, edit(1, "pkg/a.go"), writeCall(3, root+"/spec-trust.key"))
	for _, c := range []struct {
		name string
		r    *store.Run
		root string
		at   int64
		want bool
	}{
		{"a Write under the root", runOf(t, writeCall(1, root+"/spec-trust.json")), root, 2, true},
		{"the root itself", runOf(t, edit(1, root)), root, 2, true},
		{"a shell write's second path", runOf(t, bashWrite(1, "/tmp/x", root+"/spec-trust.key")), root, 2, true},
		{"a path through ..", runOf(t, writeCall(1, "/home/u/.local/state/x/../rashomon/spec-trust.json")), root, 2, true},
		{"a root with a trailing slash", runOf(t, writeCall(1, root+"/spec-trust.json")), root + "/", 2, true},
		{"a backslashed path and root", runOf(t, writeCall(1, `C:\Users\u\rashomon\k`)), `C:\Users\u\rashomon`, 2, true},
		{"a root inside the project", inProject, "/repo/.state/rashomon", 2, true},
		{"a write at at", later, root, 3, true},
		{"a write after at", later, root, 2, false},
		{"a path that cleans out of the root", runOf(t, writeCall(1, root+"/../x")), root, 2, false},
		{"a sibling", runOf(t, writeCall(1, root+"x/spec-trust.json")), root, 2, false},
		{"an MCP call", mcp, root, 2, false},
		{"no root", runOf(t, writeCall(1, root+"/spec-trust.json")), "", 2, false},
	} {
		if got, named := StoreWritten(c.r, c.root, c.at), storeTouchedAt(c.r, c.root, c.at); got != c.want || named != c.want {
			t.Errorf("%s: StoreWritten %v, store_touched %v; want %v", c.name, got, named, c.want)
		}
	}
	if StoreWritten(nil, root, 2) {
		t.Error("StoreWritten over no run: true")
	}
	local := runOf(t, edit(1, ".claude/rashomon/spec.json"))
	if StoreWritten(local, root, 2) || !storeTouchedAt(local, root, 2) {
		t.Errorf("the project's .claude/rashomon: StoreWritten %v, store_touched %v; want false, true",
			StoreWritten(local, root, 2), storeTouchedAt(local, root, 2))
	}
	clean := runOf(t, edit(1, "pkg/a.go"), bashWrite(3, "/tmp/x"), writeCall(5, root+"/spec-trust.json"))
	if n := testing.AllocsPerRun(20, func() { StoreWritten(clean, root, 6) }); n != 0 {
		t.Errorf("StoreWritten over clean paths: %v allocs, want 0", n)
	}
}
