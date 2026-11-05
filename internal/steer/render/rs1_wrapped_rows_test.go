package render

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TestRS1_WrappedRowsSayTheRunRanInsideAnotherCommand: the six rows of a
// run read from the output of a command that ran the gate inside it
// (WRAPPER-RED-STEP1) render under the whole-string check, carry their own
// template ids and slots in the binding's order, say where the run was read,
// and name no pipe; the wrapped green's rows never say it exited 0. The
// texts are pinned in verify (TestRS1_TheRowSaysWhereTheRunWasRead). Break:
// reuse the pipe's row text, or drop a row.
func TestRS1_WrappedRowsSayTheRunRanInsideAnotherCommand(t *testing.T) {
	paths := slot.NewPathList([]slot.Path{"internal/a.go"}, 0)
	for _, c := range []struct {
		pattern pattern.ID
		binding any
		id      string
		slots   []string
	}{
		{pattern.RegressionLocation, RegressionLocationWrappedBinding{Gate: "go", Minutes: 6, Paths: paths},
			"regression-location-wrapped/v1", []string{"gate", "minutes", "paths"}},
		{pattern.RegressionLocation, RegressionLocationWrappedDegradedBinding{Gate: "go", Minutes: 6, Count: 2},
			"regression-location-wrapped-degraded/v1", []string{"gate", "minutes", "count"}},
		{pattern.RegressionLocation, RegressionLocationWrappedGreenBinding{Gate: "go", Minutes: 6, Exit: 1, Paths: paths},
			"regression-location-wrapped-green/v1", []string{"gate", "minutes", "exit", "paths"}},
		{pattern.RegressionLocation, RegressionLocationWrappedGreenDegradedBinding{Gate: "go", Minutes: 6, Exit: 1, Count: 2},
			"regression-location-wrapped-green-degraded/v1", []string{"gate", "minutes", "exit", "count"}},
		{pattern.StaleEvidence, StaleEvidenceWrappedBinding{Gate: "go", Minutes: 1, Paths: paths},
			"stale-evidence-wrapped/v1", []string{"gate", "minutes", "paths"}},
		{pattern.StaleEvidence, StaleEvidenceWrappedDegradedBinding{Gate: "go", Minutes: 1, Count: 2},
			"stale-evidence-wrapped-degraded/v1", []string{"gate", "minutes", "count"}},
	} {
		m, err := New(c.pattern, c.binding)
		if err != nil {
			t.Errorf("%s: %v", c.id, err)
			continue
		}
		text := m.Text()
		if m.Template() != c.id || !strings.Contains(text, "ran inside another command") || strings.Contains(text, "pipe") || strings.Contains(text, "exited 0") {
			t.Errorf("%s: template %q text %q", c.id, m.Template(), text)
		}
		var names []string
		for _, s := range m.Slots() {
			names = append(names, s.Name)
		}
		if !slices.Equal(names, c.slots) {
			t.Errorf("%s: slots %v, want %v", c.id, names, c.slots)
		}
	}
}
