package acceptance

import (
	"slices"
	"testing"
)

// TestP6_HookAndStoreImportNoLearner: the learner is its own component with
// its own store (PRD PF-HD16): no package the hook or the store links reaches
// internal/learn, so a lesson can change no recorded byte and cost the record
// path (hook, post, probe) no time. Only recap pending reads it, at a prompt
// (LEARN-DELIVER1). The learner links no network package (TRU-40, owner rule
// 10-23).
func TestP6_HookAndStoreImportNoLearner(t *testing.T) {
	const learner = "github.com/altrace-dev-role/rashomon/internal/learn"
	for _, pkg := range []string{"./internal/hook", "./internal/store"} {
		pkgs := deps(t, pkg)
		if !slices.Contains(pkgs, "github.com/altrace-dev-role/rashomon/internal/store") {
			t.Fatalf("the dependency listing of %s (%d packages) lacks internal/store; the command did not run as expected", pkg, len(pkgs))
		}
		if slices.Contains(pkgs, learner) {
			t.Errorf("%s depends on internal/learn", pkg)
		}
	}
	pkgs := deps(t, "./internal/learn")
	if !slices.Contains(pkgs, learner) {
		t.Fatalf("the dependency listing of internal/learn (%d packages) lacks itself", len(pkgs))
	}
	for _, pkg := range pkgs {
		if why, bad := forbiddenImports[pkg]; bad {
			t.Errorf("internal/learn depends on %s, which %s", pkg, why)
		}
	}
}
