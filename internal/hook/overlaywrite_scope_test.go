package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestKeptGoEdit_ContentKeptOnlyAtScopeV5: the overlay keeps an edit's
// content only where the record scope is v5, whatever the session-content
// marker says. The gate checks the scope first, so below v5 an Edit naming a
// Go file, in a session whose marker is set, stops before the marker read:
// nothing kept and no allocation. The controls: at v5 with the marker set the
// same call is kept, and at v5 with no marker the marker read allocates, so
// the measure can see one. Through both writers, with the marker set: a store
// whose scope marker is absent (read as v4; a failed scope-marker write in
// consent.Append leaves it so) or names v4 gets no log and no group of either
// kind, and v5 gets the declared group and the landed one. Break: drop the
// scope check, and the gate row and the absent and v4 writer rows keep the
// edit; pass v5 at either writer, and that writer's group appears below v5.
func TestKeptGoEdit_ContentKeptOnlyAtScopeV5(t *testing.T) {
	t.Run("gate", func(t *testing.T) {
		f := newOvFix(t, "1\n")
		in, err := json.Marshal(f.edit("func Old() {}", "func Old() {}\n\nfunc Added() {}"))
		if err != nil {
			t.Fatal(err)
		}
		gate := func(v5 bool) bool {
			_, g := keptGoEdit(f.st, ovSess, "Edit", in, nil, f.repo, 1, v5)
			return g.ok
		}
		cost := func(v5 bool) float64 { return testing.AllocsPerRun(10, func() { gate(v5) }) }
		if gate(false) {
			t.Error("below v5, marker set: the gate kept the edit; want nothing kept")
		}
		if n := cost(false); n != 0 {
			t.Errorf("below v5, marker set: the gate costs %.0f allocs; want 0 (the scope is checked first)", n)
		}
		if !gate(true) {
			t.Error("control: at v5, marker set, the gate kept nothing; want the edit kept")
		}
		if err := os.Remove(filepath.Join(f.st.Root(), store.FileSessionContent)); err != nil {
			t.Fatal(err)
		}
		if n := cost(true); n == 0 {
			t.Error("control: at v5 with no marker the marker read costs 0 allocs, so the measure cannot see one")
		}
	})
	for _, tc := range []struct {
		name, scope string
		kept        bool
	}{
		{"writer/absent", "", false},
		{"writer/v4", store.RecordScopeV4, false},
		{"writer/v5", store.RecordScopeV5, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOvFix(t, "1\n")
			var err error
			if tc.scope == "" {
				err = store.RemoveRecordScopeMarker(f.st.Root())
			} else {
				err = store.WriteRecordScopeMarker(f.st.Root(), tc.scope)
			}
			if err != nil {
				t.Fatal(err)
			}
			in := f.edit("func Old() {}", "func Old() {}\n\nfunc Added() {}")
			f.pre("toolu_kv5", "Edit", in)
			f.post("toolu_kv5", "Edit", in)
			groups, _, exists := f.read()
			kinds := map[string]int{}
			for _, g := range groups {
				kinds[g.Kind]++
			}
			if !tc.kept {
				if exists || len(groups) != 0 {
					t.Errorf("scope %q: the log exists %v with groups %v; want no log and no group", tc.scope, exists, kinds)
				}
				return
			}
			if len(groups) != 2 || kinds[graph.OverlayDeclared] != 1 || kinds[graph.OverlayLanded] != 1 {
				t.Errorf("at v5: groups %v; want one declared and one landed", kinds)
			}
		})
	}
}
