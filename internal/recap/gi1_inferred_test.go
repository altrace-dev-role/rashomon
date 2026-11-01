package recap

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// inferredAt is the inferred gate's row (card GATE-INFER1) in state s, whose
// program is the runner's row prog.
func inferredAt(s verify.State, r verify.Reason, prog string) (verify.GateState, string) {
	g := gateAt(verify.InferredGateID, "test", s, r, 3, nil)
	g.Source, g.Trust, g.Family = verify.SourceInferred, verify.SourceInferred, "declared"
	if s == verify.Absent {
		g.LastRun, g.LastGreen = nil, nil
	}
	return g, prog
}

// TestGI1r_TheStopNamesTheInferredCommandByItsRunner: an edit and no run of
// the inferred command reads "<files>, go test (inferred) not run after the
// last edit" within 80 columns (W4: the runner's row, a record word, never
// the CI line), the label dropped only where it does not fit; a green run
// says it too. Control: a learned gate in the same states keeps today's
// sentences. Break: name the gate by its kind, drop the label, or keep the
// no-run sentence that names no gate.
func TestGI1r_TheStopNamesTheInferredCommandByItsRunner(t *testing.T) {
	absent, prog := inferredAt(verify.Absent, verify.NeverRunAfterWrite, "go test")
	green, _ := inferredAt(verify.Green, verify.OK, "go test")
	learned := gateAt("go:test", "test", verify.Absent, verify.NeverRunAfterWrite, 3, nil)
	learned.LastRun = nil
	for _, c := range []struct {
		name string
		v    verify.StopView
		want string
	}{
		{"absent", ran(viewOf(verify.RowUnverified, wrote, absent), absent, prog), "2 files edited, go test (inferred) not run after the last edit"},
		{"green", ran(viewOf(verify.RowVerified, wrote, green), green, prog), "verified - go test (inferred) passed after the last edit"},
		{"control: a learned gate", viewOf(verify.RowUnverified, wrote, learned), "2 files edited, no test or build run after the last edit"},
	} {
		if got := sentence(c.v, maxGateNames, false); got != c.want {
			t.Errorf("%s: sentence = %q, want %q", c.name, got, c.want)
		}
	}
	v := ran(viewOf(verify.RowUnverified, verify.TurnCounts{Writes: 12, PathsWritten: 12, LastWriteSeq: 5}, absent), absent, "python3 -m pytest")
	line := ValueLine(v, Impact{}, "sess-1", false)
	first, _, _ := strings.Cut(line, "\n")
	if n := columns(first); n > lineColumns || !strings.Contains(first, "python3 -m pytest") || !strings.Contains(first, "not run after the last edit") {
		t.Errorf("first line %q (%d columns), want the inferred runner named within %d", first, n, lineColumns)
	}
	v = ran(viewOf(verify.RowUnverified, verify.TurnCounts{Writes: 3, PathsWritten: 3, LastWriteSeq: 5}, absent), absent, prog)
	if first, _, _ = strings.Cut(ValueLine(v, Impact{}, "sess-1", false), "\n"); columns(first) > lineColumns ||
		!strings.Contains(first, "go test (inferred) not run after the last edit") {
		t.Errorf("first line %q, want the label kept where it fits", first)
	}
}
