package spec

import (
	"errors"
	"regexp/syntax"
)

// checkRegex applies the archive's regular expression limits (20.2.8) to one
// source and returns the compiled program's instruction count, or the reason
// it is refused. The order is the order of cost: the byte cap before any
// parse, the parse (RE2 syntax, so no backreference or lookaround), the
// repeat rules read from the unsimplified tree, and only then the
// simplification that expands every counted repeat and the compile that
// counts the result. Simplifying a nested counted repeat is what the
// archive's example, (?:\w{1000}){1000}, would make expensive, so a tree
// that nests one never reaches it.
//
// Cost: the source is at most MaxPatternBytes, a counted repeat at most
// MaxRepeat and none nests, so the expansion is bounded by roughly
// MaxRepeat times the source length before the instruction cap sees it.
func checkRegex(src string) (int, Reason) {
	if len(src) > MaxPatternBytes {
		return 0, ReasonPatternTooLong
	}
	re, err := syntax.Parse(src, syntax.Perl)
	if err != nil {
		// Go refuses a repeat count above 1000 itself; that is still the
		// repeat rule, not a syntax error.
		if se := (*syntax.Error)(nil); errors.As(err, &se) && se.Code == syntax.ErrInvalidRepeatSize {
			return 0, ReasonRepeatTooLarge
		}
		return 0, ReasonPatternSyntax
	}
	if r := repeatReason(re, false); r != "" {
		return 0, r
	}
	prog, err := syntax.Compile(re.Simplify())
	if err != nil {
		return 0, ReasonPatternSyntax
	}
	if len(prog.Inst) > MaxPatternInstructions {
		return len(prog.Inst), ReasonPatternComplex
	}
	return len(prog.Inst), ""
}

// repeatReason walks a parsed expression for the two repeat rules: no count
// above MaxRepeat, and no counted repeat inside another. A counted repeat is
// any {n}, {n,} or {n,m}; * + and ? are not counted. inside says the walk is
// below a counted repeat.
func repeatReason(re *syntax.Regexp, inside bool) Reason {
	if re.Op == syntax.OpRepeat {
		if re.Min > MaxRepeat || re.Max > MaxRepeat {
			return ReasonRepeatTooLarge
		}
		if inside {
			return ReasonNestedRepeat
		}
		inside = true
	}
	for _, sub := range re.Sub {
		if r := repeatReason(sub, inside); r != "" {
			return r
		}
	}
	return ""
}
