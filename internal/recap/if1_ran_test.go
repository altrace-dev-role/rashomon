package recap

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestIF1_AGateWithNoRunNeverRan (card INFER-FALSE1, B303, QA-2's FHV on
// ea6f100c, turn 1): the agent ran `python3 -m pytest <3 files>`, edited
// nothing and claimed done; the inferred pytest gate bound no run, so its row
// is absent, empty_run, with no last run, and the Stop said "pytest ran in an
// earlier turn and ran no checks (call 0)". A gate with no run in the record
// never prints "ran": the value line and the done flag go silent rather than
// say it, and the done flag does not fall back to the no-gate rows' "no test
// or build run is recorded", which a run no gate bound makes false too.
// Control: the same row with a run keeps its sentence, as value_test's
// "empty run in an earlier turn" does.
func TestIF1_AGateWithNoRunNeverRan(t *testing.T) {
	turn1 := verify.TurnCounts{GateRuns: 1, FirstSeq: 1}
	absent := gateAt("pytest:test", "test", verify.Absent, verify.EmptyRun, 0, nil)
	absent.LastRun = nil
	v := viewOf(verify.RowUnverified, turn1, absent)
	v.Scan = verify.ClaimScan{Claims: []verify.ClaimVerdict{{Class: verify.ClaimDone, Strict: true, Verdict: verify.Verdict(verify.Absent)}}}
	if got := ValueLine(v, Impact{}, "sess-1", false); got != "" {
		t.Errorf("value line %q, want nothing: the gate has no run to speak of", got)
	}
	if got, ok := DoneFlag(v, "sess-1", false); ok || strings.Contains(got, " ran") || strings.Contains(got, "no test or build run") {
		t.Errorf("done flag %q, %v, want nothing: no line may say the gate ran, or that nothing ran", got, ok)
	}

	ran := viewOf(verify.RowUnverified, verify.TurnCounts{GateRuns: 1, FirstSeq: 5}, gateAt("pytest:test", "test", verify.Absent, verify.EmptyRun, 2, nil))
	ran.Scan = v.Scan
	const want = "pytest ran in an earlier turn and ran no checks (call 2)"
	if got := ValueLine(ran, Impact{}, "sess-1", false); !strings.Contains(got, want) {
		t.Errorf("control, a run in the record: value line %q, want %q", got, want)
	}
	if got, ok := DoneFlag(ran, "sess-1", false); !ok || !strings.Contains(got, want) {
		t.Errorf("control, a run in the record: done flag %q, %v, want %q", got, ok, want)
	}
}
