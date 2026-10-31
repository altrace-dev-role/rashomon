package verify

import (
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Versions stamped on every snapshot and checkpoint (2.2.10; archive 20.2.7
// and 20.2.14). RulesVersion is shape.RulesVersion, the one counter that
// names the lists a build decides with; VERIFY carries no second one.
// PracticeVersion is 1 by the spec (2.2.8).
const (
	Version         = 1
	RulesVersion    = shape.RulesVersion
	PracticeVersion = 1
)

// Point is where in the record a snapshot is taken (2.2.4; archive 20.2.4).
type Point struct {
	Kind      string  `json:"kind"`
	Seq       int64   `json:"seq"`
	SessionID string  `json:"session_id"`
	PromptID  *string `json:"prompt_id"`
	AgentID   *string `json:"agent_id"`
	TaskRef   *string `json:"task_ref"`
	// Tree is the content at the point of each file the session's tree
	// names, by project-relative path (TreeTargets): the sha256, or nil for
	// a file gone, not regular or over 1 MiB; no entry for one unread. The
	// Stop hook reads it (hook.TreeShas), the one I/O the binding does; nil
	// at a point that read none, and over MaxTreeFiles (P5A, D140). Not
	// encoded: the checkpoint keeps the states it decided, not the shas.
	Tree map[string]*string `json:"-"`
	// StoreRoot is rashomon's store root, which the caller knows (D173):
	// integrity's store_touched also names a write the session made under
	// it, and the raise of one names the store. "" names no store. Not
	// encoded, as Tree is not.
	StoreRoot string `json:"-"`
}

// Point kinds (archive 20.2.4: stop|subagent_stop|subtask|commit|push|pr|query).
const (
	PointStop         = "stop"          // the turn's Stop checkpoint
	PointSubagentStop = "subagent_stop" // a SubagentStop checkpoint
	PointSubtask      = "subtask"       // a TaskUpdate or TodoWrite completion
	PointCommit       = "commit"        // the seq of an ok `git commit`
	PointPush         = "push"          // the seq of an ok `git push`
	PointPR           = "pr"            // the seq of an ok non-draft `gh pr create`
	PointQuery        = "query"         // `rashomon verify --at SEQ`, a point no record made
)

// PointKinds is the closed list in the archive's order.
var PointKinds = []string{PointStop, PointSubagentStop, PointSubtask, PointCommit, PointPush, PointPR, PointQuery}

// MaxWriters bounds writers_since_green (2.2.4).
const MaxWriters = 8

// RunRef is a gate run as a gate's state cites it (archive 20.2.4).
type RunRef struct {
	Seq        int64             `json:"seq"`
	ToolUseID  string            `json:"tool_use_id"`
	AgentID    *string           `json:"agent_id"`
	Outcome    string            `json:"outcome"` // ok | failed
	ExitCode   *int              `json:"exit_code"`
	Masked     *string           `json:"status_masked"`
	GateOutput *store.GateOutput `json:"gate_output"`
	Scope      string            `json:"scope"` // full | partial | unknown
}

// WriteRef is a relevant write as a gate's state cites it (archive 20.2.4).
type WriteRef struct {
	Seq     int64   `json:"seq"`
	AgentID *string `json:"agent_id"`
	Path    *string `json:"path"` // project-relative; null when the target is unknown
	Tool    string  `json:"tool"`
}

// AgentCount is one writer since the last green (archive 20.2.4).
type AgentCount struct {
	AgentID   *string `json:"agent_id"` // null is the main agent
	AgentType *string `json:"agent_type"`
	Writes    int     `json:"writes"`
}

// GateState is one gate's state at the point, with the keys of 2.2.4 and
// the field types of archive 20.2.4.
type GateState struct {
	Gate              string       `json:"gate"`   // "learned:go-test", "spec:unit", "ci:0"
	Kind              string       `json:"kind"`   // test | build | lint | typecheck | scan | custom
	Family            string       `json:"family"` // closed family name, or "declared"
	Source            string       `json:"source"` // declared | ci_commands | learned
	Trust             string       `json:"trust"`  // builtin | untrusted | trusted | managed
	Required          bool         `json:"required"`
	State             State        `json:"state"`
	Reason            Reason       `json:"reason"`
	LastRun           *RunRef      `json:"last_run"`
	LastGreen         *RunRef      `json:"last_green"`
	LastWrite         *WriteRef    `json:"last_write"`
	WritesSinceGreen  int          `json:"writes_since_green"`
	WritersSinceGreen []AgentCount `json:"writers_since_green"` // at most MaxWriters, then WritersOmitted
	WritersOmitted    int          `json:"writers_omitted"`
	UnknownTarget     int          `json:"unknown_target_writes"`
	PartialRunsSince  int          `json:"partial_runs_since"`

	// bound is the last run's tree against the point's where the last run
	// is green (bind), which the evidence_bound row reads; unknown with no
	// tree. Not a key: the archive's GateState has none for it (P5A).
	bound treeBind
}

// Recording is whether the window's record is complete; incomplete recording
// silences the comparator and gives TurnSummary its unknown row (2.2.5,
// Silence; 2.2.11).
type Recording string

const (
	Complete   Recording = "complete"
	Incomplete Recording = "incomplete"
)

// SpecInfo identifies the spec a snapshot was taken under (archive 20.2.14).
type SpecInfo struct {
	Path   string     `json:"path"`
	Digest string     `json:"digest"`
	Trust  spec.Trust `json:"trust"`
	Errors []string   `json:"errors"`
}

// ClaimSource is the message a claim was read from (archive 20.2.14:
// final_message; 2.2.5 names the subagent account and the handback).
type ClaimSource string

const (
	SourceFinalMessage ClaimSource = "final_message"
	SourceSubagentStop ClaimSource = "subagent_stop"
	SourceHandback     ClaimSource = "handback"
)

// ClaimClass is the closed lexicon a claim matched (2.2.5 step 4).
type ClaimClass string

const (
	ClaimTestsPass      ClaimClass = "tests_pass"
	ClaimBuildOK        ClaimClass = "build_ok"
	ClaimLintClean      ClaimClass = "lint_clean"
	ClaimTypecheckClean ClaimClass = "typecheck_clean"
	ClaimScanClean      ClaimClass = "scan_clean"
	ClaimDone           ClaimClass = "done"
)

// ClaimClasses is the closed lexicon in the spec's order.
var ClaimClasses = []ClaimClass{
	ClaimTestsPass, ClaimBuildOK, ClaimLintClean, ClaimTypecheckClean, ClaimScanClean, ClaimDone,
}

// QuoteHMACPrefix opens a claim's quote_hmac (archive 20.2.14: "hmac-sha256:").
const QuoteHMACPrefix = "hmac-sha256:"

// ClaimVerdict is one compared claim as the snapshot and the checkpoint keep
// it (archive 20.2.14): its source, class, strictness, verdict, the gates it
// mapped to, and an HMAC of its sentence under the install key, never the
// sentence.
type ClaimVerdict struct {
	Source    ClaimSource `json:"source"`
	Class     ClaimClass  `json:"class"`
	Strict    bool        `json:"strict"`
	Verdict   Verdict     `json:"verdict"`
	Gates     []SlotID    `json:"gates"`
	QuoteHMAC QuoteHMAC   `json:"quote_hmac"` // QuoteHMACPrefix + hex
}

// RuleVerdictValue is a rule's verdict: a window verdict (2.2.7) or a
// violation at the point (archive 20.2.14).
type RuleVerdictValue string

const (
	NoRecordedViolation RuleVerdictValue = "no_recorded_violation"
	RuleUnknown         RuleVerdictValue = "unknown"
	Insufficient        RuleVerdictValue = "insufficient" // fewer than 20 calls
	Violation           RuleVerdictValue = "violation"
)

// RuleVerdict is one rule's verdict with the seqs of its executed violations
// (archive 20.2.14) and the rule's hard_control text, printed beside a
// no_recorded_violation (2.2.7). P-07 proposal: hard_control travels on the
// snapshot's verdict entry so the snapshot can print it without the spec;
// the checkpoint stores CheckpointRuleVerdict, which has no hard_control.
type RuleVerdict struct {
	Rule        SlotID           `json:"rule"`
	Kind        spec.RuleKind    `json:"kind"`
	Verdict     RuleVerdictValue `json:"verdict"`
	Seqs        []int64          `json:"seqs"`
	HardControl string           `json:"hard_control"`
	// trust is the rule's own source's (spec.Compiled.TrustOf): only a
	// trusted or managed rule's verdict owes a ledger row and holds
	// record-green false. Unexported, so the encoding is unchanged, like
	// GateState's bound.
	trust spec.Trust
}

// CheckpointRuleVerdict is a rule verdict as the checkpoint keeps it
// (archive 20.2.14's example: rule, kind, verdict, seqs): RuleVerdict
// without hard_control. That field is the spec's own free text, whatever
// the spec's trust, so it stays on the Snapshot, which only prints it, and
// is never written to the record.
type CheckpointRuleVerdict struct {
	Rule    SlotID           `json:"rule"`
	Kind    spec.RuleKind    `json:"kind"`
	Verdict RuleVerdictValue `json:"verdict"`
	Seqs    []int64          `json:"seqs"`
}

// IntegrityKind names the eight lexical integrity checks (2.2.6), in the
// spec's order, then store_touched (card 3.2); STEER's kind ids follow the
// same order under `integrity/`.
type IntegrityKind string

const (
	TestRemoved        IntegrityKind = "test_removed"
	SkipAdded          IntegrityKind = "skip_added"
	AssertionsRemoved  IntegrityKind = "assertions_removed"
	SuppressionAdded   IntegrityKind = "suppression_added"
	GeneratedEdited    IntegrityKind = "generated_edited"
	TestsOnlyThenGreen IntegrityKind = "tests_only_then_green"
	GateWeakened       IntegrityKind = "gate_weakened"
	GateConfigEdited   IntegrityKind = "gate_config_edited"
	// StoreTouched is a recorded call that targets a path under
	// .claude/rashomon/, whatever its outcome (card 3.2, D131, D135);
	// STEER's reserved store-touched kind.
	StoreTouched IntegrityKind = "store_touched"
)

// IntegrityKinds is the closed list in the spec's order.
var IntegrityKinds = []IntegrityKind{
	TestRemoved, SkipAdded, AssertionsRemoved, SuppressionAdded,
	GeneratedEdited, TestsOnlyThenGreen, GateWeakened, GateConfigEdited,
	StoreTouched,
}

// IntegrityEntry is one unresolved check with the marker and the edit that
// moved it (archive 20.2.14), the seq being what the cut order sorts by.
type IntegrityEntry struct {
	Check  IntegrityKind `json:"check"`
	Marker string        `json:"marker"` // closed marker id, e.g. go.t_skip
	Seq    int64         `json:"seq"`
	// Path is the path a store_touched entry's call targeted, as the record
	// names it (project-relative under the project, as named elsewhere, so
	// absolute for a write under a store root outside it, D173); "" on a
	// store_touched entry from a call's store-root bit where the record
	// withholds its paths (D173b), and "" and left out on every other check.
	Path string `json:"path,omitempty"`
}

// IntegrityCounts are the window's net deltas (archive 20.2.14).
type IntegrityCounts struct {
	TestsDelta        int `json:"tests_delta"`
	SkipsAdded        int `json:"skips_added"`
	SuppressionsAdded int `json:"suppressions_added"`
}

// Integrity is the checks still unresolved at the point and the counts.
// UnknownMarks and UnreadBases are what the fold read no further, counted so
// that ignoring is never silent (D41, D42; beyond archive 20.2.14's golden):
// mark ids outside the marker table, once per write that carries one (how
// often the table missed), and calls whose marks are not a delta for their
// basis, across the window.
type Integrity struct {
	Unresolved   []IntegrityEntry `json:"unresolved"`
	Counts       IntegrityCounts  `json:"counts"`
	UnknownMarks int              `json:"unknown_marks"`
	UnreadBases  int              `json:"unread_bases"`
}

// PracticeComponents are the five components of 2.2.8, each in [0, 1];
// a component that is not applicable is listed in Practice.NotApplicable
// and carries 0 here.
type PracticeComponents struct {
	Gates     float64 `json:"gates"`
	Claims    float64 `json:"claims"`
	Spec      float64 `json:"spec"`
	Integrity float64 `json:"integrity"`
	Hygiene   float64 `json:"hygiene"`
}

// Practice is the diagnostic score of 2.2.8 (archive 20.2.9): Score is null
// with nothing to score and never 100 by default.
type Practice struct {
	Version       int                `json:"practice_version"`
	Score         *int               `json:"score"`
	Components    PracticeComponents `json:"components"`
	NotApplicable []string           `json:"not_applicable"`
}

// SubtaskBoundaries is whether the window had subtask boundaries at all
// (archive 20.2.4): none means the coverage reason no_subtask_boundaries,
// never an empty "all subtasks fine".
type SubtaskBoundaries string

const (
	BoundariesNone  SubtaskBoundaries = "none"
	BoundariesFound SubtaskBoundaries = "found"
)

// Subtask is one subtask point in the window (2.2.4, Points): Ref is the
// task id or the HMAC of the item text and its occurrence index; Ambiguous
// marks a same-text count change. P-07 proposal: the golden shows `items`
// without its element's shape; this is it.
type Subtask struct {
	Ref         string `json:"ref"`
	FromSeq     *int64 `json:"from_seq"`
	ToSeq       int64  `json:"to_seq"`
	Ambiguous   bool   `json:"ambiguous"`
	RecordGreen bool   `json:"record_green"`
}

// Subtasks is the subtask section of a snapshot (archive 20.2.14).
type Subtasks struct {
	Boundaries SubtaskBoundaries `json:"boundaries"`
	Reason     string            `json:"reason"` // no_subtask_boundaries, or ""
	Items      []Subtask         `json:"items"`
}

// Snapshot is the state at a point as `rashomon verify --json` prints it
// (archive 20.2.14), in that key order. Evaluate returns one; the Checkpoint
// is its bounded projection into the record.
type Snapshot struct {
	VerifyVersion    int            `json:"verify_version"`
	RulesVersion     int            `json:"rules_version"`
	PracticeVersion  int            `json:"practice_version"`
	Point            Point          `json:"point"`
	Recording        Recording      `json:"recording"`
	ProjectRoot      string         `json:"project_root"`
	Spec             *SpecInfo      `json:"spec"`
	Gates            []GateState    `json:"gates"`
	Claims           []ClaimVerdict `json:"claims"`
	SpecVerdicts     []RuleVerdict  `json:"spec_verdicts"`
	Integrity        Integrity      `json:"integrity"`
	Practice         *Practice      `json:"practice"`
	RecordGreen      bool           `json:"record_green"`
	RecordGreenAtSeq *int64         `json:"record_green_at_seq"`
	Subtasks         Subtasks       `json:"subtasks"`
	Notes            []string       `json:"notes"`
}

// CheckpointMaxBytes bounds a checkpoint record (2.2.10); over it, lists are
// cut in the fixed order and Truncated is set.
const CheckpointMaxBytes = 8 << 10

// CheckpointGate is a gate's state as the checkpoint keeps it (archive
// 20.2.14): the seqs of its runs and writes, not the refs.
type CheckpointGate struct {
	Gate             string `json:"gate"`
	Kind             string `json:"kind"`
	Source           string `json:"source"`
	Required         bool   `json:"required"`
	State            State  `json:"state"`
	Reason           Reason `json:"reason"`
	LastRunSeq       *int64 `json:"last_run_seq"`
	LastGreenSeq     *int64 `json:"last_green_seq"`
	LastWriteSeq     *int64 `json:"last_write_seq"`
	WritesSinceGreen int    `json:"writes_since_green"`
	UnknownTarget    int    `json:"unknown_target_writes"`
}

// BackgroundRef is one background task of a mapped family still running at
// the point; Seq is the launch, which the cut order sorts by. P-07 proposal:
// the golden shows `background_in_flight` as an empty list without its
// element's shape; this is it.
type BackgroundRef struct {
	Family  string `json:"family"`
	TaskRef string `json:"task_ref"`
	Seq     int64  `json:"seq"`
}

// Checkpoint is the schema-5 `verify` record (2.2.10; archive 20.2.14), in
// the spec's key order, with the header every schema-5 kind carries: a null
// seq for a spilled record and critical false (an older reader skips it).
// Over CheckpointMaxBytes its lists are cut in the fixed order (practice
// components, green gates, loose and supported and no_gate_known claims,
// then background, integrity and spec entries oldest first; non-green gates
// and strict non-green claims last), each cut list keeping its *_omitted
// count, with Truncated set. It lives here and not in the store because it
// carries verify's types and the store is this package's import.
type Checkpoint struct {
	Type                string                  `json:"type"`
	SchemaVersion       int                     `json:"schema_version"`
	Critical            bool                    `json:"critical"`
	Seq                 *int64                  `json:"seq"`
	RecordedAtUnixMS    int64                   `json:"recorded_at_unix_ms"`
	SessionID           string                  `json:"session_id"`
	PromptID            *string                 `json:"prompt_id"`
	AgentID             *string                 `json:"agent_id"`
	Point               string                  `json:"point"`
	PointSeq            int64                   `json:"point_seq"`
	PointToolUseID      *string                 `json:"point_tool_use_id"`
	TaskRef             *string                 `json:"task_ref"`
	VerifyVersion       int                     `json:"verify_version"`
	RulesVersion        int                     `json:"rules_version"`
	SpecDigest          *string                 `json:"spec_digest"`
	SpecTrust           *spec.Trust             `json:"spec_trust"`
	Recording           Recording               `json:"recording"`
	ProjectRoot         string                  `json:"project_root"`
	Gates               []CheckpointGate        `json:"gates"`
	GatesOmitted        int                     `json:"gates_omitted"`
	Claims              []ClaimVerdict          `json:"claims"`
	ClaimsOmitted       int                     `json:"claims_omitted"`
	SpecVerdicts        []CheckpointRuleVerdict `json:"spec_verdicts"`
	SpecVerdictsOmitted int                     `json:"spec_verdicts_omitted"`
	Integrity           Integrity               `json:"integrity"`
	IntegrityOmitted    int                     `json:"integrity_omitted"`
	BackgroundInFlight  []BackgroundRef         `json:"background_in_flight"`
	BackgroundOmitted   int                     `json:"background_omitted"`
	Practice            *Practice               `json:"practice"`
	PracticeOmitted     int                     `json:"practice_omitted"`
	RecordGreen         bool                    `json:"record_green"`
	Truncated           bool                    `json:"truncated"`
}
