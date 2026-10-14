package render_test

import (
	"errors"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
)

// TestDEL22e_MessageRidesOnlyItsOwnPattern: a rendered Message is checked
// against the pattern whose template made it (render.New chooses the
// briefing check by that pattern), so a Finding of another pattern cannot
// carry it onto a channel its check was not run for (P07N-14).
func TestDEL22e_MessageRidesOnlyItsOwnPattern(t *testing.T) {
	m, err := render.SampleMessage()
	if err != nil {
		t.Fatal(err)
	}
	f := steer.Finding{Pattern: pattern.RegressionLocation, MaxLevel: steer.LevelNudge, StopForm: steer.StopFormNone, Message: m, Evidence: []int64{37, 52}, AfterSeq: 57}
	if err := f.Check(); err != nil {
		t.Fatalf("the message's own pattern: %v", err)
	}
	f.Pattern = pattern.SubtaskGate
	if err := f.Check(); !errors.Is(err, steer.ErrFinding) {
		t.Errorf("a regression-location message on a subtask-gate finding: %v, want ErrFinding", err)
	}
}
