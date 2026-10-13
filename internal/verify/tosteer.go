package verify

import (
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// ToSteer is the mapping from a verify.Finding to a steer.Finding, the gap
// row the spec closes in P-07 (Contracts and names: "Key, MaxLevel,
// Priority, StopForm, AfterSeq"). P-07 proposal, for Luis to sign or amend
// in this review:
//
//   - Key is the DedupeKey as VERIFY built it (pattern|gate-or-rule|prompt).
//   - Priority is pattern.Priority of the pattern (4.2.4 step 8); a pattern
//     with no candidate priority maps to nothing, which is an error here
//     rather than a zero that would sort last by accident.
//   - MaxLevel and StopForm come from the pattern: claim-vs-record may reach
//     stop as a Stop continuation; subtask-gate, platform-rule and
//     ordering-rule may reach stop as a deny; regression-location is a
//     nudge; integrity and subagent-claim are flags (2.2.9's delivery table;
//     4.2.4 step 5 clamps the level further at route time).
//   - AfterSeq is the seq of the record fact the finding cites: the gate's
//     last run, the rule's violating call, the integrity edit; a claim cites
//     the point seq the caller passes, because a claim is judged at a point.
//   - Evidence lists the seqs the fact names, in the same order.
//   - Kind is the pattern's sub-kind: the gate's state for claim-vs-record,
//     the rule's kind for platform-rule, the check for integrity; empty for
//     patterns without a closed kind.
//
// The rendered Message is not built here: VERIFY hands facts, STEER renders
// (2.2.9), so Message stays nil until the pattern's render binds the slots.
func ToSteer(f Finding, pointSeq int64, agentID string) (steer.Finding, error) {
	if !pattern.Valid(f.Pattern) {
		return steer.Finding{}, &MappingError{Pattern: f.Pattern, Reason: "not a frozen pattern id"}
	}
	priority, ok := pattern.Priority(f.Pattern)
	if !ok {
		return steer.Finding{}, &MappingError{Pattern: f.Pattern, Reason: "no candidate priority"}
	}
	level, form, ok := levelFor(f.Pattern)
	if !ok {
		return steer.Finding{}, &MappingError{Pattern: f.Pattern, Reason: "no level for a VERIFY pattern"}
	}
	out := steer.Finding{
		Pattern:  f.Pattern,
		Kind:     kindFor(f),
		Key:      f.DedupeKey,
		Priority: priority,
		MaxLevel: level,
		StopForm: form,
		AfterSeq: pointSeq,
		AgentID:  agentID,
	}
	switch {
	case f.Gate != nil:
		if f.Gate.LastRunSeq != nil {
			out.AfterSeq = *f.Gate.LastRunSeq
			out.Evidence = append(out.Evidence, *f.Gate.LastRunSeq)
		}
		if f.Gate.LastGreenSeq != nil {
			out.Evidence = append(out.Evidence, *f.Gate.LastGreenSeq)
		}
	case f.Rule != nil:
		out.AfterSeq = f.Rule.Seq
		out.Evidence = []int64{f.Rule.Seq}
	case f.Integrity != nil:
		out.AfterSeq = f.Integrity.Seq
		out.Evidence = []int64{f.Integrity.Seq}
	case f.Claim != nil:
		out.Evidence = []int64{pointSeq}
	default:
		return steer.Finding{}, &MappingError{Pattern: f.Pattern, Reason: "no fact"}
	}
	if f.UserOnly {
		out.MaxLevel = steer.LevelFlag
		out.StopForm = steer.StopFormNone
	}
	return out, nil
}

// MappingError says why a finding could not become a steer candidate; it
// names the pattern and never any fact value.
type MappingError struct {
	Pattern pattern.ID
	Reason  string
}

func (e *MappingError) Error() string { return "verify: " + string(e.Pattern) + ": " + e.Reason }

// levelFor is the per-pattern ceiling (2.2.9; 4.2.4 step 5).
func levelFor(id pattern.ID) (steer.Level, steer.StopForm, bool) {
	switch id {
	case pattern.ClaimVsRecord:
		return steer.LevelStop, steer.StopFormStopContinue, true
	case pattern.SubtaskGate, pattern.PlatformRule, pattern.OrderingRule:
		return steer.LevelStop, steer.StopFormDeny, true
	case pattern.RegressionLocation:
		return steer.LevelNudge, steer.StopFormNone, true
	case pattern.Integrity, pattern.SubagentClaim:
		return steer.LevelFlag, steer.StopFormNone, true
	}
	return 0, "", false
}

// kindFor is the sub-kind under the pattern's id (4.2.3 kinds table).
func kindFor(f Finding) steer.Kind {
	switch {
	case f.Pattern == pattern.ClaimVsRecord && f.Claim != nil:
		return steer.Kind(f.Claim.Verdict)
	case f.Pattern == pattern.PlatformRule && f.Rule != nil:
		return steer.Kind(f.Rule.Kind)
	case f.Pattern == pattern.Integrity && f.Integrity != nil:
		return steer.Kind(f.Integrity.Kind)
	}
	return ""
}
