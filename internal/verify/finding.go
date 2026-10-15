package verify

import (
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// LabelClass is the class VERIFY puts on a finding's label (2.2.9; Archive
// 20.2.11).
type LabelClass string

const (
	FalseClaim   LabelClass = "false_claim"
	Absence      LabelClass = "absence"
	Literal      LabelClass = "literal"
	UserLabelled LabelClass = "user_labelled"
)

// LabelClasses is the closed list in the spec's order.
var LabelClasses = []LabelClass{FalseClaim, Absence, Literal, UserLabelled}

// MaxPaths bounds the paths a fact names (2.2.9).
const MaxPaths = 5

// The slot charsets of 2.2.9, checked byte by byte: no regexp compiles at
// package scope or at all (RES-2), and a check allocates nothing.
func lowerAlnum(c byte) bool { return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' }
func alnum(c byte) bool      { return lowerAlnum(c) || c >= 'A' && c <= 'Z' }

// idForm reports whether s is ^[a-z0-9][a-z0-9<extra>]{0,63}$.
func idForm(s, extra string) bool {
	if len(s) == 0 || len(s) > 64 || !lowerAlnum(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !lowerAlnum(s[i]) && strings.IndexByte(extra, s[i]) < 0 {
			return false
		}
	}
	return true
}

// SlotID is a gate or rule id as a fact carries it (2.2.9):
// ^[a-z0-9][a-z0-9._:-]{0,63}$.
type SlotID string

// Valid reports whether the id is within the slot charset.
func (s SlotID) Valid() bool { return idForm(string(s), "._:-") }

// PathSlot is a project-relative path as a fact carries it (2.2.9):
// ^[A-Za-z0-9._/@+-]{1,120}$.
type PathSlot string

// Valid reports whether the path is within the slot charset and relative: no
// leading slash and no empty, `.`, `..` or leading-'-' element (the last so
// a path segment cannot be read as a command flag), so a fact never names a
// file outside the project.
func (p PathSlot) Valid() bool {
	s := string(p)
	if len(s) == 0 || len(s) > 120 || s[0] == '/' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !alnum(s[i]) && strings.IndexByte("._/@+-", s[i]) < 0 {
			return false
		}
	}
	for _, el := range strings.Split(s, "/") {
		if el == "" || el == "." || el == ".." || el[0] == '-' {
			return false
		}
	}
	return true
}

// Program is argv[0] of a family row, the closed set a fact may name as a
// program (2.2.9): ^[a-z0-9][a-z0-9._+-]{0,63}$. Membership is checked
// against the family table by the renderer; this type holds the charset.
type Program string

// Valid reports whether the program is within the slot charset.
func (p Program) Valid() bool { return idForm(string(p), "._+-") }

// Digest is a lowercase hex digest (an HMAC-SHA256 or SHA-256); never text.
type Digest string

// Valid reports whether the digest is 64 lowercase hex digits.
func (d Digest) Valid() bool {
	if len(d) != 64 {
		return false
	}
	for i := 0; i < len(d); i++ {
		if c := d[i]; !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// QuoteHMAC is a claim's quote digest as the snapshot and a fact both carry
// it (archive 20.2.14): QuoteHMACPrefix followed by a 64-hex digest, never
// the sentence it was taken over.
type QuoteHMAC string

// Valid reports whether the value is QuoteHMACPrefix followed by a valid
// Digest.
func (q QuoteHMAC) Valid() bool {
	s := string(q)
	if !strings.HasPrefix(s, QuoteHMACPrefix) {
		return false
	}
	return Digest(s[len(QuoteHMACPrefix):]).Valid()
}

// MarkerID is a closed integrity-marker id as a fact carries it (archive
// 20.2.6: `go.t_skip`, `py.pytest_skip`, `js.only`): ^[a-z0-9][a-z0-9._]{0,63}$.
type MarkerID string

// Valid reports whether the marker id is within its charset.
func (m MarkerID) Valid() bool { return idForm(string(m), "._") }

// Finding is what VERIFY hands STEER (2.2.9). The pattern decides the facts:
// Gate alone for regression-location and subtask-gate; Rule alone for
// platform-rule and ordering-rule; Integrity, or a protect_path Rule
// (integrity/frozen-path), for integrity; Claim for claim-vs-record and
// subagent-claim, with Gate beside it when one gate's state set the verdict
// (P-07 proposal, owner L: the archive's claim-vs-record/red statement,
// 20.2.10, names that gate's program, exit code and run seq, which ClaimFact
// cannot carry). ToSteer refuses any other combination.
type Finding struct {
	Pattern   pattern.ID
	Class     LabelClass
	Event     string // the hook event whose output carries it
	DedupeKey string // pattern|gate-or-rule|prompt
	Gate      *GateFact
	Claim     *ClaimFact
	Rule      *RuleFact
	Integrity *IntegrityFact
	UserOnly  bool // untrusted spec, loose claim, WIP commit, shadow
}

// GateFact is the record fact behind a gate finding (2.2.9). No fact field
// may carry output or edit text (VER-54): every field is a slot, a closed
// vocabulary, a number or a flag.
type GateFact struct {
	GateID       SlotID
	Program      Program
	State        State
	Reason       Reason
	LastGreenSeq *int64
	LastRunSeq   *int64
	ExitCode     *int
	WritesSince  int
	Paths        []PathSlot // at most MaxPaths, project-relative
	PathsOmitted int
	OtherAgents  int
	PartialRuns  int
}

// ClaimFact is the record fact behind a claim finding (claim-vs-record,
// subagent-claim). Neither spec defines its fields; every one is a P-07
// proposal for L to sign or amend in P-07's review (gap row "`ClaimFact`,
// `RuleFact`, `IntegrityFact`"), each citing the section it is read from.
type ClaimFact struct {
	// Class is the closed lexicon the claim matched (P-07 proposal: 2.2.5
	// step 4).
	Class ClaimClass
	// Source is the message the claim was read from (P-07 proposal: 2.2.5
	// input; archive 20.2.14).
	Source ClaimSource
	// Strict is false when a disclosure word or a window cut loosened the
	// claim; only strict claims reach the model (P-07 proposal: 2.2.5 step
	// 8).
	Strict bool
	// Verdict is the worst state of the claim's gates by Order, or
	// no_gate_known (P-07 proposal: 2.2.5 step 7).
	Verdict Verdict
	// Gates are the gates the claim mapped to (P-07 proposal: 2.2.5 steps
	// 6 and 8, "the checkpoint keeps each claim's ... gates").
	Gates []SlotID
	// GatesOmitted counts the gates left out of Gates (P-07 proposal: the
	// checkpoint's cut rule, 2.2.10).
	GatesOmitted int
	// QuoteHMAC is the HMAC-SHA256 of the claim's sentence under the
	// install key, never the sentence (P-07 proposal: 2.2.5 step 8).
	QuoteHMAC QuoteHMAC
	// UnrecoveredFailures counts the turn's failed calls with no later
	// passing run of the same run key, lookups and answer exits excluded:
	// above zero, the finding is claim-vs-record/silent-failure (P-07
	// proposal: 2.2.5 Silence, "`silent-failure` counts only unrecovered
	// failures"; archive 20.2.10's silent-failure row and its "2 recorded
	// calls failed" statement).
	UnrecoveredFailures int
}

// RuleFact is the record fact behind a platform-rule, ordering-rule or
// integrity/frozen-path finding. Every field is a P-07 proposal (gap row
// "`ClaimFact`, `RuleFact`, `IntegrityFact`"), each citing its section.
type RuleFact struct {
	// RuleID is the rule's id in the loaded spec (P-07 proposal: 2.2.7;
	// archive 40.3.8 slot.RuleID).
	RuleID SlotID
	// Kind is the rule kind of 2.2.7's table; it decides the pattern and
	// the frozen kind (P-07 proposal: tech 58, `platform-rule` =
	// `verify.spec.forbid_code/forbid_command`, `integrity` includes
	// `verify.spec.protect_path`, `ordering-rule` = `verify.spec.order`).
	Kind spec.RuleKind
	// Trust is this rule's own trust; an untrusted rule gives a user flag
	// only (P-07 proposal: 2.2.7 Trust; 2.2.9 "user flag when untrusted";
	// per-rule trust on RuleFact decided 2026-10-13, archive 20.2.4).
	Trust spec.Trust
	// Seq is the call that executed the violation (P-07 proposal: 2.2.7,
	// "executed violation"; archive 20.2.14 violation seqs).
	Seq int64
	// Program is the program a forbid_command or order rule matched (P-07
	// proposal: archive 40.4.6 `command` form, 40.4.10 `{verb_label}`).
	Program *Program
	// Path is the file a forbid_code or protect_path rule matched (P-07
	// proposal: archive 40.4.6 `edit` form, "Once per rule per file").
	Path *PathSlot
	// Gate is the evidence gate an order rule names (P-07 proposal: 2.2.7
	// `order`; archive 40.4.10 `{evidence}`).
	Gate *SlotID
	// Added is how many matches the edit added (P-07 proposal: archive
	// 40.4.6 "added {k} match(es)"; 20.2.10 "added 1 match").
	Added int
	// Pragma is true when the forbid_code violation is an added suppression
	// comment, the platform-rule/pragma kind (P-07 proposal: archive 40.4.6
	// `pragma` trigger).
	Pragma bool
	// Rewritten is true when the call ran other than the model asked,
	// another hook having rewritten its input (the executed digest differs
	// from the declared one), or a known rewriting hook affects the rule; it
	// caps the finding at a user flag (P-07 proposal: tech 2.2.7's
	// forbid_command row, "marked `rewritten` when the executed digest
	// differs"; archive 40.4.6 "flag when another hook rewrote the input",
	// 40.4.10 "capped at flag"; STR-57).
	Rewritten bool
}

// IntegrityFact is the record fact behind an integrity finding from one of
// VERIFY's eight checks. Every field is a P-07 proposal (gap row
// "`ClaimFact`, `RuleFact`, `IntegrityFact`"), each citing its section.
type IntegrityFact struct {
	// Kind is the check (P-07 proposal: 2.2.6, mapped to STEER's kind ids
	// in the same order).
	Kind IntegrityKind
	// Count is the window's net adverse count (P-07 proposal: 2.2.6,
	// "unresolved while the window's net delta stays adverse").
	Count int
	// Paths are the files the check moved on, at most MaxPaths (P-07
	// proposal: archive 40.4.7 user line `{path}`).
	Paths []PathSlot
	// PathsOmitted counts the paths left out (P-07 proposal: 2.2.10's cut
	// rule).
	PathsOmitted int
	// Seq is the edit that moved the check (P-07 proposal: archive 20.2.14
	// integrity.unresolved[].seq).
	Seq int64
	// Marker is the closed marker id that moved, empty when the check has
	// none (P-07 proposal: archive 20.2.14 `"marker": "go.t_skip"`; 20.2.10
	// "(t.Skip)").
	Marker MarkerID
}
