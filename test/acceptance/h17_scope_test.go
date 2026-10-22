package acceptance

import (
	"slices"
	"testing"
)

// The fixture is a module listing in the shape moduleImports returns: each
// package's own (non-test) import statements.
var h17ScopeFixture = map[string][]string{
	rashomonModule + "/internal/hook":            {"io", rashomonModule + "/internal/store"},
	rashomonModule + "/internal/store":           {"os"},
	rashomonModule + "/test/perf/peakrss":        {"os/exec"},
	rashomonModule + "/test/perf/peakrss/reaper": {"os/exec", "syscall"},
	rashomonModule + "/test/perf/payload":        {"bytes"},
	rashomonModule + "/test/fixtures/shipped":    {"os/exec"},
	rashomonModule + "/cmd/rashomon":             {rashomonModule + "/internal/hook", rashomonModule + "/test/fixtures/shipped"},
}

// TestH17_TestPackagesStayTestOnly holds the scope H-17 and TRU-40 now take:
// the packages a shipped binary can link, which is every package outside
// test/ plus any test/ package a non-test package imports. A test/ package
// imported only from _test.go files is out of scope, and this guard is what
// stops that scope widening silently: no package outside test/ may import a
// package under test/. Break: drop the check, and the fixture's cmd/rashomon
// importing test/fixtures/shipped goes unreported.
func TestH17_TestPackagesStayTestOnly(t *testing.T) {
	t.Run("fixture", func(t *testing.T) {
		got := testOnlyViolations(h17ScopeFixture)
		want := []string{rashomonModule + "/cmd/rashomon imports " + rashomonModule + "/test/fixtures/shipped"}
		if !slices.Equal(got, want) {
			t.Fatalf("violations %q, want %q", got, want)
		}
		scope := shippedScope(h17ScopeFixture)
		for pkg, want := range map[string]bool{
			rashomonModule + "/internal/hook":            true,
			rashomonModule + "/cmd/rashomon":             true,
			rashomonModule + "/test/fixtures/shipped":    true, // a non-test package imports it
			rashomonModule + "/test/perf/peakrss":        false,
			rashomonModule + "/test/perf/peakrss/reaper": false,
			rashomonModule + "/test/perf/payload":        false,
		} {
			if scope[pkg] != want {
				t.Errorf("in scope(%s) = %v, want %v", pkg, scope[pkg], want)
			}
		}
	})
	t.Run("module", func(t *testing.T) {
		if v := testOnlyViolations(moduleImports(t)); len(v) > 0 {
			t.Errorf("a package a shipped binary can link imports a test/ package, which puts test code inside H-17's scope: %q", v)
		}
	})
}
