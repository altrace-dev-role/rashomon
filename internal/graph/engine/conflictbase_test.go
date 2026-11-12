package engine

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// cbfx is the conflict-base fixture at C1: the boundary fixture, with a
// call from E1 into hookio (with cbCallAttrs and cbCallProv, which the
// slice drops) and an import of hookio by g.lx that stand at
// the base, a protected file keep/p.lx, a decided file docs/d.lx and a
// second file of its unit, docs/d2.lx; every file carries a content digest
// as the index writes it.
type cbfx struct {
	bfx
	fileP, fileD, fileD2 graph.ID
	p0, d0, d20          graph.ID
}

func newConflictBaseFx(t *testing.T) cbfx {
	t.Helper()
	c := cbfx{bfx: newBoundaryFx()}
	f := c.f
	f.s.AssertEdge(graph.Edge{Src: c.e1, Pred: graph.Calls, Dst: c.h1, Attrs: cbCallAttrs, Prov: cbCallProv, Rev: fxC1})
	f.edge(c.fileG, graph.Imports, fxUnit("internal/hookio"), fxC1, nil)
	c.fileP = f.file("keep", "keep/p.lx", fxC1)
	c.p0 = f.sym(c.fileP, "keep", "func", "P0", fxC1, nil)
	c.fileD = f.file("docs", "docs/d.lx", fxC1)
	c.d0 = f.sym(c.fileD, "docs", "func", "D0", fxC1, nil)
	c.fileD2 = f.file("docs", "docs/d2.lx", fxC1)
	c.d20 = f.sym(c.fileD2, "docs", "func", "D20", fxC1, nil)
	for _, id := range append(c.files(), c.fileP, c.fileD, c.fileD2) {
		f.s.AssertNode(graph.Node{ID: id, Kind: "file", Attrs: map[string]string{"digest": cbDigest(id, "base")}, Rev: fxC1})
		if n, ok := f.s.Node(id, fxC1); !ok || n.Attrs["digest"] != cbDigest(id, "base") {
			t.Fatalf("precondition: %s carries no base digest: %+v", id, n)
		}
	}
	return c
}

func cbDigest(file graph.ID, v string) string { return "sha256:" + string(file) + ":" + v }

// cbEdit is a landed group for file in unit at overlay 1 as the Go overlay
// writes one (gopg Overlay): the file node with its digest, the unit's
// Contains edge, each symbol the file declares after the edit with its
// Declares edge, and the file's Imports, under the Go overlay's carried set;
// extra adds edges a Go group never holds, a Calls a language extractor
// might carry.
func cbEdit(unit string, file graph.ID, digest string, syms, imports []graph.ID, extra ...graph.Edge) graph.OverlayGroup {
	recs := []graph.Record{
		{Node: &graph.Node{ID: file, Kind: "file", Attrs: map[string]string{"digest": digest}}},
		{Edge: &graph.Edge{Src: fxUnit(unit), Pred: graph.Contains, Dst: file}},
	}
	for _, s := range syms {
		recs = append(recs, graph.Record{Node: &graph.Node{ID: s, Kind: "sym"}},
			graph.Record{Edge: &graph.Edge{Src: file, Pred: graph.Declares, Dst: s}})
	}
	for _, u := range imports {
		recs = append(recs, graph.Record{Edge: &graph.Edge{Src: file, Pred: graph.Imports, Dst: u}})
	}
	for i := range extra {
		recs = append(recs, graph.Record{Edge: &extra[i]})
	}
	return graph.OverlayGroup{Kind: graph.OverlayLanded, File: file, Digest: digest, Commit: "C1", Seq: 1,
		Carried: graph.Carried{graph.Declares, graph.Imports}, Recs: recs}
}

// cbCase is one edit under one rule set. kind is the live kind the case
// raises, "" for a control; want is the number of live conflicts both the
// slice and the whole index raise.
type cbCase struct {
	name  string
	kind  ConflictKind
	rules []Rule
	edit  func(c cbfx) graph.OverlayGroup
	want  int
}

func conflictBaseCases() []cbCase {
	bound := []Rule{boundaryRule("b1", graphU, hookU)}
	protect := []Rule{{ID: "p1", Kind: "protect_path", AppliesTo: []graph.ID{graph.FileID("keep/p.lx")}}}
	hookio := fxUnit("internal/hookio")
	every := append(append(slices.Clone(bound), protect...),
		Rule{ID: "d1", Kind: "decision", Predicate: "paths_untouched", AppliesTo: []graph.ID{fxUnit("docs")}})
	return []cbCase{
		{"a new crossing import", ConflictBoundaryCross, bound, func(c cbfx) graph.OverlayGroup {
			return cbEdit("internal/graph/engine", c.fileE, cbDigest(c.fileE, "new"), []graph.ID{c.e0, c.e1}, []graph.ID{hookio})
		}, 1},
		{"a new crossing call beside one that stands", ConflictBoundaryCross, bound, func(c cbfx) graph.OverlayGroup {
			return cbEdit("internal/graph/engine", c.fileE, cbDigest(c.fileE, "new"), []graph.ID{c.e0, c.e1}, nil,
				graph.Edge{Src: c.e0, Pred: graph.Calls, Dst: c.h1})
		}, 1},
		{"a crossing call that stands, kept", "", bound, func(c cbfx) graph.OverlayGroup {
			return cbEdit("internal/graph/engine", c.fileE, cbDigest(c.fileE, "new"), []graph.ID{c.e0, c.e1}, nil)
		}, 0},
		{"a crossing import that stands, kept", "", bound, func(c cbfx) graph.OverlayGroup {
			return cbEdit("internal/graph", c.fileG, cbDigest(c.fileG, "new"), []graph.ID{c.g0}, []graph.ID{hookio})
		}, 0},
		{"a deleted symbol drops its crossing", "", bound, func(c cbfx) graph.OverlayGroup {
			return cbEdit("internal/graph/engine", c.fileE, cbDigest(c.fileE, "new"), []graph.ID{c.e0}, nil)
		}, 0},
		{"a crossing import out of the rule's From", "", bound, func(c cbfx) graph.OverlayGroup {
			return cbEdit("internal/graphx", c.fileX, cbDigest(c.fileX, "new"), []graph.ID{c.x0}, []graph.ID{hookio})
		}, 0},
		{"a protected file edited", ConflictProtectedPath, protect, func(c cbfx) graph.OverlayGroup {
			return cbEdit("keep", c.fileP, cbDigest(c.fileP, "new"), []graph.ID{c.p0}, nil)
		}, 1},
		{"a protected file restored", "", protect, func(c cbfx) graph.OverlayGroup {
			return cbEdit("keep", c.fileP, cbDigest(c.fileP, "base"), []graph.ID{c.p0}, nil)
		}, 0},
		{"a new file under a protected pattern", ConflictProtectedPath,
			[]Rule{{ID: "p2", Kind: "protect_path", Paths: []string{"keep/n.lx"}}}, func(c cbfx) graph.OverlayGroup {
				return cbEdit("keep", graph.FileID("keep/n.lx"), "sha256:n", []graph.ID{fxSym("keep", "func", "N0")}, nil)
			}, 1},
		{"a decided file edited", ConflictDecisionContradicted,
			[]Rule{{ID: "d2", Kind: "decision", Predicate: "paths_untouched", AppliesTo: []graph.ID{graph.FileID("docs/d.lx")}}},
			func(c cbfx) graph.OverlayGroup {
				return cbEdit("docs", c.fileD, cbDigest(c.fileD, "new"), []graph.ID{c.d0}, nil)
			}, 1},
		{"a decided unit edited by another file", ConflictDecisionContradicted, every, func(c cbfx) graph.OverlayGroup {
			return cbEdit("docs", c.fileD2, cbDigest(c.fileD2, "new"), []graph.ID{c.d20}, nil)
		}, 1},
		{"a decided unit's file restored", "", every, func(c cbfx) graph.OverlayGroup {
			return cbEdit("docs", c.fileD2, cbDigest(c.fileD2, "base"), []graph.ID{c.d20}, nil)
		}, 0},
		// signature_mismatch reads an arity: the Go index writes no sig and
		// a Go group carries none, so at the post it raises nothing on
		// either side. A language with arity would need its callers in the
		// base; the post checks Go edits only (keptGoEdit).
		{"a callee changed in place, no arity", ConflictSignatureMismatch, every, func(c cbfx) graph.OverlayGroup {
			return cbEdit("internal/hookio", c.fileH, cbDigest(c.fileH, "new"), []graph.ID{c.h1}, nil)
		}, 0},
		{"every rule, one new crossing", ConflictBoundaryCross, every, func(c cbfx) graph.OverlayGroup {
			return cbEdit("internal/graph/engine", c.fileE, cbDigest(c.fileE, "new"), []graph.ID{c.e0, c.e1}, []graph.ID{hookio})
		}, 1},
	}
}

// liveOnly is cs of the kinds the post keeps (verify.ConflictRowKinds,
// none withheld), as conflictJoin filters them.
func liveOnly(cs []Conflict) []Conflict {
	var out []Conflict
	for _, c := range cs {
		if k := string(c.Kind); slices.Contains(verify.ConflictRowKinds, k) && !slices.Contains(verify.WithheldConflictKinds, k) {
			out = append(out, c)
		}
	}
	return out
}

// roundTrip is b through JSON, as the base file holds it.
func roundTrip(t *testing.T, b ConflictBase) ConflictBase {
	t.Helper()
	enc, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var out ConflictBase
	if err := json.Unmarshal(enc, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// TestConflictBase_SliceEqualsFullLoad: for each case the post's check over
// the slice, read back through JSON, raises the same live conflicts as the
// same check over the whole index, and the number the case names. The
// slice holds strictly fewer records than the index, so the equality is
// not the trivial one.
func TestConflictBase_SliceEqualsFullLoad(t *testing.T) {
	for _, tc := range conflictBaseCases() {
		t.Run(tc.name, func(t *testing.T) {
			full := newConflictBaseFx(t)
			og := tc.edit(full)
			base := ""
			if n, ok := full.f.s.Node(og.File, fxC1); ok {
				base = n.Attrs["digest"]
			}
			whole := liveOnly(EditConflicts(full.f.s, tc.rules, og, base))

			src := newConflictBaseFx(t)
			all := len(src.f.s.Edges(fxC1)) + len(src.f.s.Nodes(fxC1))
			cb := roundTrip(t, ConflictBaseOf(src.f.s, fxC1, tc.rules))
			if len(cb.Records) >= all {
				t.Errorf("the slice holds %d records of the index's %d, want fewer", len(cb.Records), all)
			}
			slice := liveOnly(EditConflicts(cb.Store("C1"), tc.rules, tc.edit(src), cb.Digests[og.File]))

			if !reflect.DeepEqual(slice, whole) {
				t.Errorf("slice %s\nwhole index %s", cstr(slice), cstr(whole))
			}
			if len(whole) != tc.want {
				t.Errorf("whole index %s, want %d", cstr(whole), tc.want)
			}
			if tc.want > 0 && tc.kind != "" && (len(whole) == 0 || whole[0].Kind != tc.kind) {
				t.Errorf("whole index %s, want kind %s", cstr(whole), tc.kind)
			}
		})
	}
}

// TestConflictBase_EveryLiveKindHasACase: a kind that joins the live set
// (verify.ConflictRowKinds, none withheld) fails here until
// conflictBaseCases holds a case of it.
func TestConflictBase_EveryLiveKindHasACase(t *testing.T) {
	have := map[string]bool{}
	for _, tc := range conflictBaseCases() {
		have[string(tc.kind)] = true
	}
	for _, k := range verify.ConflictRowKinds {
		if !slices.Contains(verify.WithheldConflictKinds, k) && !have[k] {
			t.Errorf("live kind %s has no case in conflictBaseCases", k)
		}
	}
}

// cbCallAttrs and cbCallProv are what an extractor writes on the call from
// E1 into hookio: an argument count and a keyword's value as the Python
// extractor spells them, and where it read the call.
var (
	cbCallAttrs = map[string]string{"argc": "1", "kw.token": "zqxv-plinth"}
	cbCallProv  = graph.Prov{Extractor: "pylite", Version: "v1", Evidence: []string{"e1.py:3"}}
)

// TestConflictBase_KeepsEachRecordBare: every record of the slice, read
// back through JSON, has no Attrs, a zero Prov and Certain unset, while the
// index's versions of them carry attributes (a sliced symbol's name and
// kind, the call's argument count and keyword value) and the call its
// provenance: the witness. Break: append rec unbared in ConflictBaseOf.
func TestConflictBase_KeepsEachRecordBare(t *testing.T) {
	c := newConflictBaseFx(t)
	cb := roundTrip(t, ConflictBaseOf(c.f.s, fxC1, []Rule{boundaryRule("b1", graphU, hookU)}))
	var nodes, edges int
	for _, rec := range cb.Records {
		if n := rec.Node; n != nil {
			if n.Attrs != nil || !reflect.DeepEqual(n.Prov, graph.Prov{}) {
				t.Errorf("the slice keeps node %s with attrs %v and prov %+v", n.ID, n.Attrs, n.Prov)
			}
			if x, ok := c.f.s.Node(n.ID, fxC1); ok && len(x.Attrs) > 0 {
				nodes++
			}
			continue
		}
		e := rec.Edge
		if e.Attrs != nil || !reflect.DeepEqual(e.Prov, graph.Prov{}) || e.Certain {
			t.Errorf("the slice keeps edge %s %s %s with attrs %v, prov %+v, certain %v", e.Src, e.Pred, e.Dst, e.Attrs, e.Prov, e.Certain)
		}
		for _, x := range c.f.s.Out(e.Src, fxC1, e.Pred) {
			if x.Dst == e.Dst && len(x.Attrs) > 0 && len(x.Prov.Evidence) > 0 {
				edges++
			}
		}
	}
	if nodes == 0 || edges == 0 {
		t.Errorf("witness: %d sliced nodes and %d sliced edges carry attributes in the index, want at least one of each", nodes, edges)
	}
}
