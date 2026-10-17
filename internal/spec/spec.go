// Package spec holds the repository spec's contract: the compiled form VERIFY
// evaluates against (tech spec 2.2.7). P-07 freezes the names and the limits
// the spec text states; schema parsing, the digest, discovery and the trust
// binding land in the SPEC PRs.
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

// Limits of one spec file (2.2.7): strict JSON, integers only.
const (
	MaxBytes               = 256 << 10
	MaxGates               = 32 // including ci_commands
	MaxRules               = 64
	MaxFacts               = 64
	MaxPatternBytes        = 512
	MaxPatternInstructions = 2000
	MaxSpecInstructions    = 20000
	MaxRepeat              = 100
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
