package report

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// unreadableCoverageCases are coverage records this build cannot take at
// their word: unverified with no reason to say why, or a state that is
// neither of the two this program writes. docs/store-schema.json accepted
// the first, and a newer build or a hand edit can produce the second.
var unreadableCoverageCases = []struct {
	name   string
	state  string
	reason *string
}{
	{"unverified with a null reason", store.StateUnverified, nil},
	{"an unknown state", "degraded", nil},
	{"an unknown state with a reason", "degraded", ptr(store.ReasonInternalError)},
	{"an empty state", "", nil},
}

// TestRubricERRORS5_RollupFailsClosedOnACoverageRecordItCannotRead: each of
// those records adds records_unreadable, so a run carrying one cannot read
// verified. Before, RollupCoverage collected a reason only for
// state=unverified with a non-nil reason, and all four left the run verified.
func TestRubricERRORS5_RollupFailsClosedOnACoverageRecordItCannotRead(t *testing.T) {
	for _, c := range unreadableCoverageCases {
		t.Run(c.name, func(t *testing.T) {
			covs := []store.Coverage{
				{Phase: store.PhaseStart, State: store.StateVerified},
				{Phase: store.PhaseCall, State: c.state, Reason: c.reason},
				{Phase: store.PhaseEnd, State: store.StateVerified},
			}
			facts := RollupCoverage(covs)
			if !slices.Contains(facts.Reasons, ReasonRecordsUnreadable) {
				t.Errorf("reasons = %v, want %s for a coverage record with state %q and reason %v",
					facts.Reasons, ReasonRecordsUnreadable, c.state, c.reason)
			}

			sess := build(&store.Run{Coverage: covs})
			sess.Coverage.finish()
			if sess.Coverage.State != store.StateUnverified {
				t.Errorf("the report renders %s (%v), want unverified", sess.Coverage.State, sess.Coverage.Reasons)
			}
		})
	}
}

// TestRubricERRORS5_ReadableCoverageAddsNothing is the control: verified
// records, and an unverified one that names its reason, add exactly what
// they say and nothing else.
func TestRubricERRORS5_ReadableCoverageAddsNothing(t *testing.T) {
	covs := []store.Coverage{
		{Phase: store.PhaseStart, State: store.StateVerified},
		{Phase: store.PhaseCall, State: store.StateUnverified, Reason: ptr(store.ReasonLockTimeout)},
		{Phase: store.PhaseEnd, State: store.StateVerified},
	}
	if got := RollupCoverage(covs).Reasons; !slices.Equal(got, []string{store.ReasonLockTimeout}) {
		t.Errorf("reasons = %v, want [%s]", got, store.ReasonLockTimeout)
	}
	if got := RollupCoverage(covs[:1]).Reasons; len(got) != 0 {
		t.Errorf("a verified record added %v", got)
	}
}
