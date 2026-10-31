package render

import (
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// TestMO1_ProvenanceRowsSayTheOutcomeWasReadFromOutput: the four rows of a
// run whose outcome the pipe hid and the output gave (D229) render under the
// whole-string check, name the provenance in their text, say nothing of an
// exit code, and carry their own template ids and slots in the binding's
// order. Break: render the exit row for an output-derived run, and the text
// says "exited 1" of a pipe's status.
func TestMO1_ProvenanceRowsSayTheOutcomeWasReadFromOutput(t *testing.T) {
	paths := slot.NewPathList([]slot.Path{"internal/a.py"}, 0)
	for _, c := range []struct {
		name    string
		pattern pattern.ID
		binding any
		id      string
		slots   []string
		want    string
	}{
		{"regression with paths", pattern.RegressionLocation,
			RegressionLocationOutputBinding{Gate: "pytest", Minutes: 6, Paths: paths},
			"regression-location-output/v1", []string{"gate", "minutes", "paths"},
			"rashomon record: gate 'pytest' passed earlier in this session (6 min ago) and its output reports a failure on this run " +
				"(read from its output; the pipe hid the exit status). Files written between the two runs: 'internal/a.py'."},
		{"regression degraded", pattern.RegressionLocation,
			RegressionLocationOutputDegradedBinding{Gate: "pytest", Minutes: 6, Count: 2},
			"regression-location-output-degraded/v1", []string{"gate", "minutes", "count"},
			"rashomon record: gate 'pytest' passed earlier in this session (6 min ago) and its output reports a failure on this run " +
				"(read from its output; the pipe hid the exit status). 2 file writes were recorded between the two runs."},
		{"stale with paths", pattern.StaleEvidence,
			StaleEvidenceOutputBinding{Gate: "pytest", Minutes: 1, Paths: paths},
			"stale-evidence-output/v1", []string{"gate", "minutes", "paths"},
			"rashomon record: gate 'pytest' passed earlier in this session (1 min ago), read from its output as the pipe hid the exit status; " +
				"this call then wrote 'internal/a.py'. No later run of the gate is recorded."},
		{"stale degraded", pattern.StaleEvidence,
			StaleEvidenceOutputDegradedBinding{Gate: "pytest", Minutes: 1, Count: 2},
			"stale-evidence-output-degraded/v1", []string{"gate", "minutes", "count"},
			"rashomon record: gate 'pytest' passed earlier in this session (1 min ago), read from its output as the pipe hid the exit status; " +
				"this call then wrote 2 files. No later run of the gate is recorded."},
	} {
		m, err := New(c.pattern, c.binding)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if m.Text() != c.want || m.Template() != c.id {
			t.Errorf("%s: text %q template %q, want %q %q", c.name, m.Text(), m.Template(), c.want, c.id)
		}
		var names []string
		for _, s := range m.Slots() {
			names = append(names, s.Name)
		}
		if !slices.Equal(names, c.slots) {
			t.Errorf("%s: slots %q, want %q", c.name, names, c.slots)
		}
		if !strings.Contains(m.Text(), "from its output") || strings.Contains(m.Text(), "exited") {
			t.Errorf("%s: text %q must say it was read from the output and name no exit", c.name, m.Text())
		}
	}
}
