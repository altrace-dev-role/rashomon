package engine

import (
	"reflect"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// TestConflicts_ProtectedPathFiresOnNewFileUnderPattern: a protect_path
// rule's Paths fire for a file the diff names that the index does not hold
// (a planned new file), by spec.MatchPath on the project-relative path. One
// conflict per file even when AppliesTo and Paths both match it. Control: a
// file outside the pattern gives none.
func TestConflicts_ProtectedPathFiresOnNewFileUnderPattern(t *testing.T) {
	f := newFx()
	held := f.file("secrets", "secrets/keys/old.lx", fxC1)
	fresh := graph.FileID("secrets/keys/new.lx")
	outside := graph.FileID("src/new.lx")
	rule := Rule{ID: "p1", Kind: "protect_path", Paths: []string{"secrets/keys"}}

	d := Diff{Rev: fxC1, Files: []graph.ID{fresh}, Touched: []graph.ID{fxUnit("secrets")}}
	got := onlyKind(Conflicts(f.s, d, []Rule{rule}), ConflictProtectedPath)
	want := []Conflict{{Kind: ConflictProtectedPath, Rev: fxC1, RuleID: "p1", Subject: fresh, Evidence: []graph.ID{fresh}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("a new file under the pattern: %s\nwant %s", cstr(got), cstr(want))
	}
	if got := onlyKind(Conflicts(f.s, Diff{Rev: fxC1, Files: []graph.ID{outside}}, []Rule{rule}), ConflictProtectedPath); len(got) != 0 {
		t.Errorf("a file outside the pattern: %s, want none", cstr(got))
	}
	both := Rule{ID: "p2", Kind: "protect_path", Paths: []string{"secrets/keys"}, AppliesTo: []graph.ID{held}}
	got = onlyKind(Conflicts(f.s, Diff{Rev: fxC1, Files: []graph.ID{held}}, []Rule{both}), ConflictProtectedPath)
	if len(got) != 1 {
		t.Errorf("AppliesTo and Paths both match: %s, want one", cstr(got))
	}
}

// TestConflicts_PathsUntouchedFiresOnNewFileUnderPattern is the same for a
// decision rule with the paths_untouched predicate. Control: a file outside
// the pattern gives none.
func TestConflicts_PathsUntouchedFiresOnNewFileUnderPattern(t *testing.T) {
	f := newFx()
	held := f.file("frozen", "frozen/old.lx", fxC1)
	fresh := graph.FileID("frozen/new.lx")
	outside := graph.FileID("live/new.lx")
	rule := Rule{ID: "d1", Kind: "decision", Predicate: "paths_untouched", Paths: []string{"frozen/"}}

	got := onlyKind(Conflicts(f.s, Diff{Rev: fxC1, Files: []graph.ID{fresh}}, []Rule{rule}), ConflictDecisionContradicted)
	want := []Conflict{{Kind: ConflictDecisionContradicted, Rev: fxC1, RuleID: "d1", Subject: fresh, Evidence: []graph.ID{fresh}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("a new file under the pattern: %s\nwant %s", cstr(got), cstr(want))
	}
	if got := onlyKind(Conflicts(f.s, Diff{Rev: fxC1, Files: []graph.ID{outside}}, []Rule{rule}), ConflictDecisionContradicted); len(got) != 0 {
		t.Errorf("a file outside the pattern: %s, want none", cstr(got))
	}
	both := Rule{ID: "d2", Kind: "decision", Predicate: "paths_untouched", Paths: []string{"frozen"}, AppliesTo: []graph.ID{held}}
	got = onlyKind(Conflicts(f.s, Diff{Rev: fxC1, Files: []graph.ID{held}}, []Rule{both}), ConflictDecisionContradicted)
	if len(got) != 1 {
		t.Errorf("AppliesTo and Paths both match: %s, want one", cstr(got))
	}
}
