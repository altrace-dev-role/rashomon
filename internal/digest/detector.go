package digest

import (
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Fired reports whether the detector id fires on this turn's digest. It is
// the one place each of the eight digest-level detectors' fire conditions is
// written: the end-of-turn line (internal/recap) asks it before it renders a
// sentence, and the precision harness asks it to learn which detector fired,
// so neither reads a printed sentence to find out and a rewording cannot
// change the answer.
//
// An id this digest does not carry (the two report-only detectors, see
// Destinations.Fired in internal/report) reports false.
func (d *Digest) Fired(id report.DetectorID) bool {
	switch id {
	case report.DetectorDigestUnknown:
		return d.Unknown
	case report.DetectorCoverageUnverified:
		// Unknown already says the coverage reasons: one sentence, not two.
		return !d.Unknown && d.Coverage.State != store.StateVerified
	case report.DetectorDigestTruncated:
		return d.Truncated
	case report.DetectorSilentFailure:
		return d.SilentFailures.Fires
	case report.DetectorMaskedRun:
		return d.MaskedRuns.Fires
	case report.DetectorTestsOnlyThenGreen:
		return len(d.TestBending.TestsOnlyThenGreen)+d.TestBending.TestsOnlyThenGreenOmitted > 0
	case report.DetectorFlakyPair:
		return len(d.TestBending.Flaky)+d.TestBending.FlakyOmitted > 0
	case report.DetectorDeclarationWithoutExecution:
		return len(d.Declarations.WithoutExecution) > 0
	}
	return false
}
