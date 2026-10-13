package verify

import (
	"slices"
	"strconv"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// ToSteer is the mapping from a verify.Finding to a steer.Finding, the gap
// row "The mapping from `verify.Finding` to `steer.Finding` (`Key`,
// `MaxLevel`, `Priority`, `StopForm`, `AfterSeq`)" (tech, Contracts and
// names), whose content the row records as confirmed against the cards on
// 2026-10-13. P-07 proposal, for L to sign or amend in this review:
//
//   - Kind: only frozen kinds cross (tech 72). VERIFY's vocabularies are
//     translated, never copied: integrity checks by 2.2.6's order
//     (test_removed is test-deleted, ...); forbid_code is edit (pragma when
//     the added match is a suppression comment), forbid_command is command,
//     protect_path is integrity/frozen-path (tech 58); a claim's verdict is
//     its kind (masked_inconclusive is masked, 2.2.11), a done claim over
//     stale or absent is done-unverified, and unrecovered failures are
//     silent-failure; subtask-gate takes commit, push or pr from the point.
//   - MaxLevel and StopForm: per card at the finding's event (archive
//     40.3.4: "the highest level this pattern can use at this event"). See
//     ceilingFor.
//   - Key: the card's unit of limit, the dedupe key within a prompt from
//     40.3.4 ("fact id, gate family digest, task id"), never VERIFY's
//     DedupeKey verbatim. See keyFor.
//   - Priority: pattern.Priority (4.2.4 step 8).
//   - AfterSeq: at.Seq, the last record seq the decision saw (archive
//     40.3.4), so a record below it that is written later is "recorded
//     late" (tech 4.2.13). Evidence: the fact's record seqs, ascending,
//     each at or below AfterSeq.
//
// A finding the cards cannot place is a MappingError, never a candidate:
// the wrong facts for the pattern, a claim with no contradiction, a rule of
// another pattern's kind, an event the card does not raise it at, a query
// point, a seq outside 1..at.Seq, or an id outside its charset (agentID is
// the target agent, "" for the main one). The rendered Message is not built
// here: VERIFY hands facts, STEER renders (2.2.9).
func ToSteer(f Finding, at Point, agentID string) (steer.Finding, error) {
	fail := func(reason string) (steer.Finding, error) {
		return steer.Finding{}, &MappingError{Pattern: f.Pattern, Reason: reason}
	}
	if !pattern.Valid(f.Pattern) {
		return fail("not a frozen pattern id")
	}
	priority, ok := pattern.Priority(f.Pattern)
	if !ok {
		return fail("no candidate priority")
	}
	if !raisedByVerify(f.Pattern) {
		return fail("no level for a VERIFY pattern")
	}
	if reason := checkInput(f, at, agentID); reason != "" {
		return fail(reason)
	}
	kind, reason := kindFor(f, at)
	if reason != "" {
		return fail(reason)
	}
	level, form, ok := ceilingFor(f, kind)
	if !ok {
		return fail("the card does not raise it at event " + strconv.Quote(f.Event))
	}
	key, reason := keyFor(f, kind, at)
	if reason != "" {
		return fail(reason)
	}
	out := steer.Finding{
		Pattern:  f.Pattern,
		Kind:     kind,
		Key:      key,
		Priority: priority,
		MaxLevel: level,
		StopForm: form,
		Evidence: evidenceFor(f, at),
		AfterSeq: at.Seq,
		AgentID:  agentID,
	}
	if err := out.Check(); err != nil {
		return fail(err.Error())
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

// raisedByVerify reports the seven patterns VERIFY raises (tech 58).
func raisedByVerify(id pattern.ID) bool {
	switch id {
	case pattern.ClaimVsRecord, pattern.SubagentClaim, pattern.RegressionLocation,
		pattern.SubtaskGate, pattern.PlatformRule, pattern.OrderingRule, pattern.Integrity:
		return true
	}
	return false
}

// checkInput refuses what no card can hold, returning the reason or "".
func checkInput(f Finding, at Point, agentID string) string {
	switch {
	case at.Seq < 1:
		return "no point seq"
	case at.Kind == PointQuery || at.Kind != "" && !slices.Contains(PointKinds, at.Kind):
		return "not a point a hook decides at"
	case agentID != "" && !refForm(agentID):
		return "agent id outside its charset"
	case !factsFit(f):
		return "facts do not fit the pattern"
	}
	for _, seq := range factSeqs(f) {
		if seq < 1 || seq > at.Seq {
			return "a fact seq outside 1..point seq"
		}
	}
	return idsValid(f)
}

// factsFit reports the fact set each pattern carries (Finding's comment).
func factsFit(f Finding) bool {
	g, c, r, i := f.Gate != nil, f.Claim != nil, f.Rule != nil, f.Integrity != nil
	switch f.Pattern {
	case pattern.ClaimVsRecord, pattern.SubagentClaim:
		return c && !r && !i
	case pattern.RegressionLocation, pattern.SubtaskGate:
		return g && !c && !r && !i
	case pattern.PlatformRule, pattern.OrderingRule:
		return r && !g && !c && !i
	case pattern.Integrity:
		return r != i && !g && !c
	}
	return false
}

// idsValid checks every id a key or a slot is built from against its
// charset, returning the reason or "".
func idsValid(f Finding) string {
	// The conversion keeps this compiling should a fact field be retyped,
	// so TestVER54b, not the build, reports that regression.
	if f.Gate != nil && !SlotID(f.Gate.GateID).Valid() {
		return "gate id outside its charset"
	}
	if f.Rule != nil && (!f.Rule.RuleID.Valid() || f.Rule.Path != nil && !f.Rule.Path.Valid()) {
		return "rule id or path outside its charset"
	}
	if f.Integrity != nil && f.Integrity.Marker != "" && !f.Integrity.Marker.Valid() {
		return "marker id outside its charset"
	}
	return ""
}

// factSeqs lists the record seqs the facts name, in no order.
func factSeqs(f Finding) []int64 {
	var seqs []int64
	if f.Gate != nil {
		for _, s := range []*int64{f.Gate.LastRunSeq, f.Gate.LastGreenSeq} {
			if s != nil {
				seqs = append(seqs, *s)
			}
		}
	}
	if f.Rule != nil {
		seqs = append(seqs, f.Rule.Seq)
	}
	if f.Integrity != nil {
		seqs = append(seqs, f.Integrity.Seq)
	}
	return seqs
}

// evidenceFor is the fact seqs ascending without repeats; a claim with no
// gate beside it cites the point it was judged at.
func evidenceFor(f Finding, at Point) []int64 {
	seqs := factSeqs(f)
	if f.Claim != nil && f.Gate == nil {
		seqs = append(seqs, at.Seq)
	}
	slices.Sort(seqs)
	return slices.Compact(seqs)
}

// integrityKinds is 2.2.6's translation, VERIFY's check to STEER's kind id.
var integrityKinds = map[IntegrityKind]steer.Kind{
	TestRemoved:        "test-deleted",
	SkipAdded:          "skip-added",
	AssertionsRemoved:  "assert-drop",
	SuppressionAdded:   "suppression-added",
	GeneratedEdited:    "generated-edited",
	TestsOnlyThenGreen: "tests-only-green",
	GateWeakened:       "gate-weakened",
	GateConfigEdited:   "gate-config-edited",
}

// claimKinds is a contradicting verdict's claim-vs-record kind (2.2.9,
// 2.2.11: masked_inconclusive is masked). green, unknown (VER-17: never
// fires) and no_gate_known are no contradiction and have none.
var claimKinds = map[Verdict]steer.Kind{
	Verdict(Red):                "red",
	Verdict(MaskedInconclusive): "masked",
	Verdict(Backgrounded):       "backgrounded",
	Verdict(Stale):              "stale",
	Verdict(Absent):             "absent",
}

// kindFor is the frozen kind under the pattern, or the reason there is none.
func kindFor(f Finding, at Point) (steer.Kind, string) {
	switch f.Pattern {
	case pattern.ClaimVsRecord, pattern.SubagentClaim:
		k, ok := claimKinds[f.Claim.Verdict]
		switch {
		case f.Pattern == pattern.ClaimVsRecord && at.Kind != PointStop:
			return "", "claim-vs-record away from a stop point"
		case f.Pattern == pattern.ClaimVsRecord && f.Claim.UnrecoveredFailures > 0:
			return "silent-failure", ""
		case !ok:
			return "", "a claim verdict that contradicts nothing"
		case f.Pattern == pattern.SubagentClaim:
			return "", ""
		case f.Claim.Class == ClaimDone && (k == "stale" || k == "absent"):
			return "done-unverified", ""
		}
		return k, ""
	case pattern.SubtaskGate:
		switch at.Kind {
		case PointSubtask:
			return "", ""
		case PointCommit, PointPush, PointPR:
			return steer.Kind(at.Kind), ""
		}
		return "", "subtask-gate away from a subtask, commit, push or pr point"
	case pattern.PlatformRule:
		switch {
		case f.Rule.Kind == spec.ForbidCode && f.Rule.Pragma:
			return "pragma", ""
		case f.Rule.Kind == spec.ForbidCode:
			return "edit", ""
		case f.Rule.Kind == spec.ForbidCommand && !f.Rule.Pragma:
			return "command", ""
		}
	case pattern.OrderingRule:
		if f.Rule.Kind == spec.Order && !f.Rule.Pragma {
			return "", ""
		}
	case pattern.Integrity:
		if f.Rule == nil {
			k, ok := integrityKinds[f.Integrity.Kind]
			if ok {
				return k, ""
			}
			return "", "an integrity check outside 2.2.6"
		}
		if f.Rule.Kind == spec.ProtectPath && !f.Rule.Pragma {
			return "frozen-path", ""
		}
	case pattern.RegressionLocation:
		return "", ""
	}
	return "", "a rule of another pattern's kind"
}
