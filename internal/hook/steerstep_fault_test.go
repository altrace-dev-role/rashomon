//go:build rashomonfault

package hook

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/fault"
)

// TestST_FlushFailureBeforeThePrint: the steer line is durable before its
// note prints (REC-34): when the step's Flush fails, the line Route appended
// stays in the log and nothing is printed (card 2.3 R4.7, A11). The count is
// large so a repeated run (-count) still selects the failure. The control,
// with no fault, prints the flag.
func TestST_FlushFailureBeforeThePrint(t *testing.T) {
	t.Run("control: no fault", func(t *testing.T) {
		f := newST(t)
		if b := f.red("toolu_r").Steer(stEnv(0)); !printsFlag(t, b) {
			t.Fatalf("the control printed %q, want the consent flag", b)
		}
	})
	t.Run("the step's flush fails", func(t *testing.T) {
		f := newST(t)
		p := f.red("toolu_r")
		t.Setenv(fault.EnvVar, fault.PointSteerFlush+":"+fault.FailN+"=1000000")
		if b := p.Steer(stEnv(0)); b != nil {
			t.Errorf("Steer with a failed flush = %q, want nil", b)
		}
		if lines := steerLines(t, f.st); len(lines) != 1 {
			t.Errorf("%d steer lines, want the one Route appended before the flush", len(lines))
		}
	})
}
