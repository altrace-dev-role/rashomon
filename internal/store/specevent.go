package store

import "slices"

// SpecEventRuleHit is a spec_event's reason when a call matched a
// forbid_command rule of the spec it was judged against (card
// SPEC-RULES-STOP1): the post's verdict at the call, kept, since the record
// keeps no command line at record scope v4 and the Stop could not judge the
// line again.
const SpecEventRuleHit = "rule_hit"

// specEventReasons is the closed reason set this reader accepts. A spec_event
// with any other reason is passed over by its own critical key (passOver),
// as a kind this reader refuses is: a later binary's reason that carries a
// verdict stamps critical true, so this one reads a gap, never green past it.
var specEventReasons = []string{SpecEventRuleHit}

// SpecEventReasons is the closed reason set, a copy.
func SpecEventReasons() []string { return slices.Clone(specEventReasons) }

// SpecEvent is a fact about the session's spec rules that only the hook saw
// (card SPEC-RULES-STOP1, D-e): one kind, its reason one of
// specEventReasons. It is the first critical event record: a reader that
// does not understand it cannot reach the session's rule verdicts, so an
// older reader counts it Skipped and the session reads with a gap (1.2.2).
//
// A rule_hit is the post's match of rule RuleID, declared in the spec file
// whose content digest was SourceDigest (spec.Source.Digest, the value a
// trust binding holds), by the call whose execution landed at LandedSeq, at
// hook event Event (PostToolUse | PostToolUseFailure). It holds no command
// text: the rule id is the spec owner's word, and the call is named by seq.
type SpecEvent struct {
	EventHeader
	Reason       string `json:"reason"`
	Event        string `json:"event"`
	RuleID       string `json:"rule_id"`
	SourceDigest string `json:"source_digest"`
	LandedSeq    int64  `json:"landed_seq"`
}

// NewRuleHit is a rule_hit for session's call whose execution landed at
// landedSeq, stamped at its kind's version with critical true.
func NewRuleHit(sessionID, event, ruleID, sourceDigest string, landedSeq, recordedAtMS int64) *SpecEvent {
	return &SpecEvent{
		EventHeader: EventHeader{Type: TypeSpecEvent, SchemaVersion: EventSchemaVersion, Critical: true,
			RecordedAtMS: recordedAtMS, SessionID: sessionID},
		Reason: SpecEventRuleHit, Event: event, RuleID: ruleID, SourceDigest: sourceDigest, LandedSeq: landedSeq,
	}
}

// Kind is the pair the record carries, which AppendRecord checks.
func (e *SpecEvent) Kind() (string, int) { return e.Type, e.SchemaVersion }

// SetSeq is the seq the store allocated the record under the run's lock.
func (e *SpecEvent) SetSeq(seq *int64) { e.Seq = seq }

// knownSpecReason reports whether this reader reads reason.
func knownSpecReason(reason string) bool { return slices.Contains(specEventReasons, reason) }

// readSpecEvent files a spec_event line in run: one of a known reason in
// SpecEvents; any other passed over by the critical key its head carries, a
// *bool, so an absent key is must-understand (runHead), never the false
// EventHeader's bool would decode it as.
func readSpecEvent(run *Run, line []byte, kind string, version int) error {
	var rec SpecEvent
	if err := decodeAs(line, &rec, kind, version); err != nil {
		return err
	}
	if !knownSpecReason(rec.Reason) {
		var head runHead
		if err := decodeAs(line, &head, kind, version); err != nil {
			return err
		}
		passOver(run, head.Critical)
		return nil
	}
	run.SpecEvents = append(run.SpecEvents, rec)
	return nil
}
