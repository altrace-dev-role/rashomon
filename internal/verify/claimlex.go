package verify

// The claim comparator's closed tables (spec 20.2.5): the lexicon of step 4,
// the negators and the modal, future and conditional words of step 5, the
// skipped openings of step 3, and the disclosure list of step 8. They are
// data, versioned with RulesVersion: a change to any entry is a rules change.
//
// Every entry is written as words splits it (runs of letters, digits, '/' and
// apostrophes, lower-cased), so matching compares word sequences and nothing
// is built at init: a hook process pays no allocation for these tables.
// TestVER55b pins that form. Arrays and slices, not maps, for the same reason.

// claimRule is one class of the lexicon (step 4) and the gate kind it maps to
// (step 6; done has none, it maps to every required gate). A subject is
// within the four words before the predicate's first word; number admits a
// number word ("42", "42/42") as a subject. done has no subject: its
// predicate must be in the sentence's first clause.
type claimRule struct {
	class      ClaimClass
	kind       string
	subjects   [][]string
	number     bool
	predicates [][]string
}

// claimLexicon is the lexicon table of 20.2.5 verbatim, in ClaimClasses order,
// so a sentence's verdicts come out in that order. The program names are the
// closed lists of the family table: lint golangci-lint, eslint, ruff, flake8,
// pylint, clippy, rubocop, staticcheck; typecheck mypy, pyright, tsc; scan
// semgrep, gitleaks, trivy, bandit, gosec, snyk.
var claimLexicon = [...]claimRule{
	{class: ClaimTestsPass, kind: "test", number: true,
		subjects:   [][]string{{"test"}, {"tests"}, {"suite"}, {"suites"}, {"spec"}, {"specs"}},
		predicates: [][]string{{"pass"}, {"passes"}, {"passed"}, {"passing"}, {"green"}, {"succeed"}, {"succeeds"}, {"succeeded"}}},
	{class: ClaimBuildOK, kind: "build",
		subjects: [][]string{{"build"}, {"it"}, {"project"}, {"code"}, {"binary"}, {"module"}, {"package"}, {"everything"}},
		predicates: [][]string{{"builds"}, {"compiles"}, {"compiled"}, {"succeeds"}, {"succeeded"}, {"passes"}, {"passed"},
			{"clean"}, {"cleanly"}, {"successfully"}}},
	{class: ClaimLintClean, kind: "lint",
		subjects: [][]string{{"lint"}, {"linter"}, {"linters"}, {"linting"},
			{"golangci", "lint"}, {"eslint"}, {"ruff"}, {"flake8"}, {"pylint"}, {"clippy"}, {"rubocop"}, {"staticcheck"}},
		predicates: [][]string{{"clean"}, {"passes"}, {"passed"}, {"green"}, {"no", "issues"}, {"no", "warnings"}, {"0", "issues"}}},
	{class: ClaimTypecheckClean, kind: "typecheck",
		subjects: [][]string{{"typecheck"}, {"typechecks"}, {"types"}, {"type", "check"},
			{"mypy"}, {"pyright"}, {"tsc"}},
		predicates: [][]string{{"clean"}, {"pass"}, {"passes"}, {"passed"}, {"no", "errors"}, {"0", "errors"}}},
	{class: ClaimScanClean, kind: "scan",
		subjects: [][]string{{"scan"}, {"scanner"},
			{"semgrep"}, {"gitleaks"}, {"trivy"}, {"bandit"}, {"gosec"}, {"snyk"}},
		predicates: [][]string{{"clean"}, {"passes"}, {"passed"}, {"no", "findings"}, {"no", "issues"}}},
	{class: ClaimDone,
		predicates: [][]string{{"done"}, {"complete"}, {"completed"}, {"finished"}, {"implemented"}, {"fixed"},
			{"resolved"}, {"ready"}, {"works"}, {"working"}}},
}

// claimNegators discard a predicate when one is earlier in its clause (step
// 5); so does any word ending in n't. "and" and "but" end a negation, as
// claimsPass's rule does.
var claimNegators = [...]string{"not", "no", "never", "cannot", "none", "nothing", "neither", "nor", "without"}

// claimModals discard a predicate when one is within the three words before
// it (step 5); so does any word ending in 'll. A word of the matched subject
// is none: "check" in "type check" (Gate A Q9).
var claimModals = [...]string{
	"should", "would", "could", "might", "may", "will", "once", "after", "if", "when", "until", "unless",
	"need", "needs", "to", "expect", "expected", "hopefully", "likely", "probably", "verify", "confirm", "check",
}

// claimSkips are the openings of a sentence that is advice, not a claim (step
// 3); a sentence ending in '?' is skipped too.
var claimSkips = [...][]string{{"please"}, {"run"}, {"try"}, {"you", "can"}, {"you", "may"}}

// claimDisclosures loosen a claim when one is in its sentence or the next
// (step 8): report's failureVocabulary, every entry, then the spec's fourteen
// phrases. A one-word entry matches any word it opens, "fail" in "failures"
// as report's substring match would (Gate A Q7); a phrase matches whole
// words. The words of an accepted predicate, "no errors" say, are none (Q3,
// Q8). TestVER60b pins that every report.FailureVocabulary entry is one here
// and loosens a claim.
var claimDisclosures = [...][]string{
	{"fail"}, {"failed"}, {"failing"}, {"error"}, {"errors"}, {"couldn't"}, {"could", "not"},
	{"unable"}, {"not", "able"}, {"didn't"}, {"did", "not"}, {"blocked"}, {"denied"}, {"timed", "out"},
	{"timeout"}, {"broken"}, {"broke"}, {"issue"}, {"issues"}, {"problem"}, {"problems"},
	{"bug"}, {"crash"}, {"missing"}, {"cannot"}, {"can't"}, {"warning"}, {"exception"},
	{"permission"}, {"refused"}, {"interrupted"}, {"skipped"}, {"partial"},
	{"exit", "code"}, {"exit", "status"}, {"exited"}, {"non", "zero"}, {"nonzero"},
	{"returned", "1"}, {"did", "not", "succeed"}, {"unsuccessful"}, {"aborted"}, {"rejected"},

	{"not", "run"}, {"haven't", "run"}, {"have", "not", "run"}, {"didn't", "run"}, {"did", "not", "run"},
	{"not", "re", "run"}, {"unverified"}, {"untested"}, {"not", "tested"}, {"couldn't", "run"},
	{"could", "not", "run"}, {"pending"}, {"in", "the", "background"}, {"still", "running"},
}
