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
// leading slash and no empty, `.` or `..` element, so a fact never names a
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
		if el == "" || el == "." || el == ".." {
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

// Finding is what VERIFY hands STEER (2.2.9). Exactly one of Gate, Claim,
// Rule and Integrity is set, by the pattern; the mapping to steer.Finding is
// the gap row "verify.Finding to steer.Finding" and lands with it.
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

// ClaimFact is the record fact behind a claim finding: the claim's class and
// verdict, the gates it mapped to, and an HMAC of its sentence under the
// install key, never the sentence (2.2.5 step 8).
type ClaimFact struct {
	Class        ClaimClass
	Source       ClaimSource
	Strict       bool
	Verdict      Verdict
	Gates        []SlotID
	GatesOmitted int
	QuoteHMAC    Digest
}

// RuleFact is the record fact behind a platform-rule or ordering-rule
// finding: the rule, its kind and trust, the call that executed the
// violation, and the program, path or gate the rule named.
type RuleFact struct {
	RuleID  SlotID
	Kind    spec.RuleKind
	Trust   spec.Trust
	Seq     int64
	Program *Program
	Path    *PathSlot
	Gate    *SlotID
}

// IntegrityFact is the record fact behind an integrity finding: the check,
// the window's net adverse count, the paths it moved on and the edit that
// moved it.
type IntegrityFact struct {
	Kind         IntegrityKind
	Count        int
	Paths        []PathSlot // at most MaxPaths
	PathsOmitted int
	Seq          int64
}
