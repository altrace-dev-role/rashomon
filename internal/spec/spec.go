// Package spec holds the repository spec's contract: the compiled form VERIFY
// evaluates against (tech spec 2.2.7). P-07 freezes the names and the limits
// the spec text states. Parse reads one spec.json strictly into a File and
// returns its digest; discovery, the managed spec and the trust binding land
// in the later SPEC PRs.
package spec

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
// on the fact it raises. Gates, rules and facts are added by the PR that
// parses them (2.2.7; archive 20.2.8).
type Compiled struct {
	Sources []Source
	Digest  string
	Trust   Trust
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
