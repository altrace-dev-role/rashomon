package engine

import "strings"

// whyClass is the reason class of a role resolution's reason text, the only
// form of it the evidence file keeps. A reason is built from a fixed
// prefix and, after it, a word the request or the index supplied ("x is
// not in the index", "named 'x': ..."); the evidence file stores nothing
// of the request, so it prints the longest prefix the engine builds and
// none of what follows. The result is "" for "", the longest literal
// prefix found in whyPrefixes, or "other": the output alphabet is the
// table, so no word of a prompt can be printed unless a table literal
// holds one, and none does.
func whyClass(s string) string {
	if s == "" {
		return ""
	}
	best := ""
	for _, p := range whyPrefixes {
		if len(p) > len(best) && strings.HasPrefix(s, p) {
			best = p
		}
	}
	if best == "" {
		return "other"
	}
	return best
}

// whyPrefixes is the closed table of the reason prefixes the engine builds
// (claim.go, roles.go, units.go; Refused and Roles.Why). A refusal is
// kept as the pick's Refused and as "refused: " plus it in the role's Why,
// so each refusal prefix appears bare and behind "refused: ". It is a
// slice of constant expressions: a map here would cost init bytes.
var whyPrefixes = []string{
	// the pick's Refused, and the role's Why behind "refused: "
	"named '", "the owner '", "no unit names the head '", "below the floor", "ambiguous: ", "'",
	"refused: named '", "refused: the owner '", "refused: no unit names the head '", "refused: below the floor", "refused: ambiguous: ", "refused: '",
	// the role's Why when nothing was refused
	"no unit is named", "no unit names the target", "anchor ",
	ClassNamed, ClassUnit, ClassQualified, ClassSingle, ClassFull, ClassPartial,
	ClassUnit + "; tie settled by ", ClassQualified + "; tie settled by ", ClassSingle + "; tie settled by ",
	ClassFull + "; tie settled by ", ClassPartial + "; tie settled by ",
}
