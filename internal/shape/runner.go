// This file decides which runner row a line's first command invokes, and
// which flags may stand before it: the test runners (testCommands), the flags
// that may stand before a row's words (leadingFlags), and the matcher the
// verb class and the masked parser both read them through (runnerOn).

package shape

import "strings"

// testCommands are the test runners the test verb class recognises: the
// program, as path.Base names it, and then the plain words that must follow
// it, past the flags leadingFlags lets stand before them (runnerOn). A
// runner that is its own program is a list of one.
//
// A fixed list, compared and discarded. Nothing from the line is kept but the
// class; an argument is looked at only to be told equal to one of these.
// Anything this list does not name -- `make -C dir test`, `npx mocha`,
// `go test` behind `nice` -- stays what it was before: execute, or package
// for go and cargo. That is an under-claim, and the detections built on this
// class are worth only as much as their refusal to over-claim.
//
// The wrappers on the list are the ones whose exit status is the runner's:
// a project's own gradlew and mvnw, npm's `t` alias, `npx jest` and `npx
// vitest`, `uv run pytest`, `poetry run pytest` and `bundle exec rspec`. So
// are the `timeout N` and `time` prefixes (runnerPrefix), except for
// timeout's own 124, which the report reads as no result. A pipe or a list
// after the runner is still refused (wholeCommand): without pipefail its
// status is the last stage's.
//
// The rule applied: a runner is on the list when it is a known test tool, a
// build tool or launcher given its test command (`go test`, `npm t`,
// `npm run test`, `python -m pytest`, `python -m unittest`), or a listed
// wrapper that passes its runner's exit status through. `make check` is not,
// since check is not make's test command. That rule does not keep lint out:
// go test runs vet first, an npm `pretest` script runs before `npm test`,
// tox's default envlist and a make `test` target can each include lint, and
// a lint failure fixed only in a file named like a test then reads as the
// tests-only pattern.
var testCommands = [][]string{
	{"pytest"}, {"jest"}, {"vitest"}, {"mocha"}, {"rspec"}, {"phpunit"},
	{"ctest"}, {"tox"}, {"nox"},

	{"go", "test"}, {"cargo", "test"}, {"npm", "test"}, {"npm", "t"}, {"npm", "run", "test"},
	{"yarn", "test"}, {"pnpm", "test"}, {"bun", "test"}, {"dotnet", "test"},
	{"mvn", "test"}, {"mvnw", "test"}, {"gradle", "test"}, {"gradlew", "test"}, {"make", "test"},
	{"python", "-m", "pytest"}, {"python3", "-m", "pytest"},
	{"python", "-m", "unittest"}, {"python3", "-m", "unittest"},

	{"npx", "jest"}, {"npx", "vitest"}, {"uv", "run", "pytest"}, {"poetry", "run", "pytest"},
	{"bundle", "exec", "rspec"},
	{"rake", "test"}, {"composer", "test"}, {"mix", "test"}, {"bazel", "test"},
}

// leadingFlags are, per program, the flags that may stand between the program
// and a row's first word (runnerOn). A flag is listed only when the program's
// own reference shows all three: it takes no separate value (an attached one
// is matched by an entry ending in *, a prefix); it changes neither the
// directory nor the file the runner reads; and it changes neither whether the
// run happens nor how its result reaches the exit status. No entry may be on
// notARun or notARunAny (TestLeadingFlagsAreNotRefusals): a listed flag is
// passed over, not read by refusesRun, so a listed refusal would make a line
// that runs no tests read as one that does. An unlisted token stops the
// search, and the line reads as it would with nothing listed: an
// under-claim, which a later entry moves (a table change, and a RulesVersion
// bump). An exact entry written `flag=value` is unlisted (`mvn -B=true
// test`); a prefix entry admits whatever follows it, so `make -j=4 test` and
// `make -j4x test` are admitted, and make rejects both with exit 2 before
// any recipe. An alias is the same program and shares its slice, as notARun's
// pytestNotARun is shared: python3 python's, mvnw mvn's, gradlew gradle's.
//
//   - make -j*: the job count, attached or left out (GNU make's --help: `-j
//     [N], --jobs[=N]`). A separate count, `make -j 4 test`, is read by make
//     and stops the search at the 4.
//   - mvn -B, -q: batch mode and quiet output (Maven's CLIManager).
//   - cargo +*: the toolchain rustup's proxy runs, `cargo +nightly test`.
//     The citation is rustup's; a cargo installed without rustup rejects
//     every toolchain word as an unknown command (exit 101, Homebrew's cargo
//     measured 10-24), so there `cargo +nightly test` is the same false red
//     as `cargo +nonesuch test`, a red that never greens.
//   - gradle --no-daemon: the build runs without the daemon (Gradle's
//     command-line reference).
//   - python -u: unbuffered output (python3 --help).
//   - npx --yes, -y: install the package without asking (npm's config
//     reference, yes).
//   - bazel --output_base=*: where the outputs are written, not the
//     workspace read (bazel's command-line reference, startup options).
//     --bazelrc is not listed: an rc file holds options of every kind, the
//     ones that decide a test's verdict among them.
var leadingFlags = map[string][]string{
	"make":    {"-j*"},
	"mvn":     mavenLeadingFlags,
	"mvnw":    mavenLeadingFlags,
	"cargo":   {"+*"},
	"gradle":  gradleLeadingFlags,
	"gradlew": gradleLeadingFlags,
	"python":  pythonLeadingFlags,
	"python3": pythonLeadingFlags,
	"npx":     {"--yes", "-y"},
	"bazel":   {"--output_base=*"},
}

var (
	mavenLeadingFlags  = []string{"-B", "-q"}
	gradleLeadingFlags = []string{"--no-daemon"}
	pythonLeadingFlags = []string{"-u"}
)

// runnerOn reports whether the command whose program, past runnerPrefix, is
// the token at i, named prog, and which ends before toks[end], is a row of
// list -- testCommands, or buildCommands -- and is not refused by notARun:
// the comparison runsTests makes, over one command's words rather than the
// whole line's, so that a `-v` given to the grep after a pipe is not read as
// make's. The verb class and the masked parser both read a runner through
// it, so the two cannot disagree on where its words start.
//
// A row's words are looked for at the token after the program, and then
// past each of prog's leadingFlags in front of them: `make -j4 test` is
// `make test`'s row. Any other token stops the search, and the row is not
// matched. A one-word row has no words to look for, so it is matched at the
// token after the program, with nothing passed over.
func runnerOn(toks []token, i int, prog string, end int, list [][]string) bool {
next:
	for _, c := range list {
		if c[0] != prog {
			continue
		}
		from := i + 1
		for !rowAt(toks, from, end, c) {
			if from >= end || !leadingFlag(toks, from, prog) {
				continue next
			}
			from++
		}
		return !refusesRun(refusalsOf(c), toks[:i], toks[from+len(c)-1:end])
	}
	return false
}

// rowAt reports whether row c's words after its program are the tokens from
// toks[from] on, each a plain word of the command, which ends before
// toks[end].
func rowAt(toks []token, from, end int, c []string) bool {
	for n, want := range c[1:] {
		k := from + n
		if k >= end {
			return false
		}
		t := toks[k]
		if t.quotedAt >= 0 || t.text != want || runsOn(toks, k) {
			return false
		}
	}
	return true
}

// leadingFlag reports whether the token at k is one of prog's leadingFlags,
// written as a plain word whose text the shell does not change: unquoted,
// not running into a process substitution, and with no expansion, glob or
// escape in it, any of which may make it more words than one, or none.
func leadingFlag(toks []token, k int, prog string) bool {
	t := toks[k]
	if t.meta || t.quotedAt >= 0 || t.opaque || t.ticks > 0 || runsOn(toks, k) ||
		strings.ContainsAny(t.text, "$`*?[{\\") {
		return false
	}
	for _, f := range leadingFlags[prog] {
		if p, ok := strings.CutSuffix(f, "*"); ok {
			if strings.HasPrefix(t.text, p) {
				return true
			}
		} else if t.text == f {
			return true
		}
	}
	return false
}
