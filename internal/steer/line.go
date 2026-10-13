package steer

import "encoding/json"

// Facts carries the record-linkage identifiers a steer.ndjson line can
// cite (tech 4.2.3: "`facts` (each nullable: `program`, `runner_digest`,
// `target_id`, `mechanism_id`, `class_id`, `rule_id`)"). Every
// member is independently nullable -- a line populates whichever subset
// its pattern actually consulted -- so each is a pointer rather than a
// string that could not distinguish "empty" from "not applicable."
type Facts struct {
	Program      *string `json:"program"`
	RunnerDigest *string `json:"runner_digest"`
	TargetID     *string `json:"target_id"`
	MechanismID  *string `json:"mechanism_id"`
	ClassID      *string `json:"class_id"`
	RuleID       *string `json:"rule_id"`
}

// LineType is the fixed "type" value every steer.ndjson line carries
// (Technical spec 4.2.3's example: "type":"steer"). Stamping it is a
// constructor's job in a later wave; this package only names the value.
const LineType = "steer"

// LineVersion is the steer.ndjson schema version carried as Line.V
// (Technical spec 4.2.3's example: "v":1). Bump it, and this comment,
// together with any change to Line's shape that an older reader could not
// parse.
const LineVersion = 1

// Line is one record in runs/<dir>/steer.ndjson, a 0600 side file outside
// the main record store -- internal/store/kinds.go notes the same
// boundary from the other side: "There is no steer kind (ED-1): STEER's
// lines live in their own side file, runs/<dir>/steer.ndjson." Technical
// spec 4.2.3 gives every key by example plus an "Also:" list of the keys
// the example omits; this struct carries both groups. JSON tags are
// exactly the spec's lower_snake_case key names. Nullable fields are
// pointers so that "we don't know" (JSON null) stays distinct from the
// zero value of whatever type would otherwise have to stand in for it --
// matching the example line, which writes "agent_id":null and
// "suppressed_by":null explicitly rather than omitting those keys.
type Line struct {
	Type      string `json:"type"`
	V         int    `json:"v"`
	ID        string `json:"id"`
	AtUnixMS  int64  `json:"at_unix_ms"`
	SessionID string `json:"session_id"`
	PromptID  string `json:"prompt_id"`
	// AgentID is null for the main agent, matching Finding.AgentID's ""
	// meaning the same thing one layer up.
	AgentID        *string    `json:"agent_id"`
	Project        string     `json:"project"`
	Pattern        string     `json:"pattern"`
	Kind           string     `json:"kind"`
	PatternVersion int        `json:"pattern_version"`
	Level          string     `json:"level"`
	Channel        Channel    `json:"channel"`
	Event          string     `json:"event"`
	Attendance     Attendance `json:"attendance"`
	// Arm is one of the three closed Arm values (tech 4.2.9).
	Arm          Arm     `json:"arm"`
	AfterSeq     int64   `json:"after_seq"`
	EvidenceSeqs []int64 `json:"evidence_seqs"`
	// Slots are the slot values the line's message bound, by slot name,
	// kept as raw JSON: the spec's own example carries strings, numbers and
	// an array ("exit":1, "paths":[...]), and each pattern's binding struct
	// (tech 4.2.7) decides its slots' shapes.
	// TestLine_DecodesTheSpecsExampleLine reads that example verbatim.
	Slots map[string]json.RawMessage `json:"slots"`
	// ModelText and UserText are "" outside delivered and flag lines
	// (Technical spec 4.2.3: "text only on delivered and flag lines") --
	// present as empty strings, not omitted, so every line keeps every
	// key. Each is capped at 2,048 bytes (tech 1.2.9), a looser ceiling
	// than render's own 1,200/1,600-byte rendered-message cap (see
	// internal/steer/render) that leaves room for UserText's mirror
	// wrapper; enforcing it is a constructor's job in a later wave, not
	// this contract's.
	ModelText string    `json:"model_text"`
	UserText  string    `json:"user_text"`
	State     LineState `json:"state"`
	// SuppressedBy is null unless State is suppressed (or a budget_*
	// suppression produced one of the other non-delivered states).
	SuppressedBy    *SuppressedBy `json:"suppressed_by"`
	InvocationID    string        `json:"invocation_id"`
	RendererVersion int           `json:"renderer_version"`
	RulesVersion    int           `json:"rules_version"`

	// SteerID equals ID (tech 4.2.3: "`steer_id` (= `id`)"). The
	// spec does not say why a consumer would want the same value under
	// two keys; this package preserves the duplication rather than
	// resolving it, since collapsing the two here would be a guess about
	// a reader this slice cannot see.
	SteerID string `json:"steer_id"`
	// LabelRule is the MEASURE rule that labels this line's firing: one of
	// the ids of tech 5.2.4 B's table, which is the only evaluator (MSR 13,
	// decided (a) 2026-10-13). It names a rule, not an outcome; the outcome
	// is a Label in MEASURE's own label record. Null on a line whose pattern
	// has no rule assigned yet (MSR 2, tech 4.5).
	LabelRule *LabelRule `json:"label_rule"`
	// Delivery is derived per state (tech 4.2.3) and null on a candidate,
	// deferred or unconfirmed line ("`unconfirmed` carries null";
	// "`candidate` and `deferred` carry null until final").
	Delivery *Delivery `json:"delivery"`
	// MirroredToUser is whether this line's mirror reached the user: true
	// for delivered, flag_only and holdout lines.
	MirroredToUser bool  `json:"mirrored_to_user"`
	Facts          Facts `json:"facts"`
	RecordsOffset  int64 `json:"records_offset"`
	// HarnessID and Surface are opaque identifiers the spec names but
	// never enumerates (unlike, say, SuppressedBy's closed 22). Plain
	// strings, no frozen vocabulary.
	HarnessID string `json:"harness_id"`
	Surface   string `json:"surface"`
	// Mirror is the channel the mirror used on this line (tech 6.2.1): one
	// of the five Mirror values, MirrorNone when no
	// channel carried it. MirroredToUser above is the per-line outcome. The
	// zero value "" is not a Mirror: whoever writes a line sets one.
	Mirror Mirror `json:"mirror"`
	// MirrorFailure is why a mirror did not reach the user: null when it
	// did or when none was attempted.
	MirrorFailure *MirrorFailure `json:"mirror_failure"`
	// Outcome is null on every line but a stop-level ask that Claude Code
	// turned into a deny because nobody was attending (tech 4.2.3: "closed,
	// null on every other line"). The line stays delivered.
	Outcome *Outcome `json:"outcome"`
}

// LabelRule names one row of MEASURE's labelling table (tech 5.2.4 B):
// which later evidence confirms or refutes a firing. The ids are the
// table's; which pattern declares which is MSR 2's (tech 4.5).
type LabelRule string

const (
	LabelRuleGateRed        LabelRule = "gate_red"
	LabelRuleGateUnverified LabelRule = "gate_unverified"
	LabelRuleClaimVsRecord  LabelRule = "claim_vs_record"
	LabelRuleKnownFix       LabelRule = "known_fix"
	LabelRuleNeedsHuman     LabelRule = "needs_human"
	LabelRulePolicyMatch    LabelRule = "policy_match"
	LabelRuleRepeatFailure  LabelRule = "repeat_failure"
	LabelRuleAbsence        LabelRule = "absence"
	LabelRuleNone           LabelRule = "none"
)

// AllLabelRules returns the nine rule ids in tech 5.2.4 B's order.
func AllLabelRules() []LabelRule {
	return []LabelRule{
		LabelRuleGateRed, LabelRuleGateUnverified, LabelRuleClaimVsRecord,
		LabelRuleKnownFix, LabelRuleNeedsHuman, LabelRulePolicyMatch,
		LabelRuleRepeatFailure, LabelRuleAbsence, LabelRuleNone,
	}
}

// Mirror is the channel a line's mirror used (tech 6.2.1).
type Mirror string

const (
	MirrorSystemMessage    Mirror = "system_message"
	MirrorSteerLog         Mirror = "steer_log"
	MirrorNotification     Mirror = "notification"
	MirrorSDKInformational Mirror = "sdk_informational"
	MirrorNone             Mirror = "none"
)

// AllMirrors returns the five mirror channels in the spec's order.
func AllMirrors() []Mirror {
	return []Mirror{MirrorSystemMessage, MirrorSteerLog, MirrorNotification, MirrorSDKInformational, MirrorNone}
}

// MirrorFailure is why a mirror did not reach the user; a line that
// mirrored carries null.
type MirrorFailure string

const (
	MirrorFailed MirrorFailure = "failed"
	MirrorKilled MirrorFailure = "killed"
	MirrorBusy   MirrorFailure = "busy"
)

// AllMirrorFailures returns the three failure values.
func AllMirrorFailures() []MirrorFailure {
	return []MirrorFailure{MirrorFailed, MirrorKilled, MirrorBusy}
}
