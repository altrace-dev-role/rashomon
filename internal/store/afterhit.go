package store

// AfterHit is the post's record of a call that matched the After pattern of
// an order rule of the spec in force (card SPEC-RULES1 1b, #216 ruling 1):
// kept as a rule_hit is (specevent.go), so the order judge reads records
// only and no command line is stored for it. It is a kind of its own, added
// as spec_event was after the 1.2.0 freeze, at EventSchemaVersion and
// critical: a spec_event's keys are frozen at v5 ("All v5 keys always
// present", TestDEL22b), and an after_hit carries four keys a rule_hit has
// none of, which join no existing kind. A reader that does not know the kind
// passes it over by its critical key (classifyRunLine), reading the session
// with a gap, never green past it.
//
// RuleID is the rule's id as the spec declares it, SourceDigest the content
// digest of the spec file that declared it (the value a trust binding holds,
// so an edit of that file drops the hit), LandedSeq the seq of the call's
// execution record, Event the hook event (PostToolUse | PostToolUseFailure).
// ExitCode is 0 at PostToolUse, the parsed status at a failure, null where
// none was stated. Foreground is false for a command still running in the
// background. CWDDigest is the call's declaration's keyed digest of its
// directory, "" where unknown, which no Then call's compares equal to. Maybe
// is true where the line may run the After program but the lexical read
// cannot say it does: a hit that decides nothing, so the Then after it reads
// undecided, never violated. Nothing here is command text.
type AfterHit struct {
	EventHeader
	Event        string `json:"event"`
	RuleID       string `json:"rule_id"`
	SourceDigest string `json:"source_digest"`
	LandedSeq    int64  `json:"landed_seq"`
	ExitCode     *int   `json:"exit_code"`
	Foreground   bool   `json:"foreground"`
	CWDDigest    string `json:"cwd_digest"`
	Maybe        bool   `json:"maybe"`
}

// NewAfterHit is an after_hit for session's call whose execution landed at
// landedSeq, stamped at its kind's version with critical true.
func NewAfterHit(sessionID, event, ruleID, sourceDigest string, landedSeq int64, exit *int, foreground bool, cwdDigest string, maybe bool, recordedAtMS int64) *AfterHit {
	return &AfterHit{
		EventHeader: EventHeader{Type: TypeAfterHit, SchemaVersion: EventSchemaVersion, Critical: true,
			RecordedAtMS: recordedAtMS, SessionID: sessionID},
		Event: event, RuleID: ruleID, SourceDigest: sourceDigest, LandedSeq: landedSeq,
		ExitCode: exit, Foreground: foreground, CWDDigest: cwdDigest, Maybe: maybe,
	}
}

// Kind is the pair the record carries, which AppendRecord checks.
func (e *AfterHit) Kind() (string, int) { return e.Type, e.SchemaVersion }

// SetSeq is the seq the store allocated the record under the run's lock.
func (e *AfterHit) SetSeq(seq *int64) { e.Seq = seq }

// readAfterHit files an after_hit line in run.
func readAfterHit(run *Run, line []byte, kind string, version int) error {
	var rec AfterHit
	if err := decodeAs(line, &rec, kind, version); err != nil {
		return err
	}
	run.AfterHits = append(run.AfterHits, rec)
	return nil
}
