package engine

import (
	"strings"
	"testing"
)

// A coordinated list is one constituent (W-M1.28): its commas end no
// clause, so what the clause says after the list's last item stays the
// verb's. Before the rule "in report, seed and spend using the line
// scanner" cut "seed and spend using the line scanner" into a clause of
// its own, which opened with no verb and named nothing, and the use phrase
// was lost. Lists of two, three and four items, with the Oxford comma and
// without it, and "or" for "and", read the same: the clause keeps the
// first item, as a phrase keeps the first of two things an "and" joins.
func TestClauses_ACoordinatedListIsOneConstituent(t *testing.T) {
	const verb, tail = "Replace every hand-rolled line reader in ", " using the line scanner."
	for _, list := range []string{
		"report and seed",
		"report, seed and spend",
		"report, seed, and spend",
		"report, seed, spend and scan",
		"report, seed, spend, and scan",
		"report, seed or spend",
		"report, the seed loader and the spend scanner",
	} {
		prompt := verb + list + tail
		f := Parse(prompt)
		if len(f.Targets) != 2 || words(f.Targets[0]) != tokensOf("hand-rolled line reader") || words(f.Targets[1]) != tokensOf("report") {
			t.Errorf("%q: targets %v, want [hand-rolled line reader] then [report]", prompt, f.Targets)
		}
		if len(f.Uses) != 1 || words(f.Uses[0]) != tokensOf("line scanner") {
			t.Errorf("%q: uses %v, want [line scanner]: the list's commas cut the use phrase off the verb", prompt, f.Uses)
		}
		if cl := clausesOf(strings.TrimSuffix(prompt, ".")); len(cl) != 1 {
			t.Errorf("%q: clauses %q, want one", prompt, cl)
		}
	}
}

// A comma before a clause still ends the one before it: a join word after
// the comma with no list before it ("then fix", "and how many"), a verb
// after it ("log and retry"), and a list's last item that does not stand
// alone ("the writer drops rows and the reader spins" is two clauses, not
// items) (W-M1.28).
func TestClauses_ACommaBeforeAClauseStillEndsIt(t *testing.T) {
	for _, tc := range []struct{ sentence, want string }{
		{"update report, then fix the seed test", "update report | fix the seed test"},
		{"how many callers are proven, and how many are proven or possible", "how many callers are proven | how many are proven or possible"},
		{"For every Go symbol an edit touches, report two numbers", "For every Go symbol an edit touches | report two numbers"},
		{"On failure, log and retry", "On failure | log | retry"},
		{"Without changing the log format, make the linker skip anonymous receivers", "Without changing the log format | make the linker skip anonymous receivers"},
		{"Fix the queue, the writer drops rows and the reader spins", "Fix the queue | the writer drops rows and the reader spins"},
		{"Fix the store, the cache, then the loader", "Fix the store | the cache | the loader"},
	} {
		var got []string
		for _, cl := range clausesOf(tc.sentence) {
			got = append(got, strings.Join(cl, " "))
		}
		if strings.Join(got, " | ") != tc.want {
			t.Errorf("%q: clauses %q, want %q", tc.sentence, strings.Join(got, " | "), tc.want)
		}
	}
}

// A phrase after a list item attaches by its preposition (W-M1.28): an
// "of" phrase is the item noun's complement and stays in the item. Before
// the rule the clause went on with it after the list's last item, and
// "Fix the parser, the lexer, and the handling of empty files" targeted
// the parser of empty files. A destination after the list is still the
// verb's, and an item with a complement is an item in any place.
func TestClauses_AnOfPhraseStaysWithItsListItem(t *testing.T) {
	for _, tc := range []struct{ prompt, dest string }{
		{"Fix the parser, the lexer, and the handling of empty files.", ""},
		{"Fix the parser, the handling of empty files, and the lexer.", ""},
		{"Replace the parser, the lexer and the handling of empty files with the line scanner.", "line scanner"},
	} {
		f := Parse(tc.prompt)
		if len(f.Targets) != 1 || words(f.Targets[0]) != tokensOf("parser") {
			t.Errorf("%q: targets %v, want [parser]", tc.prompt, f.Targets)
		}
		if d, ok := destination(f); ok != (tc.dest != "") || ok && words(d) != tokensOf(tc.dest) {
			t.Errorf("%q: destination %v (%v), want [%s]", tc.prompt, d, ok, tc.dest)
		}
		if cl := clausesOf(strings.TrimSuffix(tc.prompt, ".")); len(cl) != 1 {
			t.Errorf("%q: clauses %q, want one", tc.prompt, cl)
		}
	}
}

// A clause goes on after a list's last item only with a phrase its verb
// takes (W-M1.28): a place, a use, an exclusion or a destination. Any
// other word after the last item is the item's own or opens a clause of
// its own, so the list does not form and its commas end clauses as before
// the list rule. Before the rule "dial timeouts, the retry budget, and
// drain before close" read as the one clause "dial timeouts before
// close", and the verb word ending "the pool size log" went onto the
// list's first item.
func TestClauses_AListGoesOnOnlyWithAPhraseItsVerbTakes(t *testing.T) {
	for _, tc := range []struct{ sentence, want string }{
		{"dial timeouts, the retry budget, and drain before close", "dial timeouts | the retry budget | drain before close"},
		{"the dial timeout, the retry budget and the pool size log", "the dial timeout | the retry budget and the pool size log"},
		{"Fix the parser, the lexer and the scanner in the report package", "Fix the parser in the report package"},
		{"Fix the parser, the lexer and the scanner without changing the log format", "Fix the parser without changing the log format"},
		{"Replace the parser, the lexer and the scanner with the line reader", "Replace the parser with the line reader"},
	} {
		var got []string
		for _, cl := range clausesOf(tc.sentence) {
			got = append(got, strings.Join(cl, " "))
		}
		if strings.Join(got, " | ") != tc.want {
			t.Errorf("%q: clauses %q, want %q", tc.sentence, strings.Join(got, " | "), tc.want)
		}
	}
}
