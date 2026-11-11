package knowledge

import (
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// Refusal is one predicate of an otherwise admitted row that Rules did not
// compile: the row's id, the predicate's kind and why.
type Refusal struct {
	Row       string
	Predicate string
	Reason    string
}

// ReasonRowKind is the Refusal reason of an approved DECLARED row of a kind
// that does not compile and that names a known predicate.
const ReasonRowKind = "row kind not compiled"

// rowKindCompiles is the one switch of which row kinds may compile (W-M2.1,
// ruling E2). A compiled rule's kind is derived from its predicate, never
// from the row: every predicate in spec.RulePredicates is a decision check,
// so every compiled rule is a decision. The row kind only admits the row.
// A row of another kind that is approved, DECLARED and names a known
// predicate is refused with ReasonRowKind; admitting a kind is adding it here.
func rowKindCompiles(kind string) bool { return kind == KindDecision }

// compiledControl is the HardControl of every compiled rule, on spec's task
// rule precedent: one fixed sentence naming rashomon and the source row. A
// decision is not enforced by any evaluator today
// (spec.UnenforcedRuleKinds), so the sentence says it blocks nothing.
const compiledControl = "rashomon compiled this rule from approved knowledge row %s; it is a nudge and blocks nothing"

// Rules compiles knowledge rows to spec rules. It admits a row whose status
// is approved, whose tier is DECLARED and whose kind rowKindCompiles allows,
// and compiles one decision rule per predicate named in spec.RulePredicates.
// A row with prose and no predicate, a candidate and an INFERRED row
// compile to nothing; there is no prose parsing. Returned as refusals, in
// input order: an admitted row's unknown predicate, a row with no
// statement, and a row of another kind that names a known predicate. Pure and deterministic: no I/O, output in input
// order.
func Rules(rows []Row) ([]spec.Rule, []Refusal) {
	var out []spec.Rule
	var refused []Refusal
	for _, row := range rows {
		if row.Status != StatusApproved || row.Tier != TierDeclared || len(row.Predicates) == 0 {
			continue
		}
		if !rowKindCompiles(row.Kind) {
			// An approved check the binary ignores must be countable.
			for _, p := range row.Predicates {
				if slices.Contains(spec.RulePredicates, p.Kind) {
					refused = append(refused, Refusal{Row: row.ID, Predicate: p.Kind, Reason: ReasonRowKind})
				}
			}
			continue
		}
		why := statement(row)
		for i, p := range row.Predicates {
			switch {
			case !slices.Contains(spec.RulePredicates, p.Kind):
				refused = append(refused, Refusal{Row: row.ID, Predicate: p.Kind, Reason: "predicate is not in spec.RulePredicates"})
			case why == "":
				refused = append(refused, Refusal{Row: row.ID, Predicate: p.Kind, Reason: "row has no statement (clause and title are empty)"})
			default:
				out = append(out, compileRule(row, i, p, why))
			}
		}
	}
	return out, refused
}

func compileRule(row Row, i int, p Predicate, why string) spec.Rule {
	r := spec.Rule{
		ID: ruleID(row.ID, i), Kind: string(spec.Decision), Why: why,
		HardControl: strings.Replace(compiledControl, "%s", clip(cleanText(row.ID), 60), 1),
		Predicate:   p.Kind, AppliesTo: slices.Clone(row.AppliesTo), Source: row.ID,
	}
	if p.Kind == spec.PredicatePathsUntouched && p.Arg != "" {
		r.Paths = []string{p.Arg}
	}
	return r
}

// statement is the row's own sentence: the clause, else the title, as one
// line cut to spec.MaxWhyRunes.
func statement(row Row) string {
	s := cleanText(row.Clause)
	if s == "" {
		s = cleanText(row.Title)
	}
	return clip(s, spec.MaxWhyRunes)
}

// cleanText drops control characters and trims space.
func cleanText(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s))
}

func clip(s string, runes int) string {
	for utf8.RuneCountInString(s) > runes {
		_, n := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-n]
	}
	return strings.TrimSpace(s)
}

// ruleID is a spec id (^[a-z0-9][a-z0-9._-]{0,47}$) from the row id and the
// predicate's index: "k-" and the id lowercased with every other character
// a hyphen, cut so the "-<index>" suffix always fits.
func ruleID(id string, i int) string {
	suffix := "-" + strconv.Itoa(i)
	b := []byte("k-")
	for _, c := range []byte(strings.ToLower(id)) {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-' {
			b = append(b, c)
		} else {
			b = append(b, '-')
		}
	}
	if max := spec.MaxIDBytes - len(suffix); len(b) > max {
		b = b[:max]
	}
	return string(b) + suffix
}
