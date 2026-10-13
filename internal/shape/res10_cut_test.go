package shape

import (
	"strings"
	"testing"
)

// TestRES10_DeriveSaysTheCdFoldWasCut: past MaxLeadingCd leading `cd DIR &&`
// steps the hook records the start directory as unknown (RES-10), so a path
// the line names relative to it is not known either, and the shape says the
// line was cut. MaxLeadingCd steps are folded and the line is not cut.
func TestRES10_DeriveSaysTheCdFoldWasCut(t *testing.T) {
	line := func(n int) string { return strings.Repeat("cd a && ", n) + "touch f" }
	if _, cut := DeriveCut("Bash", bashInput(t, line(MaxLeadingCd)), benchKey); cut {
		t.Errorf("%d steps: cut, want the fold to read them all", MaxLeadingCd)
	}
	if _, cut := DeriveCut("Bash", bashInput(t, line(MaxLeadingCd+1)), benchKey); !cut {
		t.Errorf("%d steps: not cut, want cut past the fold", MaxLeadingCd+1)
	}
}
