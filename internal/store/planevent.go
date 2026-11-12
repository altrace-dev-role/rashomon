package store

// TypePlanCheck is the plan-check record (W-M2.6 hook half): the post's fact
// that an accepted plan was checked against the spec's path rules, kept
// whenever the plan is checked, whether or not a note was delivered, so the
// measure reads that the check ran and what it found without the plan.
//
// It is the FIRST non-critical kind. Nothing a reader concludes about the
// session's rules depends on it, so its header says critical false
// (EventHeader.Critical) and a reader without the type counts the line
// Ignored, not Skipped (passOver): the session still reads whole. Not a
// spec_event, which is critical with a closed reason set (specevent.go).
const TypePlanCheck = "plan_check"

// PlanCheck holds no path and no word of the plan. LandedSeq is the seq of the
// ExitPlanMode call's execution record. RuleIDs are the ids of the rules that
// covered a named path, in the clear (they are the spec owner's words).
// Named, Planned and Matched count the checked facts: the distinct paths a
// rule covered, how many of them the project did not hold yet, and the
// (path, rule) pairs. PathDigests are those paths as keyed digests
// (intent.PathDigest), the same value the steer line's record form counts.
type PlanCheck struct {
	EventHeader
	LandedSeq   int64    `json:"landed_seq"`
	RuleIDs     []string `json:"rule_ids"`
	Named       int      `json:"named"`
	Planned     int      `json:"planned"`
	Matched     int      `json:"matched"`
	PathDigests []string `json:"path_digests"`
}

// NewPlanCheck is a plan_check for session's call whose execution landed at
// landedSeq, stamped at its kind's version with critical false. Its lists
// are never null.
func NewPlanCheck(sessionID string, landedSeq int64, ruleIDs []string, named, planned, matched int, pathDigests []string, recordedAtMS int64) *PlanCheck {
	if ruleIDs == nil {
		ruleIDs = []string{}
	}
	if pathDigests == nil {
		pathDigests = []string{}
	}
	return &PlanCheck{
		EventHeader: EventHeader{Type: TypePlanCheck, SchemaVersion: EventSchemaVersion, Critical: false,
			RecordedAtMS: recordedAtMS, SessionID: sessionID},
		LandedSeq: landedSeq, RuleIDs: ruleIDs, Named: named, Planned: planned, Matched: matched, PathDigests: pathDigests,
	}
}

// Kind is the pair the record carries, which AppendRecord checks.
func (e *PlanCheck) Kind() (string, int) { return e.Type, e.SchemaVersion }

// SetSeq is the seq the store allocated the record under the run's lock.
func (e *PlanCheck) SetSeq(seq *int64) { e.Seq = seq }

// readPlanCheck files a plan_check line in run.
func readPlanCheck(run *Run, line []byte, kind string, version int) error {
	var rec PlanCheck
	if err := decodeAs(line, &rec, kind, version); err != nil {
		return err
	}
	run.PlanChecks = append(run.PlanChecks, rec)
	return nil
}
