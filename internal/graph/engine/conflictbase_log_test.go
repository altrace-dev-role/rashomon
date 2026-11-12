package engine

import (
	"bytes"
	"maps"
	"os"
	"reflect"
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// slWith is og as the hook holds it in memory: each record at og's overlay
// revision with the Go overlay's Prov, and each symbol with a sig beside its
// other attributes.
func slWith(og graph.OverlayGroup) graph.OverlayGroup {
	rev := graph.RevOverlay(og.Commit, og.Seq)
	prov := graph.Prov{Extractor: "go/parser", Version: "v1"}
	recs := make([]graph.Record, len(og.Recs))
	for i, r := range og.Recs {
		if r.Node != nil {
			n := *r.Node
			n.Rev, n.Prov = rev, prov
			if n.Kind == "sym" {
				n.Attrs = maps.Clone(n.Attrs)
				if n.Attrs == nil {
					n.Attrs = map[string]string{}
				}
				n.Attrs["sig"] = "func ( slw int ) error"
			}
			recs[i].Node = &n
		}
		if r.Edge != nil {
			e := *r.Edge
			e.Rev, e.Prov = rev, prov
			recs[i].Edge = &e
		}
	}
	og.Recs = recs
	return og
}

// TestSessionLog_LoadedEditGivesTheLiveConflicts: for each case of
// conflictBaseCases, the edit as the hook holds it (slWith: a Prov on every
// record, a sig on every symbol) and the same edit read back from a session
// log, which keeps neither (graph.LogRecords), raise the same live
// conflicts, the number the case names: equal in every field but the Prov
// of the edge a conflict copies, which the log does not keep and no reader
// of a conflict reads (Key is the edge's ids). Witness: the edit in memory
// carries a sig and a Prov, the loaded one neither, and a live conflict's
// edge carries the hook's Prov where the loaded one's has none.
func TestSessionLog_LoadedEditGivesTheLiveConflicts(t *testing.T) {
	witnessed := 0
	for _, tc := range conflictBaseCases() {
		t.Run(tc.name, func(t *testing.T) {
			mem := newConflictBaseFx(t)
			with := slWith(tc.edit(mem))
			l := graph.OpenSessionLog(t.TempDir(), "rid", graph.RevCommit("C1"), true)
			if err := l.AppendOverlayGroup(graph.BuildOverlayGroup(with), nil); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(l.Path)
			if err != nil {
				t.Fatal(err)
			}
			_, skipped, groups, err := graph.LoadSessionLogGroups(graph.NewStore(), bytes.NewReader(b))
			if err != nil || skipped != 0 || len(groups) != 1 {
				t.Fatalf("load: %d groups, %d skipped, err %v; want the one group", len(groups), skipped, err)
			}
			loaded := groups[0]
			var sigs, provs int
			for _, r := range with.Recs {
				if r.Node != nil && r.Node.Attrs["sig"] != "" {
					sigs++
				}
				if r.Node != nil && r.Node.Prov.Extractor != "" || r.Edge != nil && r.Edge.Prov.Extractor != "" {
					provs++
				}
			}
			if sigs == 0 || provs != len(with.Recs) {
				t.Errorf("witness: the edit in memory has %d sigs and %d of %d records with a Prov", sigs, provs, len(with.Recs))
			}
			if len(loaded.Recs) != len(with.Recs) {
				t.Errorf("the loaded edit has %d records, want %d", len(loaded.Recs), len(with.Recs))
			}
			for _, r := range loaded.Recs {
				if r.Node != nil && (r.Node.Attrs["sig"] != "" || !reflect.DeepEqual(r.Node.Prov, graph.Prov{})) {
					t.Errorf("the loaded node %s keeps attrs %v and prov %+v", r.Node.ID, r.Node.Attrs, r.Node.Prov)
				}
				if r.Edge != nil && !reflect.DeepEqual(r.Edge.Prov, graph.Prov{}) {
					t.Errorf("the loaded edge %s %s %s keeps prov %+v", r.Edge.Src, r.Edge.Pred, r.Edge.Dst, r.Edge.Prov)
				}
			}

			base := ""
			if n, ok := mem.f.s.Node(with.File, fxC1); ok {
				base = n.Attrs["digest"]
			}
			live := liveOnly(EditConflicts(mem.f.s, tc.rules, with, base))
			got := liveOnly(EditConflicts(newConflictBaseFx(t).f.s, tc.rules, loaded, base))
			if !reflect.DeepEqual(sansEdgeProv(got), sansEdgeProv(live)) {
				t.Errorf("loaded %s\nin memory %s", cstr(got), cstr(live))
			}
			for i := range live {
				if live[i].Edge.Src == "" || i >= len(got) {
					continue
				}
				witnessed++
				if reflect.DeepEqual(live[i].Edge.Prov, graph.Prov{}) || !reflect.DeepEqual(got[i].Edge.Prov, graph.Prov{}) {
					t.Errorf("witness: conflict %d's edge Prov in memory %+v, loaded %+v; want the hook's and none", i, live[i].Edge.Prov, got[i].Edge.Prov)
				}
			}
			if len(live) != tc.want {
				t.Errorf("in memory %s, want %d", cstr(live), tc.want)
			}
		})
	}
	if witnessed == 0 {
		t.Error("witness: no case's live conflict copies an edge")
	}
}

// sansEdgeProv is cs with the Prov of each conflict's edge zeroed: the one
// field a conflict from a loaded edit differs in, since the log keeps no
// Prov, and one no reader of a conflict reads.
func sansEdgeProv(cs []Conflict) []Conflict {
	out := slices.Clone(cs)
	for i := range out {
		out[i].Edge.Prov = graph.Prov{}
	}
	return out
}
