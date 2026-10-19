package report

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestREC55c_ASupervisorRecordIsNotAProbeRun: the supervisor writes a start
// or end coverage record for a probe whose child failed or never started.
// That record says the probe did not finish, so -- like a paused probe's --
// it must not count as the session's start or end having been recorded, and
// the hook entry it carries was never resolved. Its reason still reaches the
// rollup, so the session reads unverified with the reason named.
func TestREC55c_ASupervisorRecordIsNotAProbeRun(t *testing.T) {
	for _, reason := range []string{store.ReasonHookKilled, store.ReasonSupervisorSpawnFailed} {
		t.Run(reason, func(t *testing.T) {
			r := reason
			covs := []store.Coverage{
				{Type: store.TypeCoverage, Phase: store.PhaseStart, State: store.StateUnverified, Reason: &r, HookEntry: store.EntryUnknown},
				{Type: store.TypeCoverage, Phase: store.PhaseEnd, State: store.StateUnverified, Reason: &r, HookEntry: store.EntryUnknown},
			}
			f := RollupCoverage(covs)
			if f.StartRecorded || f.EndRecorded {
				t.Errorf("a %s record counted as a recorded probe: start %v, end %v", reason, f.StartRecorded, f.EndRecorded)
			}
			if len(f.Reasons) != 1 || f.Reasons[0] != reason {
				t.Errorf("reasons %v, want [%s]", f.Reasons, reason)
			}
		})
	}
}
