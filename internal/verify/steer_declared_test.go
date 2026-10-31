package verify

import (
	"strconv"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// dgStale is SteerAt at call n's own post under sp and no index, the run
// ranIn /repo, for DG2's tests.
func dgStale(t *testing.T, r *store.Run, n int64, sp *spec.Compiled) SteerView {
	t.Helper()
	ranIn(r, "/repo", "")
	return SteerAt(r, nil, sp, dgKey, callPoint(t, r, n), snNow, snEvent, "t"+strconv.FormatInt(n, 10))
}

// TestDG2_ADeclaredGateStalesWithNoIndex (card D222): a trusted spec's
// required gate "unit", green once, then an edit, raises stale-evidence with
// no learned index at all, and the note names the gate by its id. The second
// edit raises none.
func TestDG2_ADeclaredGateStalesWithNoIndex(t *testing.T) {
	r := snRun(t, gateCall(2, "go-test", store.ExecOK), edit(3, "internal/a.go"), edit(4, "internal/b.go"))
	recorded(t, r, 2, "go test ./...")
	sp := dgSpec(spec.Trusted, dgGate("unit", "go", "test", "./..."))
	got := staleFindings(dgStale(t, r, 3, sp))
	want := "rashomon record: gate 'unit' exited 0 earlier in this session (1 min ago); this call then wrote 'internal/a.go'. " +
		"No later run of the gate is recorded."
	if len(got) != 1 || got[0].Message.Text() != want || got[0].Key != "spec:unit" {
		t.Fatalf("the first edit after the green: %+v, want one finding keyed spec:unit saying %q", got, want)
	}
	if got := staleFindings(dgStale(t, r, 4, sp)); len(got) != 0 {
		t.Errorf("the second edit: %d findings, want 0", len(got))
	}
}

// TestDG2b_ALearnedGateStillNeedsTheIndex (D164): with no index a spec that
// declares a required gate the run never matched does not make the learned
// go test family raise, and nor does a spec whose gate is advisory, or one
// that is untrusted: only a declared required gate does.
func TestDG2b_ALearnedGateStillNeedsTheIndex(t *testing.T) {
	no := false
	advisory := dgGate("unit", "go", "test", "./...")
	advisory.Gate.Required = &no
	for _, c := range []struct {
		name string
		sp   *spec.Compiled
	}{
		{"another declared gate, never run", dgSpec(spec.Trusted, dgGate("lint", "golangci-lint", "run"))},
		{"an advisory declared gate", dgSpec(spec.Trusted, advisory)},
		{"an untrusted spec", dgSpec(spec.Untrusted, dgGate("unit", "go", "test", "./..."))},
		{"no spec", nil},
	} {
		r := snRun(t, gateCall(2, "go-test", store.ExecOK), edit(3, "internal/a.go"))
		recorded(t, r, 2, "go test ./...")
		if got := staleFindings(dgStale(t, r, 3, c.sp)); len(got) != 0 {
			t.Errorf("%s: %d stale-evidence findings, want 0", c.name, len(got))
		}
	}
}

// TestDG2c_DeclaresRequired: DeclaresRequired reads the spec the way
// declaredGates does, with no key.
func TestDG2c_DeclaresRequired(t *testing.T) {
	no := false
	advisory := dgGate("unit", "go", "test")
	advisory.Gate.Required = &no
	for _, c := range []struct {
		name string
		sp   *spec.Compiled
		want bool
	}{
		{"nil", nil, false},
		{"trusted required", dgSpec(spec.Trusted, dgGate("unit", "go", "test")), true},
		{"managed required", dgSpec(spec.Managed, dgGate("unit", "go", "test")), true},
		{"untrusted required", dgSpec(spec.Untrusted, dgGate("unit", "go", "test")), false},
		{"trusted advisory", dgSpec(spec.Trusted, advisory), false},
		{"trusted none", dgSpec(spec.Trusted), false},
	} {
		if got := DeclaresRequired(c.sp); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
