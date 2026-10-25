package recap

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestLineSpeaksExactlyWhenADetectorFired: the end-of-turn line and the
// detector ids are one decision. For each digest-level detector, a digest that
// makes only that one fire has Fired true for that id and no other, and Line
// speaks; a clean digest fires none and Line is silent. The precision harness
// learns which detector fired from Fired and never from the sentence, so this
// is what keeps the two from drifting apart without any wording being read.
func TestLineSpeaksExactlyWhenADetectorFired(t *testing.T) {
	digestLevel := map[report.DetectorID]func(*digest.Digest){
		report.DetectorDigestUnknown: func(d *digest.Digest) {
			d.Unknown, d.Coverage.State, d.Coverage.Reasons = true, store.StateUnverified, []string{store.ReasonProbeAbsent}
		},
		report.DetectorCoverageUnverified: func(d *digest.Digest) {
			d.Coverage.State, d.Coverage.Reasons = store.StateUnverified, []string{store.ReasonProbeAbsent}
		},
		report.DetectorDigestTruncated: func(d *digest.Digest) { d.Truncated = true },
		report.DetectorSilentFailure: func(d *digest.Digest) {
			d.SilentFailures.Fires, d.SilentFailures.Failed = true, 1
		},
		report.DetectorMaskedRun:          func(d *digest.Digest) { d.MaskedRuns.Fires, d.MaskedRuns.Runs = true, 1 },
		report.DetectorTestsOnlyThenGreen: func(d *digest.Digest) { d.TestBending.TestsOnlyThenGreenOmitted = 1 },
		report.DetectorFlakyPair:          func(d *digest.Digest) { d.TestBending.FlakyOmitted = 1 },
		report.DetectorDeclarationWithoutExecution: func(d *digest.Digest) {
			d.Declarations.WithoutExecution = []report.Unexecuted{{}}
		},
	}
	clean := cleanDigest()
	for _, id := range report.DetectorIDs() {
		if clean.Fired(id) {
			t.Errorf("clean digest fires %s", id)
		}
	}
	if line, ok := Line(clean, clean.SessionID, true); ok {
		t.Errorf("clean digest spoke: %q", line)
	}
	for id, set := range digestLevel {
		d := cleanDigest()
		set(d)
		for _, other := range report.DetectorIDs() {
			if got, want := d.Fired(other), other == id; got != want {
				t.Errorf("setting %s: Fired(%s) = %v, want %v", id, other, got, want)
			}
		}
		if line, ok := Line(d, d.SessionID, true); !ok || line == "" {
			t.Errorf("%s fired and the line is silent", id)
		}
	}
}
