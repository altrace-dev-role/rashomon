package pgraph

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Tier says how a fact was produced and so how far it can be trusted. The set
// is closed; the zero value is invalid. A tier belongs to the producer, is
// copied onto each fact it writes, and is part of the fact's identity, so
// nothing can upgrade a fact: the same triple from another tier is another
// fact.
type Tier uint8

const (
	// Proven facts are read from go/types: the compiler's own resolution.
	Proven Tier = iota + 1
	// Algorithmic facts come from a sound-but-imprecise algorithm and may
	// include spurious edges. Class hierarchy analysis (Dispatch) is the
	// producer of this tier.
	Algorithmic
	// Proposed facts come from a model and are never evidence by themselves.
	Proposed
)

var tierNames = [...]string{Proven: "PROVEN", Algorithmic: "ALGORITHMIC", Proposed: "model-proposed"}

func (t Tier) String() string {
	if t < Proven || t > Proposed {
		return fmt.Sprintf("invalid-tier(%d)", uint8(t))
	}
	return tierNames[t]
}

// ParseTier accepts exactly the three spellings String produces.
func ParseTier(s string) (Tier, error) {
	for t := Proven; t <= Proposed; t++ {
		if tierNames[t] == s {
			return t, nil
		}
	}
	return 0, fmt.Errorf("pgraph: unknown tier %q", s)
}

// Predicate is the closed set of relations a fact can state.
type Predicate uint8

const (
	// Declares: a package declares a package-level object, a type declares
	// one of its methods.
	Declares Predicate = iota + 1
	// Calls: a function (or the package, for variable initialisers) contains
	// a call whose static callee is the object. A call on an interface names
	// the interface method; which implementations it can reach is dispatch,
	// not this predicate.
	Calls
	// Embeds: a struct or interface type embeds the named type.
	Embeds
	// Implements: the concrete type's own method set satisfies the interface.
	Implements
	// ImplementsViaPointer: only the pointer type's method set satisfies it.
	ImplementsViaPointer
	// MayCall: a function contains a call on an interface method that class
	// hierarchy analysis says can reach the concrete method. Never PROVEN.
	MayCall
)

var predicateNames = [...]string{Declares: "declares", Calls: "calls", Embeds: "embeds", Implements: "implements", ImplementsViaPointer: "implements-via-pointer", MayCall: "may-call"}

func (p Predicate) String() string {
	if p < Declares || p > MayCall {
		return fmt.Sprintf("invalid-predicate(%d)", uint8(p))
	}
	return predicateNames[p]
}

// ParsePredicate accepts exactly the spellings String produces.
func ParsePredicate(s string) (Predicate, error) {
	for p := Declares; p <= MayCall; p++ {
		if predicateNames[p] == s {
			return p, nil
		}
	}
	return 0, fmt.Errorf("pgraph: unknown predicate %q", s)
}

// Producer is who writes facts: a name, a version and a tier. The tier is a
// property of the producer. Only this package can construct a PROVEN or
// ALGORITHMIC producer; code outside it can only be model-proposed.
type Producer struct {
	name, version string
	tier          Tier
}

// ProposedProducer returns a producer whose facts are Proposed.
func ProposedProducer(name, version string) (Producer, error) {
	return newProducer(name, version, Proposed)
}

func newProducer(name, version string, t Tier) (Producer, error) {
	if strings.Contains(name, "@") {
		return Producer{}, fmt.Errorf("pgraph: producer name %q contains '@'", name)
	}
	if err := checkField("producer name", name); err != nil {
		return Producer{}, err
	}
	if err := checkField("producer version", version); err != nil {
		return Producer{}, err
	}
	return Producer{name, version, t}, nil
}

// Tier returns the tier of every fact this producer writes.
func (p Producer) Tier() Tier { return p.tier }

// String is the extractor label stored on facts: name@version.
func (p Producer) String() string { return p.name + "@" + p.version }

// MaxFieldBytes caps every stored text field (symbol, config, commit,
// extractor label) so a record has a known worst-case size.
const MaxFieldBytes = 8192

// checkField enforces the stored-field alphabet: non-empty, bounded, valid
// UTF-8, no control characters. Tab and newline are the log's separators, so
// excluding them here is what makes the format unambiguous.
func checkField(what, s string) error {
	switch {
	case s == "":
		return fmt.Errorf("pgraph: empty %s", what)
	case len(s) > MaxFieldBytes:
		return fmt.Errorf("pgraph: %s is %d bytes, cap %d", what, len(s), MaxFieldBytes)
	case !utf8.ValidString(s):
		return fmt.Errorf("pgraph: %s is not valid UTF-8", what)
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("pgraph: %s contains control character %U", what, r)
		}
	}
	return nil
}
