package verify

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// claimKey is the install key the claim tests HMAC under.
var claimKey = []byte("install-key-for-claims")

// allKinds is one required green gate of each kind a class maps to, so a
// VER55 row's verdict never decides whether it matched.
func allKinds() []GateState {
	var gs []GateState
	for _, k := range []string{"test", "build", "lint", "typecheck", "scan"} {
		gs = append(gs, GateState{Gate: "learned:" + k + ":r:x@d:repo", Kind: k, Family: "r:x@d:repo",
			Required: true, State: Green, LastGreen: &RunRef{Seq: 1}})
	}
	return gs
}

// claimsOf is Claims over one window with allKinds, a relevant write unless
// dry, and nothing delivered.
func claimsOf(msg string, dry bool) ClaimScan {
	return Claims([]byte(msg), nil, claimKey, allKinds(), !dry, SourceFinalMessage, nil)
}

// classes is a scan's verdicts as class/strict, in output order.
func classes(s ClaimScan) []string {
	out := []string{}
	for _, c := range s.Claims {
		out = append(out, fmt.Sprintf("%s/%t", c.Class, c.Strict))
	}
	return out
}

// ver55Rows is the extraction table (20.2.5 steps 2 to 5 and 8): the
// message, the one class it claims or none, its strictness, whether the
// window holds no relevant write, and why. The fuzz target seeds from it.
var ver55Rows = []struct {
	msg    string
	class  ClaimClass
	strict bool
	dry    bool
	why    string
}{
	// The spec's named rows.
	{msg: "password", why: "whole words: pass is inside password"},
	{msg: "bypass", why: "whole words"},
	{msg: "greenfield", why: "whole words"},
	{msg: "The tests should pass.", why: "modal"},
	{msg: "run go test to confirm", why: "skipped: Run"},
	{msg: "The tests don't pass yet.", why: "n't, and yet after"},
	{msg: "All 42 tests pass.", class: ClaimTestsPass, strict: true},
	{msg: "It builds.", class: ClaimBuildOK, strict: true},
	{msg: "Lint is clean.", class: ClaimLintClean, strict: true},

	// The card's named rows.
	{msg: "The tests pass, but two are skipped.", class: ClaimTestsPass, why: "skipped in the sentence"},
	{msg: "Tests passed. The build failed.", class: ClaimTestsPass, why: "failed in the next sentence"},
	{msg: "I haven't run the tests but they should pass.", why: "modal; haven't before but"},
	{msg: "Done.", class: ClaimDone, strict: true},
	{msg: "Done.", dry: true, why: "done needs a relevant write"},
	{msg: "> rashomon: tests pass", why: "quote line"},
	{msg: "`go test` passes", why: "the code span leaves no subject"},
	{msg: "See https://x/tests-pass", why: "URL"},
	{msg: "Neither the build nor the tests pass.", why: "neither, nor"},
	{msg: "No regressions, all tests pass.", class: ClaimTestsPass, strict: true},
	{msg: "Fixed and verified.", class: ClaimDone, strict: true},
	{msg: "42/42 green", class: ClaimTestsPass, strict: true},
	{msg: "golangci-lint reports 0 issues.", class: ClaimLintClean, strict: true},
	{msg: "0 issues from golangci-lint", why: "the subject follows the predicate (Q4)"},
	{msg: "tsc reports no errors.", class: ClaimTypecheckClean, strict: true},
	{msg: "semgrep is clean.", class: ClaimScanClean, strict: true},
	{msg: "Not yet done.", why: "not before"},
	{msg: "It will be done once CI passes.", why: "will; once"},

	// Two strict positives per class.
	{msg: "The test suite passes.", class: ClaimTestsPass, strict: true},
	{msg: "All specs passed.", class: ClaimTestsPass, strict: true},
	{msg: "The project compiles cleanly.", class: ClaimBuildOK, strict: true},
	{msg: "The binary builds successfully.", class: ClaimBuildOK, strict: true},
	{msg: "The linter passed.", class: ClaimLintClean, strict: true},
	{msg: "eslint reports no warnings.", class: ClaimLintClean, strict: true},
	{msg: "Typecheck passes.", class: ClaimTypecheckClean, strict: true},
	{msg: "mypy is clean.", class: ClaimTypecheckClean, strict: true},
	{msg: "The scanner found no findings.", class: ClaimScanClean, strict: true},
	{msg: "gosec passes.", class: ClaimScanClean, strict: true},
	{msg: "The fix is complete.", class: ClaimDone, strict: true},
	{msg: "Implemented the parser.", class: ClaimDone, strict: true},
	{msg: "The type check passes.", class: ClaimTypecheckClean, strict: true, why: "Q9: the subject's own check is no modal"},

	// The windows' edges and the clause rules.
	{msg: "Tests on the branch pass.", class: ClaimTestsPass, strict: true, why: "the subject four words before"},
	{msg: "Tests on the main branch pass.", why: "the subject five words before"},
	{msg: "Tests ran on every platform and all pass.", why: "the subject far before"},
	{msg: "It should ship: all tests pass.", class: ClaimTestsPass, strict: true, why: "the modal four words before"},
	{msg: "Tests pass on main yet again.", class: ClaimTestsPass, strict: true, why: "yet three words after"},
	{msg: "Nothing changed and all tests pass.", class: ClaimTestsPass, strict: true, why: "and ends the negation"},
	{msg: "No new code but the tests pass.", class: ClaimTestsPass, strict: true, why: "but ends the negation"},
	{msg: "All tests pass, and the feature is complete.", class: ClaimTestsPass, strict: true, why: "done outside the first clause"},

	// Negated.
	{msg: "The tests do not pass.", why: "not"},
	{msg: "Tests didn't pass.", why: "n't"},
	{msg: "None of the tests pass.", why: "none"},
	{msg: "The build never succeeded.", why: "never"},
	{msg: "The build hasn't succeeded.", why: "n't"},
	{msg: "Lint is not clean.", why: "not"},
	{msg: "No tests passed.", why: "no"},
	{msg: "Nothing in the suite passes.", why: "nothing"},
	{msg: "The scan is not clean.", why: "not"},
	{msg: "mypy cannot pass.", why: "cannot"},
	{msg: "It's not done.", why: "not"},
	{msg: "Not all tests pass.", why: "not"},
	{msg: "I'm not sure the tests pass.", why: "not, earlier in the clause"},
	{msg: "Neither lint nor typecheck passed.", why: "neither, nor"},

	// Modal, future or conditional within three words before.
	{msg: "The code would compile cleanly.", why: "would"},
	{msg: "If the tests pass, merge it.", why: "if"},
	{msg: "Tests will pass after the merge.", why: "will"},
	{msg: "Tests might pass.", why: "might"},
	{msg: "The linter may be clean.", why: "may"},
	{msg: "Hopefully the build succeeds.", why: "hopefully"},
	{msg: "The tests need to pass.", why: "need, to"},
	{msg: "We expect lint to be clean.", why: "to"},
	{msg: "Once the build succeeds, deploy.", why: "once"},
	{msg: "The scan will likely be clean.", why: "will, likely"},
	{msg: "Typecheck should pass after this.", why: "should"},
	{msg: "It could build cleanly.", why: "could"},

	// yet within two words after.
	{msg: "Tests pass yet fail on CI.", why: "yet"},
	{msg: "Lint passes, yet two rules are off.", why: "yet"},
	{msg: "The typecheck is clean yet.", why: "yet"},
	{msg: "Everything compiles yet nothing runs.", why: "yet"},
	{msg: "Not done yet.", why: "not; yet"},

	// Disclosed, so loose.
	{msg: "All tests pass, but one is skipped.", class: ClaimTestsPass, why: "skipped"},
	{msg: "The build succeeded with one warning.", class: ClaimBuildOK, why: "warning"},
	{msg: "Lint is clean. Typecheck has errors.", class: ClaimLintClean, why: "errors, next sentence"},
	{msg: "Tests pass; the integration suite is still running.", class: ClaimTestsPass, why: "still running"},
	{msg: "The build passes, but e2e tests are pending.", class: ClaimBuildOK, why: "pending"},
	{msg: "Unit tests pass. Integration tests were not run.", class: ClaimTestsPass, why: "not run, next sentence"},
	{msg: "The scan is clean, though the job timed out.", class: ClaimScanClean, why: "timed out"},
	{msg: "Tests passed in the background.", class: ClaimTestsPass, why: "in the background"},
	{msg: "Typecheck passes; mypy exited with code 1.", class: ClaimTypecheckClean, why: "exited"},
	{msg: "The broker tests pass.", class: ClaimTestsPass, why: "Q7's accepted noise: broker opens with broke"},

	// Skipped sentences.
	{msg: "Do the tests pass?", why: "a question"},
	{msg: "Please make sure the tests pass.", why: "Please"},
	{msg: "Run the tests: all tests pass.", why: "Run"},
	{msg: "Try the build, it compiles.", why: "Try"},
	{msg: "You can check that lint is clean.", why: "You can"},
	{msg: "You may see the tests pass.", why: "You may"},

	// Stripped before splitting.
	{msg: "> All tests pass.", why: "quote line"},
	{msg: "```\nall tests pass\n```", why: "fenced block"},
	{msg: "```go\nall tests pass", why: "a fence runs to the window's end"},
	{msg: "Ran `tests pass` locally.", why: "inline code"},
	{msg: "He wrote \"the tests pass\" in the log.", why: "double-quoted span"},
	{msg: "Logs at https://ci.example/r?tests=pass", why: "URL"},
	{msg: "All te\u200bsts pass.", class: ClaimTestsPass, strict: true, why: "step 1 drops a rune neither graphic nor space"},
}

func TestVER55_ClaimExtractionTable(t *testing.T) {
	if len(ver55Rows) < 60 {
		t.Fatalf("%d rows, want at least 60 (20.2.5)", len(ver55Rows))
	}
	for _, r := range ver55Rows {
		want := []string{}
		if r.class != "" {
			want = []string{fmt.Sprintf("%s/%t", r.class, r.strict)}
		}
		if got := classes(claimsOf(r.msg, r.dry)); !slices.Equal(got, want) {
			t.Errorf("%q: claims %v, want %v (%s)", r.msg, got, want, r.why)
		}
	}

	// Step 1's ASCII path keeps what unicode's two tests keep.
	for c := range utf8.RuneSelf {
		r := rune(c)
		if got, want := len(sanitize([]byte{byte(c)})) == 1, unicode.IsGraphic(r) || unicode.IsSpace(r); got != want {
			t.Errorf("sanitize keeps %q: %t, want %t", r, got, want)
		}
	}

	// A double-quoted span is removed up to maxQuotedRunes runes and no further.
	for _, n := range []int{maxQuotedRunes, maxQuotedRunes + 1} {
		quoted := "all tests pass " + strings.Repeat("x", n-len("all tests pass "))
		msg := "He wrote \"" + quoted + "\" here."
		want := []string{}
		if n > maxQuotedRunes {
			want = []string{"tests_pass/true"}
		}
		if got := classes(claimsOf(msg, false)); !slices.Equal(got, want) {
			t.Errorf("a quoted span of %d runes: claims %v, want %v", n, got, want)
		}
	}
}

// TestVER55b_TablesAreWrittenAsWordsSplitsThem: every table entry is the
// word sequence words gives for it, so matching compares like with like and
// nothing is split at init; the lexicon is in ClaimClasses order.
func TestVER55b_TablesAreWrittenAsWordsSplitsThem(t *testing.T) {
	var phrases [][]string
	for i, r := range claimLexicon {
		if r.class != ClaimClasses[i] {
			t.Fatalf("lexicon row %d is %s, want %s: output order follows ClaimClasses", i, r.class, ClaimClasses[i])
		}
		phrases = append(append(phrases, r.subjects...), r.predicates...)
	}
	phrases = append(append(phrases, claimSkips[:]...), claimDisclosures[:]...)
	for _, w := range claimNegators {
		phrases = append(phrases, []string{w})
	}
	for _, w := range claimModals {
		phrases = append(phrases, []string{w})
	}
	for _, p := range phrases {
		if got := words(strings.Join(p, " ")); !slices.Equal(got, p) {
			t.Errorf("entry %q splits as %q", p, got)
		}
	}
}
