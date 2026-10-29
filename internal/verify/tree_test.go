package verify

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// hashedEdit is an Edit of path whose post recorded sha for it.
func hashedEdit(seq int64, path, sha string) call {
	c := edit(seq, path)
	c.sha = sha
	return c
}

// treeAt is the one gate and the ledger at a stop point past every record of
// r, with tree as the point's.
func treeAt(t *testing.T, r *store.Run, tree map[string]*string) (Snapshot, GateState, []Obligation) {
	t.Helper()
	snap, obs := EvaluateWithObligations(r, nil, nil, Point{Kind: PointStop, Seq: 100, SessionID: "s1", Tree: tree}, time.Time{})
	return snap, one(t, snap.Gates), obs
}

// shaA, shaB and shaC stand for three contents of one file.
var (
	shaA = strings.Repeat("a", 64)
	shaB = strings.Repeat("b", 64)
	shaC = strings.Repeat("c", 64)
)

// TestPG6a_EditAfterTheRunChangesTheDigest: a green run is bound to the tree
// it ran on, the last sha the session's writes before it recorded per relevant
// path (card P5A 5.1, PF-PG6, INV-14). Read as the run left it, the gate is
// Green and evidence_bound met on the green run: also where an earlier write
// to the path was overwritten before the run, and where a docs file the tree
// does not hold moved. The session's own change since reads Stale
// writes_after_green, evidence_bound unmet on the green run: an Edit after the
// run, and a moved tree beside an unnamed write, which may be the session's.
func TestPG6a_EditAfterTheRunChangesTheDigest(t *testing.T) {
	for _, tc := range []struct {
		name   string
		calls  []call
		gate   int // the index of the green run among calls
		tree   map[string]*string
		state  State
		reason Reason
		bound  ObligationState
	}{
		{"the tree as the run left it", []call{hashedEdit(1, "pkg/a.go", shaA), gateCall(2, "go-test", store.ExecOK)},
			1, map[string]*string{"pkg/a.go": &shaA}, Green, OK, ObligationMet},
		{"the last write before the run", []call{hashedEdit(1, "pkg/a.go", shaA), hashedEdit(2, "pkg/a.go", shaB), gateCall(3, "go-test", store.ExecOK)},
			2, map[string]*string{"pkg/a.go": &shaB}, Green, OK, ObligationMet},
		{"a docs file moved", []call{hashedEdit(1, "pkg/a.go", shaA), hashedEdit(2, "docs/a.md", shaA), gateCall(3, "go-test", store.ExecOK)},
			2, map[string]*string{"pkg/a.go": &shaA, "docs/a.md": &shaB}, Green, OK, ObligationMet},
		{"an Edit after the run", []call{hashedEdit(1, "pkg/a.go", shaA), gateCall(2, "go-test", store.ExecOK), hashedEdit(3, "pkg/a.go", shaB)},
			1, map[string]*string{"pkg/a.go": &shaB}, Stale, WritesAfterGreen, ObligationUnmet},
		{"a moved tree beside an unnamed write", []call{hashedEdit(1, "pkg/a.go", shaA), gateCall(2, "go-test", store.ExecOK), unnamedWrite(3)},
			1, map[string]*string{"pkg/a.go": &shaB}, Stale, WritesAfterGreen, ObligationUnmet},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runOf(t, tc.calls...)
			_, g, obs := treeAt(t, r, tc.tree)
			wantState(t, g, tc.state, tc.reason)
			wantRow(t, rowOf(t, obs, EvidenceBound), tc.bound, declSeq(r, tc.gate))
		})
	}
}

// TestPG6b_OverCapIsUnknown: a tree the record or the point cannot give is
// unknown, never met (card P5A 5.1, INV-2). TreeTargets names each relevant
// path a session's hashed writes name once, in path order, with the file the
// last write named it by, and none over MaxTreeFiles: 64 paths are 64
// targets, 65 are nil, so the Stop reads no file and the point has no tree.
// With no tree, a write whose post recorded no sha, or a path the point has
// no entry for, the gate stays Green and evidence_bound is unknown with no
// evidence; a run with no write before it is met against an empty tree. A
// schema 6 execution written before the key (added in place, D140) decodes
// it null, the no-sha row: unknown, fail closed.
func TestPG6b_OverCapIsUnknown(t *testing.T) {
	edits := func(n int) []call {
		calls := make([]call, n)
		for i := range calls {
			calls[i] = hashedEdit(int64(i+1), fmt.Sprintf("/repo/pkg/f%02d.go", n-i), shaA)
		}
		return calls
	}
	got := TreeTargets(runOf(t, edits(MaxTreeFiles)...))
	if len(got) != MaxTreeFiles || !slices.IsSortedFunc(got, func(a, b TreeTarget) int { return strings.Compare(a.Path, b.Path) }) {
		t.Fatalf("64 paths: %d targets, sorted %t; want 64 in path order", len(got), slices.IsSortedFunc(got, func(a, b TreeTarget) int { return strings.Compare(a.Path, b.Path) }))
	}
	if got := TreeTargets(runOf(t, edits(MaxTreeFiles+1)...)); got != nil {
		t.Fatalf("65 paths: %d targets, want nil", len(got))
	}

	r := runOf(t, edit(1, "pkg/a.go"), edit(2, "pkg/b.go"), hashedEdit(3, "pkg/b.go", shaA), edit(4, "pkg/c.go"), edit(5, "docs/a.md"), bashWrite(6, "pkg/d.go"))
	old, cwd := filepath.FromSlash("/old"), filepath.FromSlash("/repo")
	r.Declarations[1].CWD, r.Declarations[2].CWD, r.Declarations[4].CWD = &old, &cwd, &cwd
	want := []TreeTarget{{Path: "pkg/b.go", File: filepath.Join(cwd, "pkg/b.go")}}
	if got := TreeTargets(r); !slices.Equal(got, want) {
		t.Errorf("TreeTargets %+v, want %+v: a relative path joins the cwd of the last write to name it, and one with none, a docs file and a Bash write are no target", got, want)
	}
	if got := TreeTargets(nil); got == nil || len(got) != 0 {
		t.Errorf("TreeTargets(nil) = %#v, want empty", got)
	}
	var before store.Execution
	if err := json.Unmarshal([]byte(`{"type":"execution","schema_version":6,"tool_use_id":"t1","tool_name":"Edit","outcome":"ok"}`), &before); err != nil || before.ContentSHA256 != nil {
		t.Errorf("a v6 execution with no content_sha256 decodes %v, %v; want nil, nil", before.ContentSHA256, err)
	}

	for _, tc := range []struct {
		name  string
		calls []call
		tree  map[string]*string
		bound ObligationState
	}{
		{"no tree at the point", []call{hashedEdit(1, "pkg/a.go", shaA), gateCall(2, "go-test", store.ExecOK)}, nil, ObligationUnknown},
		{"no write and no tree", []call{gateCall(1, "go-test", store.ExecOK)}, nil, ObligationUnknown},
		{"no write and an empty tree", []call{gateCall(1, "go-test", store.ExecOK)}, map[string]*string{}, ObligationMet},
		{"a write with no sha", []call{edit(1, "pkg/a.go"), gateCall(2, "go-test", store.ExecOK)}, map[string]*string{"pkg/a.go": &shaA}, ObligationUnknown},
		{"a path the point did not read", []call{hashedEdit(1, "pkg/a.go", shaA), gateCall(2, "go-test", store.ExecOK)}, map[string]*string{}, ObligationUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runOf(t, tc.calls...)
			_, g, obs := treeAt(t, r, tc.tree)
			wantState(t, g, Green, OK)
			var evidence []int64
			if tc.bound == ObligationMet {
				evidence = []int64{g.LastGreen.Seq}
			}
			wantRow(t, rowOf(t, obs, EvidenceBound), tc.bound, evidence...)
		})
	}
}

// TestCONC1_ForeignEditIsUnknownNotUnmet: a path of the green run's tree that
// moved with no write in this session's record since is a foreign edit (card
// P5A 5.2): the gate is Stale with stale_foreign, so the record is not green,
// and both its gate_after_write and evidence_bound rows are unknown with no
// evidence, never unmet (INV-2). A file gone is moved; one moved path decides
// beside one the point could not read; the moved path is the last of eight.
func TestCONC1_ForeignEditIsUnknownNotUnmet(t *testing.T) {
	var calls []call
	tree := map[string]*string{}
	for i, name := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		path := "pkg/" + name + ".go"
		calls = append(calls, hashedEdit(int64(i+1), path, shaA))
		tree[path] = &shaA
	}
	calls = append(calls, gateCall(9, "go-test", store.ExecOK))
	for _, tc := range []struct {
		name string
		edit func(tree map[string]*string)
	}{
		{"one of eight moved", func(tree map[string]*string) { tree["pkg/h.go"] = &shaC }},
		{"a file gone", func(tree map[string]*string) { tree["pkg/h.go"] = nil }},
		{"a move beside a file unread", func(tree map[string]*string) { tree["pkg/h.go"] = &shaC; delete(tree, "pkg/a.go") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := map[string]*string{}
			for p, s := range tree {
				at[p] = s
			}
			tc.edit(at)
			snap, g, obs := treeAt(t, runOf(t, calls...), at)
			wantState(t, g, Stale, StaleForeign)
			if g.WritesSinceGreen != 0 || g.UnknownTarget != 0 || snap.RecordGreen {
				t.Errorf("writes %d, unknown target %d, record green %t; want 0, 0, false", g.WritesSinceGreen, g.UnknownTarget, snap.RecordGreen)
			}
			wantRow(t, rowOf(t, obs, GateAfterWrite), ObligationUnknown)
			wantRow(t, rowOf(t, obs, EvidenceBound), ObligationUnknown)
		})
	}
}
