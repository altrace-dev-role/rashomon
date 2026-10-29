// Package spec holds the repository spec's contract: the compiled form VERIFY
// evaluates against (tech spec 2.2.7). P-07 freezes the names and the limits
// the spec text states. Parse reads one spec.json strictly into a File and
// returns its digest; Discover finds the files that apply to a call and
// merges them into a Compiled. The trust binding lands in a later SPEC PR.
package spec

import "time"

// Trust is how far a spec's text may travel (2.2.7, Trust).
type Trust string

const (
	// Untrusted is the default: user flags only, never `why` or a fact value.
	Untrusted Trust = "untrusted"
	// Trusted is bound by `rashomon spec trust` to (project_root, spec_path,
	// spec_digest); any edit drops it.
	Trusted Trust = "trusted"
	// Managed is the managed-directory spec, read as trusted.
	Managed Trust = "managed"
	// Builtin is the trust of a declaration no file makes (TrustOf of
	// SourceImplied; 1.3's learned gates carry the same word). It is not a
	// source's trust, so it is not in Trusts, and Lowest reads it as
	// untrusted.
	Builtin Trust = "builtin"
)

// Trusts is the closed list in the spec's declaration order.
var Trusts = []Trust{Untrusted, Trusted, Managed}

// RuleKind names the five rule kinds of 2.2.7's table.
type RuleKind string

const (
	ForbidCode      RuleKind = "forbid_code"
	ForbidCommand   RuleKind = "forbid_command"
	ProtectPath     RuleKind = "protect_path"
	RequireEvidence RuleKind = "require_evidence"
	Order           RuleKind = "order"
)

// RuleKinds is the closed list in the table's order.
var RuleKinds = []RuleKind{ForbidCode, ForbidCommand, ProtectPath, RequireEvidence, Order}

// Limits of one spec file (2.2.7; archive 20.2.8, Schema): strict JSON, integers
// only. MaxBytes is the file; MaxGates counts ci_commands entries too; the
// Pattern, Spec and Repeat limits bound every regular expression (the
// instruction counts are len(prog.Inst) of regexp/syntax).
const (
	MaxBytes               = 256 << 10
	MaxGates               = 32 // including ci_commands
	MaxRules               = 64
	MaxFacts               = 64
	MaxPatternBytes        = 512
	MaxPatternInstructions = 2000
	MaxSpecInstructions    = 20000
	MaxRepeat              = 100
	// MaxWhyRunes caps a rule's why and hard_control and a fact's why, in runes
	// (archive 20.2.8, Schema).
	MaxWhyRunes = 300
	// MaxAlternativeRunes caps a rule's alternative, in runes (archive 20.2.8,
	// Schema).
	MaxAlternativeRunes = 160
	// MaxIDBytes is the longest id the id pattern allows: one leading character
	// and 47 more (archive 20.2.8, Schema).
	MaxIDBytes = 48
	// MaxDepth caps JSON nesting. The schema nests six deep at most, so this is
	// slack, not a schema rule: it bounds the recursion that reads an untrusted
	// file and the length of an error's pointer.
	MaxDepth = 32
	// DigestPrefix opens every spec digest: "sha256:" + hex(sha256(JCS(file))).
	DigestPrefix = "sha256:"
)

// Source is one spec file that applies: the repository spec or the managed
// spec (2.2.7).
type Source struct {
	// Path is the file the spec was read from.
	Path string
	// Digest is DigestPrefix followed by the hex SHA-256 of the file's JCS form.
	Digest string
	Trust  Trust
}

// Compiled is a parsed, validated spec as Evaluate receives it (decided
// 2026-10-13 for P-07): Sources in precedence order, the managed spec first
// and then the repository spec; Digest is DigestPrefix followed by the hex
// SHA-256 over the source digests in that order, the one value the
// checkpoint's spec_digest carries; Trust is the lowest among the sources,
// the checkpoint's spec_trust, while each rule keeps its own source's trust
// on the fact it raises (2.2.7; archive 20.2.8). Discover builds it (README
// 3.1); every slice is in a fixed order, sources by precedence and entries by
// file order.
type Compiled struct {
	Sources []Source
	Digest  string
	Trust   Trust
	// Gates, Rules and Facts are the sources' declarations merged, managed
	// first, each Source indexing Sources; the rule a task file implies
	// (3.2) follows the declared rules with Source SourceImplied.
	Gates []Declared
	Rules []DeclaredRule
	Facts []DeclaredFact
	// Dropped is each repository declaration a managed one shadowed.
	Dropped []Dropped
	// Errors has one line per file that applied nothing: "<path>: <Parse's
	// error>", or "<path>: managed spec ignored: not admin-owned"; and
	// "<task path>: <ParseTask's or the read's error>" (3.2).
	Errors []string
	// Holdout is the managed spec's holdout object when it has one, else the
	// repository spec's, else the zero Holdout (on at DefaultHoldoutBP).
	Holdout Holdout
	// Task is the active task file (README 3.2): nil with none, and nil
	// with an Errors line when it was refused or could not be read.
	// TaskDigest is its digest, "" when Task is nil.
	Task       *Task
	TaskDigest string
}

// Lowest is the lower of two trusts in the order untrusted < trusted <
// managed; an unknown value is untrusted.
func Lowest(a, b Trust) Trust {
	rank := func(t Trust) int {
		switch t {
		case Trusted:
			return 1
		case Managed:
			return 2
		}
		return 0
	}
	if rank(a) <= rank(b) {
		if rank(a) == 0 {
			return Untrusted
		}
		return a
	}
	if rank(b) == 0 {
		return Untrusted
	}
	return b
}

// The field holdout's bounds (PF-MS5; README 5.6): a share in basis points of
// 500 to 2000, 1000 when the file names none, and a window of 1 to 365 days.
const (
	MinHoldoutBP         = 500
	DefaultHoldoutBP     = 1000
	MaxHoldoutBP         = 2000
	MaxHoldoutWindowDays = 365
)

// Holdout is a spec's holdout object (PF-MS5): the project's own switch for
// the W4 field holdout, which 2.5's steer line reads through On and passes to
// arm.Assign. The zero value means the file has no holdout object, and On of
// the zero value is on at DefaultHoldoutBP, 1000 basis points; a Holdout that
// Parse returns from an object is never zero, since its RateBP is 500 to 2000.
// Started is a UTC midnight, and it is set exactly when WindowDays is.
type Holdout struct {
	Off        bool      `json:"off,omitempty"`
	RateBP     int       `json:"rate_bp,omitempty"`
	Started    time.Time `json:"started,omitzero"`
	WindowDays int       `json:"window_days,omitempty"`
}

// On reports whether the holdout is on at now and, when it is, its share in
// basis points. It is off when the file says off, and off once the window has
// closed: the window runs from Started through the day Started plus
// WindowDays, in UTC, and closes at the next midnight. Before Started it is on,
// as with no window: the window only ends the holdout. The rate is 0 when off.
// On reads no clock; now is the hook's.
func (h Holdout) On(now time.Time) (on bool, rateBP int) {
	if h.Off {
		return false, 0
	}
	if h.WindowDays > 0 && !now.Before(h.Started.AddDate(0, 0, h.WindowDays+1)) {
		return false, 0
	}
	if h.RateBP == 0 {
		return true, DefaultHoldoutBP
	}
	return true, h.RateBP
}
