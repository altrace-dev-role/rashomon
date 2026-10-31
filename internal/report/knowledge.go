package report

// Knowledge is what the project knows, as the record shows it for one
// session (card REPORT1b, D162: evidence, never a score). It reads four
// stores that already exist and keeps none of its own: the project index
// (which gate families the project requires), spec-trust.json (the bindings,
// MAC-checked), the lesson ledger (read without writing), and the session's
// checkpoints compared pairwise, the first against the project's newest
// checkpoint before it. Every item names the store or the checkpoint seq it
// came from, and nothing is estimated.
//
// Null on a Session the report did not read it for: a report of every
// session reads none, and a session with no checkpoint has nothing to fold.
type Knowledge struct {
	// Project is the project root as the checkpoints name it.
	Project string `json:"project"`
	// Baseline is the project's newest checkpoint before this session's
	// first, null when there is none.
	Baseline *KnowledgeBaseline `json:"baseline"`
	// BaselineWalkCap is, when the baseline's best-effort walk looked at only
	// the first this-many session directories of a larger store, that number,
	// else 0: a null baseline is then "not found there" and a found one may
	// not be the newest.
	BaselineWalkCap int      `json:"baseline_walk_cap"`
	Retained        Retained `json:"retained"`
	Discovered      []Change `json:"discovered"`
	Invalidated     []Change `json:"invalidated"`
	// GateStates is, per gate, how many times its state changed between two
	// consecutive checkpoints; empty when any checkpoint cut gates
	// (GatesCut says by how many at most).
	GateStates []GateStates `json:"gate_states"`
	GatesCut   int          `json:"gates_cut"`
}

// KnowledgeBaseline is the checkpoint the fold starts from.
type KnowledgeBaseline struct {
	SessionID        string `json:"session_id"`
	RecordedAtUnixMS int64  `json:"recorded_at_unix_ms"`
	PointSeq         int64  `json:"point_seq"`
}

// Retained is what the project holds now. Prior is true when the index
// establishes no gate family, so every gate is required (the fail-closed
// prior), which RequiredFamilies 0 would misread. Specs is the bindings in
// spec-trust.json for this project; BindingsUnsigned says the file did not
// carry the store's MAC, so none is listed. SpecAtLastStop is the trust the
// session's last checkpoint read the spec under, which is not the same fact
// as a binding. Lessons is the approved ones; LedgerUnchecked says the
// ledger's state did not check, so every lesson reads as proposed.
type Retained struct {
	Prior            bool        `json:"prior"`
	RequiredFamilies int         `json:"required_families"`
	Specs            []SpecHeld  `json:"specs"`
	BindingsUnsigned bool        `json:"bindings_unsigned"`
	SpecAtLastStop   *SpecAtStop `json:"spec_at_last_stop"`
	Lessons          []LessonRef `json:"lessons"`
	LedgerUnchecked  bool        `json:"ledger_unchecked"`
	Facts            string      `json:"facts"`
}

// SpecHeld is one binding: the spec file, the digest of the file as bound
// (Parse's, not the compiled digest) and when it was bound.
type SpecHeld struct {
	Path       string `json:"path"`
	FileDigest string `json:"file_digest"`
	BoundAtMS  int64  `json:"bound_at_unix_ms"`
}

// SpecAtStop is the spec's trust and compiled digest at a checkpoint.
type SpecAtStop struct {
	Trust    string `json:"trust"`
	Compiled string `json:"compiled"`
	PointSeq int64  `json:"point_seq"`
}

// LessonRef is an approved lesson: its id and tier.
type LessonRef struct {
	ID   string `json:"id"`
	Tier string `json:"tier"`
}

// Change is one thing discovered or invalidated: what changed, from and to
// (empty where one side is none), the store or checkpoint that shows it, and
// the checkpoint's point seq or the time when it has one.
type Change struct {
	What     string `json:"what"`
	From     string `json:"from"`
	To       string `json:"to"`
	Source   string `json:"source"`
	PointSeq *int64 `json:"point_seq"`
	AtMS     *int64 `json:"at_unix_ms"`
}

// GateStates is one gate's tally across the fold: how many state changes
// between consecutive checkpoints, and the last one's from, to and seq.
type GateStates struct {
	Gate     string `json:"gate"`
	Changes  int    `json:"changes"`
	LastFrom string `json:"last_from"`
	LastTo   string `json:"last_to"`
	LastSeq  int64  `json:"last_seq"`
}
