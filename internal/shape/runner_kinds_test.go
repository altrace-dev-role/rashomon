package shape

import (
	"slices"
	"strings"
	"testing"
)

// c8LintLines, c8TypecheckLines and c8ScanLines are lines whose first runner
// is on the lint, typecheck or scan list: each carries a runner digest, the
// runner_kind of its list and the row's own words as runner_name. A literal
// word each, not the constants, so the vocabulary is held to what the schema
// says.
var c8KindLines = []struct{ cmd, kind, name string }{
	{"golangci-lint run ./...", "lint", "golangci-lint run"},
	{"go vet ./...", "lint", "go vet"},
	{"staticcheck ./...", "lint", "staticcheck"},
	{"ruff check .", "lint", "ruff check"},
	{"flake8 src", "lint", "flake8"},
	{"pylint pkg", "lint", "pylint"},
	{"eslint .", "lint", "eslint"},
	{"npx eslint src", "lint", "npx eslint"},
	{"npx --yes eslint src", "lint", "npx eslint"},
	{"cargo clippy --all-targets", "lint", "cargo clippy"},
	{"cargo +nightly clippy", "lint", "cargo clippy"},
	{"rubocop", "lint", "rubocop"},

	{"mypy src", "typecheck", "mypy"},
	{"python3 -m mypy src", "typecheck", "python3 -m mypy"},
	{"pyright", "typecheck", "pyright"},
	{"tsc --noEmit", "typecheck", "tsc --noEmit"},
	{"npx tsc --noEmit", "typecheck", "npx tsc --noEmit"},
	{"cargo check --workspace", "typecheck", "cargo check"},

	{"semgrep --config auto .", "scan", "semgrep"},
	{"bandit -r src", "scan", "bandit"},
	{"gosec ./...", "scan", "gosec"},
	{"govulncheck ./...", "scan", "govulncheck"},
	{"gitleaks detect", "scan", "gitleaks detect"},
	{"pip-audit", "scan", "pip-audit"},
	{"npm audit", "scan", "npm audit"},
	{"cargo audit", "scan", "cargo audit"},

	// A bare tsc emits, so it stays a build; only --noEmit right after it is a check.
	{"tsc", "build", "tsc"},
	{"tsc -p . --noEmit", "build", "tsc"},
	{"npm run lint", "build", "npm run"},
}

// TestC8_RunnerKindIsTheListsWord: a lint, typecheck or scan runner carries
// its own kind, whether its line returns its status or a pipe hides it.
func TestC8_RunnerKindIsTheListsWord(t *testing.T) {
	for _, tc := range c8KindLines {
		for _, suffix := range []string{"", " 2>&1 | tail -20"} {
			cmd := tc.cmd + suffix
			t.Run(cmd, func(t *testing.T) {
				sh := verbOf(t, cmd)
				if sh.RunnerDigest == nil || sh.RunnerKind == nil || *sh.RunnerKind != tc.kind {
					t.Fatalf("digest set %t kind %s, want kind %s", sh.RunnerDigest != nil, orNil(sh.RunnerKind), tc.kind)
				}
				if sh.RunnerName == nil || *sh.RunnerName != tc.name {
					t.Errorf("runner_name %s, want %q", orNil(sh.RunnerName), tc.name)
				}
				want := MaskedNone
				if suffix != "" {
					want = MaskedBuild
				}
				if sh.StatusMasked == nil || *sh.StatusMasked != want {
					t.Errorf("status_masked %s, want %s: a check runner hides its status as a build's does", orNil(sh.StatusMasked), want)
				}
				if sh.VerbClass == VerbTest {
					t.Errorf("verb_class test: only the test list makes a test run")
				}
			})
		}
	}
}

// TestC8_NotARunIsNoRunner: help and version output is no lint, check or
// scan, so it is no gate run.
func TestC8_NotARunIsNoRunner(t *testing.T) {
	for _, cmd := range []string{
		"ruff --version", "eslint --help", "mypy --version", "golangci-lint --help", "semgrep -h",
		"go vet -n ./...", "cargo clippy --version", "npm audit --help",
	} {
		if sh := verbOf(t, cmd); sh.RunnerDigest != nil || sh.RunnerKind != nil {
			t.Errorf("%q: runner digest set %t kind %s, want none", cmd, sh.RunnerDigest != nil, orNil(sh.RunnerKind))
		}
	}
}

// TestC8_TheRunnerListsAreDisjoint: a row is on one list only, so a runner's
// kind and its name never depend on the order the lists are read in.
func TestC8_TheRunnerListsAreDisjoint(t *testing.T) {
	seen := map[string]bool{}
	for _, l := range runnerLists {
		for _, r := range l.rows {
			w := strings.Join(r, " ")
			if seen[w] {
				t.Errorf("row %q is on two lists", w)
			}
			seen[w] = true
		}
	}
}

// TestC8_RunnerKindsAreTheFiveLists: the vocabulary, in the lists' order, for
// the schema enum test.
func TestC8_RunnerKindsAreTheFiveLists(t *testing.T) {
	if got, want := RunnerKinds(), []string{"test", "build", "lint", "typecheck", "scan"}; !slices.Equal(got, want) {
		t.Fatalf("RunnerKinds() = %q, want %q", got, want)
	}
}

// TestC8_RunnerNamesAreEveryRow: RunnerNames is every row of every list.
func TestC8_RunnerNamesAreEveryRow(t *testing.T) {
	n := 0
	for _, l := range runnerLists {
		n += len(l.rows)
		for _, r := range l.rows {
			if !slices.Contains(RunnerNames(), strings.Join(r, " ")) {
				t.Errorf("RunnerNames lacks %q", strings.Join(r, " "))
			}
		}
	}
	if got := len(RunnerNames()); got != n {
		t.Errorf("RunnerNames has %d words for %d rows", got, n)
	}
}
