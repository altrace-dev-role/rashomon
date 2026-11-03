package store

// ThenCheck is the post's verdict on whether a Bash call's line runs the Then
// of an order rule of the spec in force (card ORDER-LIVE1): one per Bash call
// the post read under the rule, whatever the verdict. The Stop's order judge
// reads the Then side of a line the record holds no command for from it, as
// it reads the After side from an after_hit: no command line is stored for
// either. It is a kind of its own for the reason an after_hit is (a
// spec_event's keys are frozen at v5), at EventSchemaVersion and critical.
//
// RuleID, SourceDigest and LandedSeq are an after_hit's. Then is ThenYes where
// one of the line's commands runs the rule's Then for certain, ThenMaybe where
// one may or the line was not read whole, else ThenNo. A call the record
// holds no then_check for (its post never landed, the append was lost) reads
// maybe, so the rule reads undecided and never met on a gap. Nothing here is
// command text.
type ThenCheck struct {
	EventHeader
	RuleID       string `json:"rule_id"`
	SourceDigest string `json:"source_digest"`
	LandedSeq    int64  `json:"landed_seq"`
	Then         string `json:"then"`
}

// The closed set of a then_check's verdicts.
const (
	ThenYes   = "yes"
	ThenNo    = "no"
	ThenMaybe = "maybe"
)

// NewThenCheck is a then_check for session's call whose execution landed at
// landedSeq, stamped at its kind's version with critical true.
func NewThenCheck(sessionID, ruleID, sourceDigest string, landedSeq int64, then string, recordedAtMS int64) *ThenCheck {
	return &ThenCheck{
		EventHeader: EventHeader{Type: TypeThenCheck, SchemaVersion: EventSchemaVersion, Critical: true,
			RecordedAtMS: recordedAtMS, SessionID: sessionID},
		RuleID: ruleID, SourceDigest: sourceDigest, LandedSeq: landedSeq, Then: then,
	}
}

// Kind is the pair the record carries, which AppendRecord checks.
func (e *ThenCheck) Kind() (string, int) { return e.Type, e.SchemaVersion }

// SetSeq is the seq the store allocated the record under the run's lock.
func (e *ThenCheck) SetSeq(seq *int64) { e.Seq = seq }

// readThenCheck files a then_check line in run.
func readThenCheck(run *Run, line []byte, kind string, version int) error {
	var rec ThenCheck
	if err := decodeAs(line, &rec, kind, version); err != nil {
		return err
	}
	run.ThenChecks = append(run.ThenChecks, rec)
	return nil
}
