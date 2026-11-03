package verify

import (
	"fmt"
	"slices"
	"testing"
)

// Card MISREAD-DONE1: the claim comparator read three kinds of word, and a
// table's header row, as a claim that none of them makes, and the Stop said
// "done was claimed" for it. Each row is a sentence of the L1 corpus word for
// word (the final message of a replay turn whose line was false), or a
// control that must keep its claim. The fix narrows the reading at the word
// and drops the header line; the lexicon's entries do not change.
type md1Row struct {
	msg    string
	class  ClaimClass
	strict bool
	why    string
}

func md1Run(t *testing.T, rows []md1Row) {
	t.Helper()
	for _, r := range rows {
		want := []string{}
		if r.class != "" {
			want = []string{fmt.Sprintf("%s/%t", r.class, r.strict)}
		}
		if got := classes(claimsOf(r.msg, false)); !slices.Equal(got, want) {
			t.Errorf("%q: claims %v, want %v (%s)", r.msg, got, want, r.why)
		}
	}
}

// TestMD1a_ANumberFromAnotherClauseIsNoCount: a number stands as the count
// of a test claim only in the predicate's own clause, and never with '#'
// before it, which makes it an id (a pull request's number). An item number
// or a commit's ordinal a clause back is not one; a word subject still
// reaches across, as Q4 reads it, and past an id.
func TestMD1a_ANumberFromAnotherClauseIsNoCount(t *testing.T) {
	md1Run(t, []md1Row{
		{msg: "That control belonged in the red commit (TEAM-BAR 5), and the green commit message will say it came in late.",
			why: "eng-2 6537: TEAM-BAR's item 5, two clause breaks back; green is the commit's"},
		{msg: "- **452b8395** hook (commit 3, green)", why: "eng-2 4149: commit 3, a clause back"},
		{msg: "**#179, commit 43b56303:** the green run passed (459 s, rc 0).", why: "eng-4 3262: #179, two clauses back"},
		{msg: "- **#170 is green but not merged yet.**", why: "manager 3418: #170 is a pull request, in the predicate's clause"},
		{msg: "I recorded the manager's ruling in `gates/PR-194.txt`: `TestSRS1d4_` is a deliberate pin, so #194 stays GREEN.",
			why: "qa-1 4687: #194 is a pull request, and GREEN its grade"},

		{msg: "Tests: 42 passed.", class: ClaimTestsPass, strict: true, why: "control: the count in the predicate's clause"},
		{msg: "The tests, all 42, pass.", class: ClaimTestsPass, strict: true, why: "control: the word subject reaches across"},
		{msg: "42/42 green", class: ClaimTestsPass, strict: true, why: "control: the count beside its predicate"},
		{msg: "The tests on #42 pass.", class: ClaimTestsPass, strict: true, why: "control: a word subject behind an id"},
		{msg: "Fixed #42.", class: ClaimDone, strict: true, why: "control: done reads no subject, so an id beside it changes nothing"},
	})
}

// TestMD1b_ANameInCapitalsIsNoClaim: a done predicate written in capitals, in
// a sentence that is not all capitals, is a name (a protocol message, a
// verdict's token), not a claim. A sentence in capitals throughout keeps its
// claim, and so does a predicate read with its subject ("20/20 PASS"): done
// reads none, so its word alone can be a name.
func TestMD1b_ANameInCapitalsIsNoClaim(t *testing.T) {
	md1Run(t, []md1Row{
		{msg: "Then green gets committed and I send FIX BRANCH READY to the manager.", why: "eng-2 7545: the message's name"},
		{msg: "The manager gets FIX BRANCH READY for #197 once the re-check passes.", why: "eng-1 7705: the message's name"},
		{msg: "GATE-BIND1 and INSTALL-NOTOOLCHAIN1 get their READY or RED as their checks land.", why: "eng-1 7705: two tokens"},
		{msg: "FIX BRANCH READY has gone to the manager.", why: "eng-2 5617: the message's name"},
		{msg: "I'll put that in the FIX BRANCH READY message.", why: "eng-4 3262: the message's name"},

		{msg: "DONE.", class: ClaimDone, strict: true, why: "control: capitals throughout"},
		{msg: "ALL TESTS PASS.", class: ClaimTestsPass, strict: true, why: "control: capitals throughout"},
		{msg: "It is ready.", class: ClaimDone, strict: true, why: "control: the word in lower case"},
		{msg: "Fixed the parser; CI is green.", class: ClaimDone, strict: true, why: "control: a capital opens the word"},
		{msg: "Ran the named test 20x on PR head (809bd58c) and 20x on main (5472e932) under goslot: 20/20 PASS both sides, " +
			"consistent with the author's own admission that it doesn't reproduce locally.",
			class: ClaimTestsPass, strict: true, why: "control, qa-3 228: the count is PASS's subject"},
		{msg: "Tests PASS.", class: ClaimTestsPass, strict: true, why: "control: a word subject beside a predicate in capitals"},
	})
}

// TestMD1c_HalfOfAHyphenedWordIsNoPredicate: a predicate joined by a hyphen
// to a letter is part of another word. Subjects are not touched, so
// golangci-lint still names the linter, and a hyphen with a space beside it
// joins nothing.
func TestMD1c_HalfOfAHyphenedWordIsNoPredicate(t *testing.T) {
	md1Run(t, []md1Row{
		{msg: "That gives the Windows half of row 9a a ready-made fix.", why: "eng-4 3082: ready-made"},
		{msg: "The half-finished port stays on its branch.", why: "finished after a hyphen"},

		{msg: "golangci-lint is clean.", class: ClaimLintClean, strict: true, why: "control: a hyphened subject"},
		{msg: "Fixed - the parser no longer panics.", class: ClaimDone, strict: true, why: "control: a spaced hyphen"},
		{msg: "The parser is fixed--the panic is gone.", class: ClaimDone, strict: true, why: "control: a double hyphen is a dash"},
	})
}

// TestMD1d_ATableHeaderIsNoClaim: a markdown table's header row, the line
// above its delimiter row, names columns. eng-4's 13:28 message (8727) headed
// its merges table "Conflict resolved", and the Stop read a done claim in it.
// The body's cells are read as any line is, and a bar in prose with no
// delimiter row under it is no table.
func TestMD1d_ATableHeaderIsNoClaim(t *testing.T) {
	md1Run(t, []md1Row{
		{msg: "**Merges** (each message names the sha it merged):\n\n| Branch | Merged tip | Merged in | Conflict resolved |\n|---|---|---|---|\n" +
			"| #214 `fix/consent-scope1` | `32933bde` | main `5472e932` | `disclose.go`: kept main's `spec_event` clause and CS1's v5/`content_sha256` clause |",
			why: "eng-4 8727: the merges table's header"},
		{msg: "| Step | Fixed |\n| :--- | ---: |\n| one | two |", why: "a header over an aligned delimiter row"},

		{msg: "| Card | State |\n|---|---|\n| GATE-INFER1 | done |", class: ClaimDone, strict: true, why: "control: a body cell"},
		{msg: "| Tests | pass |\n| 42 | 42 |", class: ClaimTestsPass, strict: true, why: "control: no delimiter row, no table"},
		{msg: "Tests pass | see the log.\n---", class: ClaimTestsPass, strict: true, why: "control: a rule under prose is no delimiter row"},
		{msg: "| Tests | pass |\n| | |", class: ClaimTestsPass, strict: true, why: "control: empty cells are no delimiter row"},
		{msg: "All tests pass.\n|---|---|", class: ClaimTestsPass, strict: true, why: "control: a line with no bar heads no table"},
	})
}
