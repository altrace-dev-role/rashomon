package recap

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// declaredAt is a trusted spec's gate with the id, in state s, whose program
// word is prog.
func declaredAt(id string, s verify.State, r verify.Reason, seq int64, exit *int) verify.GateState {
	g := gateAt("spec:"+id, "test", s, r, seq, exit)
	g.Source, g.Family = "declared", "declared"
	return g
}

// ran is v with g's program word set to prog.
func ran(v verify.StopView, g verify.GateState, prog string) verify.StopView {
	v.Programs[g.Gate] = prog
	return v
}

// TestSNG1_ADeclaredGateIsNamedByItsID: a gate a trusted spec declares is
// named by its id, in the slot's quote, not by the program that ran it; a
// learned gate keeps its program word; an id the slot refuses (too long, or
// outside its charset) and a ci: gate, whose id is a position, keep the
// program word too. Break: drop the declared branch and the line says go.
func TestSNG1_ADeclaredGateIsNamedByItsID(t *testing.T) {
	unit := declaredAt("unit", verify.Green, verify.OK, 3, nil)
	lint := declaredAt("lint", verify.Green, verify.OK, 4, nil)
	red := declaredAt("unit", verify.Red, verify.FailedExit, 7, exitCode(1))
	long := declaredAt(strings.Repeat("a", maxIDName+1), verify.Green, verify.OK, 3, nil)
	odd := declaredAt("Unit Test", verify.Green, verify.OK, 3, nil)
	ci := gateAt("ci:0", "test", verify.Green, verify.OK, 3, nil)
	ci.Source = "ci_commands"
	learned := gateAt("go:test", "test", verify.Green, verify.OK, 3, nil)
	for _, c := range []struct {
		name string
		v    verify.StopView
		want string
	}{
		{"verified", ran(viewOf(verify.RowVerified, wrote, unit), unit, "go"), "verified - 'unit' passed after the last edit"},
		{"two declared", ran(ran(viewOf(verify.RowVerified, wrote, unit, lint), unit, "go"), lint, "golangci-lint"),
			"verified - 'unit' and 'lint' passed after the last edit"},
		{"beside a learned gate", ran(viewOf(verify.RowVerified, wrote, unit, learned), unit, "go"),
			"verified - 'unit' and go passed after the last edit"},
		{"red", ran(viewOf(verify.RowRed, wrote, red), red, "go"), "2 files edited; 'unit' exited 1 after the last edit (call 7)"},
		{"ran only", ran(viewOf(verify.RowRanOnly, noWrite, unit), unit, "go"), "no edits; 'unit' passed (call 3)"},
		{"too long an id", ran(viewOf(verify.RowVerified, wrote, long), long, "go"), "verified - go passed after the last edit"},
		{"an id outside the charset", ran(viewOf(verify.RowVerified, wrote, odd), odd, "go"), "verified - go passed after the last edit"},
		{"a ci gate", ran(viewOf(verify.RowVerified, wrote, ci), ci, "go"), "verified - go passed after the last edit"},
		{"a learned gate", viewOf(verify.RowVerified, wrote, learned), "verified - go passed after the last edit"},
	} {
		if got := sentence(c.v, maxGateNames); got != c.want {
			t.Errorf("%s: sentence = %q, want %q", c.name, got, c.want)
		}
	}
	// The longest id the rule allows keeps the one-gate sentence on a line.
	widest := declaredAt(strings.Repeat("a", maxIDName), verify.Green, verify.OK, 3, nil)
	v := ran(viewOf(verify.RowVerified, wrote, widest), widest, "go")
	if n := columns(prefix + fitted(v, prefix) + "."); n > lineColumns {
		t.Errorf("the verified line is %d columns with the longest id, want at most %d", n, lineColumns)
	}
}
