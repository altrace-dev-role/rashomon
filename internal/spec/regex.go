package spec

import (
	"errors"
	"regexp/syntax"
)

// checkRegex applies the regular expression bounds of archive 20.2.8 to one
// source: RE2 syntax, at most MaxPatternBytes of source, no counted repeat
// above MaxRepeat, none inside another, and at most MaxPatternInstructions
// compiled instructions (len(prog.Inst)). It returns the instruction count for
// the spec-wide total, or the reason and the cap that refused it.
//
// The order is the order of cost: the byte cap before any parse, the repeat
// rules on the parsed tree, and only then the simplification that expands
// every counted repeat. Expanding a nested counted repeat is what makes
// (?:\w{1000}){1000} expensive, so a tree that nests one never reaches it.
// Cost: with the source at most 512 bytes and a repeat at most 100, the
// expansion is at most about 100 times the source length.
func checkRegex(src string) (insts int, reason Reason, limit int) {
	if len(src) > MaxPatternBytes {
		return 0, ReasonPatternLong, MaxPatternBytes
	}
	re, err := syntax.Parse(src, syntax.Perl)
	if err != nil {
		// Go refuses a repeat count above 1000 itself; that is still the
		// repeat rule, not a syntax error.
		if se := (*syntax.Error)(nil); errors.As(err, &se) && se.Code == syntax.ErrInvalidRepeatSize {
			return 0, ReasonRepeatTooLarge, MaxRepeat
		}
		return 0, ReasonPatternSyntax, 0
	}
	if reason, limit = repeatRule(re, false); reason != "" {
		return 0, reason, limit
	}
	prog, err := syntax.Compile(re.Simplify())
	if err != nil {
		return 0, ReasonPatternSyntax, 0
	}
	if len(prog.Inst) > MaxPatternInstructions {
		return 0, ReasonPatternComplex, MaxPatternInstructions
	}
	return len(prog.Inst), "", 0
}

// CheckRegex applies the archive 20.2.8 regular-expression bounds to one
// pattern. It returns the compiled instruction count, or the closed reason
// and limit that refused it. The pattern is not retained.
func CheckRegex(src string) (insts int, reason Reason, limit int) {
	return checkRegex(src)
}

// repeatRule walks a parsed expression for the two repeat rules. A counted
// repeat is {n}, {n,} or {n,m}; * + and ? are not counted. inside says the
// walk is below a counted repeat.
func repeatRule(re *syntax.Regexp, inside bool) (Reason, int) {
	if re.Op == syntax.OpRepeat {
		if re.Min > MaxRepeat || re.Max > MaxRepeat {
			return ReasonRepeatTooLarge, MaxRepeat
		}
		if inside {
			return ReasonNestedRepeat, 0
		}
		inside = true
	}
	for _, sub := range re.Sub {
		if r, l := repeatRule(sub, inside); r != "" {
			return r, l
		}
	}
	return "", 0
}
