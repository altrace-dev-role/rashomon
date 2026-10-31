package verify

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestST1x_TheStoreRootBitStandsForUnnamedCalls (D173c): a call integrity
// counts unnamed (an MCP call, a may-write with no list, a shell call whose
// written list is incomplete, a file tool's write with no file_path) is a
// store write for StoreWritten and a store_touched entry with no path when
// its input named the store root, at record scope v5 as at v4, the same call
// dropping at both. A call that did not name the root, or one whose written
// list is complete and elsewhere, is neither: the bit is read only where the
// record cannot name the target.
func TestST1x_TheStoreRootBitStandsForUnnamedCalls(t *testing.T) {
	const root = "/home/u/.local/state/rashomon"
	yes, no := true, false
	build := func(named bool, scope string, tweak func(d *store.Declaration), c call) *store.Run {
		r := runOf(t, c)
		d := &r.Declarations[0]
		tweak(d)
		d.StoreNamed = named
		store.GateDeclaration(d, scope)
		return r
	}
	mcp := func(d *store.Declaration) { d.Shape.VerbClass = shape.VerbMCP }
	mayWrite := func(d *store.Declaration) { d.Shape.MayWrite, d.WrittenPaths = true, nil }
	incomplete := func(d *store.Declaration) { d.WrittenPathsComplete = &no }
	complete := func(d *store.Declaration) { d.WrittenPathsComplete = &yes }
	noFile := func(d *store.Declaration) { d.FilePath = nil }
	keep := func(*store.Declaration) {}
	p := root + "/spec-trust.json"
	withPath := func(d *store.Declaration) { d.FilePath = &p }
	withCmd := func(d *store.Declaration) { d.Command = &p }
	for _, scope := range []string{store.RecordScopeV5, store.RecordScopeV4} {
		for _, c := range []struct {
			name  string
			r     *store.Run
			named bool
			want  bool
		}{
			{"an MCP call that named the root", build(true, scope, mcp, writeCall(1, "/tmp/x")), true, true},
			{"an MCP call that did not", build(false, scope, mcp, writeCall(1, "/tmp/x")), false, false},
			{"a may-write with no list", build(true, scope, mayWrite, bashWrite(1, "/tmp/x")), true, true},
			{"a may-write with no list, not naming", build(false, scope, mayWrite, bashWrite(1, "/tmp/x")), false, false},
			{"an incomplete written list", build(true, scope, incomplete, bashWrite(1, "/tmp/x")), true, true},
			{"an incomplete list, not naming", build(false, scope, incomplete, bashWrite(1, "/tmp/x")), false, false},
			{"a Write with no file_path", build(true, scope, noFile, writeCall(1, "/tmp/x")), true, true},
			{"a Read that named the root", build(true, scope, withPath, call{seq: 1, tool: "Read", class: shape.VerbRead, outcome: store.ExecOK}), true, scope == store.RecordScopeV4},
			{"a Bash read that named the root", build(true, scope, withCmd, call{seq: 1, tool: "Bash", prog: "cat", class: shape.VerbRead, outcome: store.ExecOK}), true, scope == store.RecordScopeV4},
			{"a complete list elsewhere", build(true, scope, complete, bashWrite(1, "/tmp/x")), true, scope == store.RecordScopeV4},
			{"a Write whose path is elsewhere", build(true, scope, keep, writeCall(1, "/tmp/x")), true, scope == store.RecordScopeV4},
		} {
			if got, entry := StoreWritten(c.r, root, 2), storeTouchedAt(c.r, root, 2); got != c.want || entry != c.want {
				t.Errorf("%s at %s: StoreWritten %v, store_touched %v; want %v", c.name, scope, got, entry, c.want)
			}
		}
	}
	// A call whose recorded path shows the root is the one path entry, not
	// that and the bit's.
	shown := build(true, store.RecordScopeV5, incomplete, bashWrite(1, root+"/spec-trust.json"))
	_, sw, _, _ := classify(shown)
	if got := integrity(shown, pickExecutions(shown), sw, 0, 2, root).Unresolved; len(got) != 1 || got[0].Path == "" {
		t.Errorf("an incomplete list that shows the root: entries %+v, want one with its path", got)
	}
	r := build(true, store.RecordScopeV5, mcp, writeCall(1, "/tmp/x"))
	_, writes, _, _ := classify(r)
	if got := integrity(r, pickExecutions(r), writes, 0, 2, root).Unresolved; len(got) != 1 || got[0] != (IntegrityEntry{Check: StoreTouched, Seq: 1}) {
		t.Errorf("the v5 MCP call's entries %+v, want one store_touched at seq 1 with no path", got)
	}
}

// TestST1y_UnnamedWriteIsAppendWritesUnnamed (D173c): writesUnnamed and
// appendWrites' unnamed list read the same calls, over every shape the two
// look at, so the store-root bit is read exactly where integrity counts a
// call unnamed.
func TestST1y_UnnamedWriteIsAppendWritesUnnamed(t *testing.T) {
	yes, no := true, false
	for _, tool := range []string{"Bash", "Write", "Edit", "mcp__x__y"} {
		for _, verb := range []string{shape.VerbWrite, shape.VerbMCP, shape.VerbExecute, shape.VerbTest} {
			for _, may := range []bool{false, true} {
				for _, paths := range [][]string{nil, {"/tmp/a"}} {
					for _, complete := range []*bool{nil, &yes, &no} {
						for _, file := range []*string{nil, new(string)} {
							d := &store.Declaration{Seq: 1, ToolName: tool}
							d.FilePath, d.WrittenPaths, d.WrittenPathsComplete = file, paths, complete
							d.Shape.VerbClass, d.Shape.MayWrite = verb, may
							_, unnamed := appendWrites(nil, nil, d)
							if got := writesUnnamed(d); got != (len(unnamed) > 0) {
								t.Errorf("%s %v may=%v paths=%v complete=%v file=%v: writesUnnamed %v, appendWrites unnamed %v",
									tool, verb, may, paths, complete, file != nil, got, unnamed)
							}
						}
					}
				}
			}
		}
	}
}
