package render

import (
	"errors"
	"fmt"
	"strings"
)

// A rendered note is a statement of fact, never an order and never
// addressed to the agent (tech 4.2.7; STR-14; LUI-11). The same check runs
// over the template table at build time (TestSTR14_TemplatesAreStatementsOfFact)
// and over every rendered string (Validate), outside a quote: a quote is the
// agent's or the spec's own words, which keep only Approved's own rules.
//
// How the list is read (decided for P-07, the spec owner's confirmation
// pending): the second-person words and the phrases that can only be
// orders are refused anywhere; the single verbs are refused as the first
// word of a sentence or clause, where only a verb can stand, because as
// plain words they are also the nouns the spec's own statements use ("no
// later run is recorded", "a check ran at call 50").

// secondPerson may not appear outside a quote.
var secondPerson = []string{"you", "your", "yours"}

// imperativePhrases may not appear outside a quote (LUI-11's list).
var imperativePhrases = []string{"please", "make sure", "do not", "don't", "you must", "you should"}

// imperativeOpenings may not open a sentence or a clause (LUI-11's list).
var imperativeOpenings = []string{"run", "re-run", "use", "try", "fix", "check", "stop"}

// connectives are skipped at a clause start: an imperative after one is
// still an imperative ("then run curl").
var connectives = []string{"then", "and", "so", "now", "just", "also", "first", "next", "or", "but"}

// ErrNotAStatement is the error every refusal below wraps.
var ErrNotAStatement = errors.New("render: not a statement of fact")

// statementOfFact checks unquoted text, quotes already removed, against the
// three lists (LUI-11, as decided 2026-10-13). Words are runs of letters,
// digits, apostrophes and hyphens, compared ASCII case-insensitively; a
// slot placeholder ({name}) is a word of its own, so a template cannot hide
// an opening behind one. A clause starts at the start of the text, after
// the prefix, and after '.', ';', ':', '!', '?', ',', a dash standing on its
// own or a line feed, with any run of connectives skipped; a bare verb is
// refused there, where only a verb can stand, and nowhere else, because as
// plain words the same verbs are the nouns the spec's statements use ("no
// later run is recorded", "a check ran at call 50").
func statementOfFact(unquoted string) error {
	text := strings.ToLower(unquoted)
	for _, w := range secondPerson {
		if containsWord(text, w) {
			return fmt.Errorf("%w: second person %q outside a quote", ErrNotAStatement, w)
		}
	}
	for _, p := range imperativePhrases {
		if containsWord(text, p) {
			return fmt.Errorf("%w: imperative %q outside a quote", ErrNotAStatement, p)
		}
	}
	text = strings.TrimPrefix(text, strings.ToLower(MessagePrefix))
	text = strings.ReplaceAll(text, " - ", ". ")
	text = strings.ReplaceAll(text, "--", ". ")
	for _, clause := range strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '?' || r == '!' || r == ';' || r == ':' || r == ',' || r == '\n'
	}) {
		words := strings.Fields(clause)
		for len(words) > 0 && isConnective(strings.Trim(words[0], "()\"")) {
			words = words[1:]
		}
		if len(words) == 0 {
			continue
		}
		first := strings.Trim(words[0], "()\"")
		for _, w := range imperativeOpenings {
			if first == w {
				return fmt.Errorf("%w: a clause opens with the imperative %q outside a quote", ErrNotAStatement, w)
			}
		}
	}
	return nil
}

func isConnective(w string) bool {
	for _, c := range connectives {
		if w == c {
			return true
		}
	}
	return false
}

// containsWord reports whether text holds phrase as whole words: the byte
// before and after it, when present, is not a word byte.
func containsWord(text, phrase string) bool {
	for from := 0; from <= len(text)-len(phrase); {
		i := strings.Index(text[from:], phrase)
		if i < 0 {
			return false
		}
		i += from
		before := i == 0 || !isWordByte(text[i-1])
		after := i+len(phrase) == len(text) || !isWordByte(text[i+len(phrase)])
		if before && after {
			return true
		}
		from = i + 1
	}
	return false
}

func isWordByte(c byte) bool { return isAlnum(c) || c == '\'' || c == '-' }

// CheckTemplate runs the statement check over a template's text, with its
// {name} placeholders standing as words. A quote in the template's own text
// is not yet the agent's or the spec's words -- that only happens once a
// slot fills it, and a slot's value earns the quote's exemption by passing
// its own checks (deny list, entropy, Approved's rules) at render time. So
// a template's quote may wrap only a single {placeholder}; any other
// quoted span fails here rather than reaching the renderer's exemption.
func CheckTemplate(format string) error {
	if err := templateQuotesHoldOnlyPlaceholders(format); err != nil {
		return err
	}
	unquoted, err := outsideQuotes(format)
	if err != nil {
		return err
	}
	return statementOfFact(unquoted)
}

// templateQuotesHoldOnlyPlaceholders walks format with outsideQuotes' own
// quote rule and refuses any quoted span that is not exactly one
// {placeholder}.
func templateQuotesHoldOnlyPlaceholders(format string) error {
	inQuote := false
	start := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '\'' {
			continue
		}
		prev := i > 0 && isAlnum(format[i-1])
		next := i+1 < len(format) && isAlnum(format[i+1])
		switch {
		case !inQuote && !prev:
			inQuote = true
			start = i + 1
		case inQuote && !next:
			inQuote = false
			if q := format[start:i]; !isPlaceholderOnly(q) {
				return fmt.Errorf("%w: a template quote holds %q, not one placeholder", ErrNotAStatement, q)
			}
		}
	}
	if inQuote {
		return errors.New("render: message holds a quote that is never closed")
	}
	return nil
}

// isPlaceholderOnly reports whether q is exactly one {name} placeholder.
func isPlaceholderOnly(q string) bool {
	return len(q) >= 3 && q[0] == '{' && q[len(q)-1] == '}' && validSlotName(q[1:len(q)-1])
}

// checkTable runs CheckTemplate over every row of a template table.
func checkTable(table map[templateKey]template) error {
	var errs []error
	for key, tmpl := range table {
		if err := CheckTemplate(tmpl.format); err != nil {
			errs = append(errs, fmt.Errorf("%s/%s: %w", key.pattern, tmpl.id, err))
		}
	}
	return errors.Join(errs...)
}
