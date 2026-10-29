package shape

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func verbOf(t *testing.T, cmd string) Shape {
	t.Helper()
	raw, err := json.Marshal(map[string]string{"command": cmd, "description": "run the tests"})
	if err != nil {
		t.Fatal(err)
	}
	return Derive("Bash", raw, []byte("key"))
}

// TestTestRunnerIsRecognised: a shell call is test when command position holds
// a runner from the fixed list, decided under the same certainty rules the
// program is -- so `cd x && go test ./...` counts, and anything the search
// cannot vouch for stays what it was.
func TestTestRunnerIsRecognised(t *testing.T) {
	// The program a prefixed run records: the prefix, not the runner behind
	// it. The report reads exit status 124 as a timeout that fired only when
	// the program is timeout, so a runner name here turns every fired timeout
	// into a failed run.
	wantProgram := map[string]string{
		"timeout 120 go test ./...":          "timeout",
		"cd /repo && timeout 60s cargo test": "timeout",
		"time -p go test":                    "time",
		"time go test ./...":                 "time",
	}
	visited := map[string]bool{}
	for _, tc := range []struct {
		cmd  string
		want string
		why  string
	}{
		// Runner as program.
		{"pytest", VerbTest, ""},
		{"pytest -x tests/test_login.py", VerbTest, ""},
		{"jest --watchAll=false", VerbTest, ""},
		{"vitest run", VerbTest, ""},
		{"mocha", VerbTest, ""},
		{"rspec spec/models", VerbTest, ""},
		{"phpunit", VerbTest, ""},
		{"ctest --output-on-failure", VerbTest, ""},
		{"tox", VerbTest, ""},
		{"nox", VerbTest, ""},
		{"./node_modules/.bin/jest", VerbTest, "the program is its base name, as it always was"},

		// Program and first plain argument.
		{"go test ./...", VerbTest, ""},
		{"go test", VerbTest, ""},
		{"cargo test --release", VerbTest, ""},
		{"npm test", VerbTest, ""},
		{"npm run test", VerbTest, ""},
		{"yarn test", VerbTest, ""},
		{"pnpm test", VerbTest, ""},
		{"bun test", VerbTest, ""},
		{"dotnet test", VerbTest, ""},
		{"mvn test", VerbTest, ""},
		{"mvn test -DskipTests=false", VerbTest, "=false forces the tests to run"},
		{"mvn test -Dmaven.test.skip=false", VerbTest, ""},
		{"mvn test -DskipTests=true", VerbExecute, "the tests are skipped"},
		{"mvn test -DskipTests=1", VerbTest, "Java's Boolean.valueOf reads only true as true"},
		{"mvn test -DskipTests=TRUE", VerbExecute, "true ignoring case"},
		{"mvn test -Dmaven.test.skip.exec=true", VerbExecute, ""},
		{"go test -list=0 ./...", VerbPackage, "-list takes a pattern, not a boolean"},
		{"go test -list=F ./...", VerbPackage, ""},
		{"tox -e=0", VerbExecute, "-e takes an environment name"},
		{"nox -k=0", VerbExecute, "-k takes a keyword expression"},
		{"gradle test", VerbTest, ""},
		{"make test", VerbTest, ""},
		{"python -m pytest -q", VerbTest, ""},
		{"python3 -m pytest", VerbTest, ""},
		{"/usr/local/go/bin/go test ./pkg", VerbTest, ""},

		// Wrappers and prefixes that pass the runner's exit status through.
		{"./gradlew test", VerbTest, "the program is its base name"},
		{"gradlew test --tests Foo", VerbTest, ""},
		{"./mvnw test", VerbTest, ""},
		{"npm t", VerbTest, "npm's alias for npm test"},
		{"npx jest", VerbTest, ""},
		{"npx vitest run", VerbTest, ""},
		{"uv run pytest -q", VerbTest, ""},
		{"poetry run pytest", VerbTest, ""},
		{"bundle exec rspec spec/models", VerbTest, ""},
		{"timeout 120 go test ./...", VerbTest, "timeout's status is the runner's, unless it fires"},
		{"timeout 2.5m npx jest", VerbTest, ""},
		{"time go test ./...", VerbTest, ""},
		{"time -p pytest", VerbTest, ""},
		{"time -p go test", VerbTest, ""},
		{"timeout 600 time make test", VerbTest, ""},
		{"cd /repo && timeout 60s cargo test", VerbTest, ""},

		// Under #29's rules: a directory change, an assignment, a separator.
		{"cd /repo && go test ./...", VerbTest, "cd is where a command runs, not what it runs"},
		{"cd a; cd b && npm test", VerbTest, ""},
		{"CGO_ENABLED=0 go test ./...", VerbTest, ""},
		{"( cd x && pytest )", VerbTest, ""},
		{"go test ./... 2>&1", VerbTest, "a redirection is part of the runner's command"},
		{"go test ./... > out.txt 2>&1", VerbTest, ""},
		{"go test ./...\n", VerbTest, "a trailing newline starts no command"},

		// A flag leadingFlags lists may stand before the row's words
		// (runnerOn); each row names the entry that admits it.
		{"make -j4 test", VerbTest, "make -j#: the job count, attached"},
		{"mvn -B -q test", VerbTest, "mvn -B, -q: batch mode, quiet output"},
		{"bazel --output_base=/tmp test //...", VerbTest, "bazel --output_base=*: the output location"},
		{"cargo +nightly test", VerbTest, "cargo +*: rustup's toolchain word"},
		{"gradle --no-daemon test", VerbTest, "gradle --no-daemon"},
		{"python -u -m pytest", VerbTest, "python -u: unbuffered output"},
		{"npx --yes jest", VerbTest, "npx --yes: install without asking"},
		{"npx -y jest", VerbTest, "npx -y, --yes's short form"},
		{"python3 -u -m pytest", VerbTest, "python3 shares python's slice"},
		{"./mvnw -B test", VerbTest, "mvnw shares mvn's slice"},
		{"./gradlew --no-daemon test", VerbTest, "gradlew shares gradle's slice"},
		{"python -m pytest", VerbTest, "matched at the first position, before any list is read"},
		{"cmake --build .", VerbExecute, "cmake is a build runner, and --build its row's word"},
		// ACCEPTED GAP: an unlisted flag before the row's words stops the
		// match, though each of these runs the tests (in sub).
		{"make -C sub test", VerbExecute, "-C is not listed"},
		{"go -C sub test ./...", VerbPackage, ""},

		// The runner is not the whole line, so the outcome is another
		// program's: the class stays what the program gave it.
		{"go test ./... 2>&1 | tail -20", VerbPackage, "the status is tail's"},
		{"go test | grep FAIL", VerbPackage, "grep inverts it: exit 1 when the tests passed"},
		{"go test ./... || true", VerbPackage, ""},
		{"go test ./...; echo done", VerbPackage, ""},
		{"go test &", VerbPackage, "the status is the fork's"},
		{"go test ./... && echo PASS", VerbPackage, "a pass there is echo's"},
		{"go test ./...\necho done", VerbPackage, "the next line runs last"},
		{"cd /repo && make test | tee log", VerbExecute, ""},
		{"pytest -q; exit 0", VerbExecute, ""},

		// Not a test run: a neighbouring subcommand.
		{"go build ./...", VerbPackage, ""},
		{"go vet ./...", VerbPackage, ""},
		{"npm install", VerbPackage, ""},
		{"npm run build", VerbPackage, ""},
		{"npm run", VerbPackage, ""},
		{"make", VerbExecute, ""},
		{"make install", VerbExecute, ""},
		{"python -m http.server", VerbExecute, ""},
		{"python -m", VerbExecute, ""},
		{"python tests.py", VerbExecute, ""},
		{"cargo testx", VerbPackage, "equal to the list, not a prefix of it"},
		{"go testing", VerbPackage, ""},

		// Uncertain: the argument is not a plain word of this command.
		{`go "test" ./...`, VerbPackage, "quoted: a quote is where plain stops"},
		{`go 'test'`, VerbPackage, ""},
		{`go te\st`, VerbPackage, "an escaped byte is a quoted one"},
		{"go $SUB ./...", VerbPackage, ""},
		{"go $(echo test)", VerbPackage, ""},
		{"go\ntest", VerbPackage, "the next line is the next command"},
		{"go; test", VerbPackage, ""},
		{"go && test -f x", VerbPackage, ""},
		{"go test<(echo x)", VerbPackage, "the shell reads test/dev/fd/63 as one word"},
		{"npm run\ntest", VerbPackage, ""},
		{"python -m 'pytest'", VerbExecute, ""},
		{`go $"test"`, VerbPackage, "a locale-quoted word keeps its $ in the text"},
		{"go `echo test`", VerbPackage, ""},
		{`go ${x}test`, VerbPackage, ""},
		{`go test\`, VerbPackage, "the shell keeps the trailing backslash the tokenizer dropped"},
		{`go test ./... "unterminated`, VerbPackage, "a line the lexer could not finish is not vouched for"},
		{"make -C dir test", VerbExecute, "the first plain argument is -C: under-claim, never guess"},
		{"timeout -k 5 60 go test ./...", VerbExecute, "an option changes timeout's status: refused, not read"},
		{"timeout 60", VerbExecute, ""},
		{"timeout 1m5 go test", VerbExecute, "not a duration"},
		{"time timeout 60 go test", VerbExecute, "timeout only as the outer prefix"},
		{"time", VerbExecute, ""},
		{"npx mocha", VerbPackage, "only jest and vitest behind npx"},
		{"uv run python -m pytest", VerbPackage, ""},
		{"bundle exec rake test", VerbPackage, ""},
		{"./gradlew build", VerbExecute, ""},
		{"timeout 120 go test ./... 2>&1 | tail -20", VerbExecute, "a prefix does not lift the pipe refusal"},
		{"npx jest | tee out", VerbPackage, ""},
		{"echo go test", VerbExecute, "an argument that looks like a runner is an argument"},
		{"cd /x || go test", VerbExecute, "only && and ; are followed past cd; the program is cd"},
		{"$GO test", VerbUnknown + "-null", "the program is whatever the variable holds"},
		{"test -f go.mod", VerbExecute, "the test builtin is not a test runner"},
	} {
		got := verbOf(t, tc.cmd)
		if prog, ok := wantProgram[tc.cmd]; ok {
			visited[tc.cmd] = true
			if got.Program == nil || *got.Program != prog {
				t.Errorf("%q: program %s, want %q: the report reads a fired timeout's 124 by it", tc.cmd, programOf(got), prog)
			}
		}
		want := tc.want
		if want == VerbUnknown+"-null" {
			if got.Program != nil || got.VerbClass != VerbExecute {
				t.Errorf("%q: program %s, class %q; want null and execute%s", tc.cmd, programOf(got), got.VerbClass, because(tc.why))
			}
			continue
		}
		if got.VerbClass != want {
			t.Errorf("%q: verb class %q, want %q%s", tc.cmd, got.VerbClass, want, because(tc.why))
		}
	}
	for cmd := range wantProgram {
		if !visited[cmd] {
			t.Errorf("wantProgram names %q, which no row runs", cmd)
		}
	}
}

// programOf is a shape's program for a failure message: quoted, or null.
func programOf(s Shape) string {
	if s.Program == nil {
		return "null"
	}
	return strconv.Quote(*s.Program)
}

// TestTestClassRefusesWhatDoesNotRunTests: a runner on the list, whole on its
// line, still is not a test run when an argument makes it do something else --
// compile, list, print, watch, show its version -- or when the command names a
// target that is not only tests: `tox -e lint`, `nox -s lint`. `make check`
// is off the list for the same reason: it is lint and tests on most projects.
// Each of these ends ok or failed on something other than a test's result, so
// as a test run it would complete a pattern on an outcome no test had.
func TestTestClassRefusesWhatDoesNotRunTests(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		want string
	}{
		{"make check", VerbExecute},
		{"tox -e lint", VerbExecute},
		{"tox -elint", VerbExecute},
		{"tox --env=lint", VerbExecute},
		{"tox -l", VerbExecute},
		{"nox -s lint", VerbExecute},
		{"nox --session lint", VerbExecute},
		{"nox -k lint", VerbExecute},
		{"go test -c ./pkg", VerbPackage},
		{"go test -n ./...", VerbPackage},
		{"go test -list . ./...", VerbPackage},
		{"go test -list=Foo ./...", VerbPackage},
		{"go test --help", VerbPackage},
		{"pytest --collect-only", VerbExecute},
		{"pytest --co -q", VerbExecute},
		{"python -m pytest --version", VerbExecute},
		{"pytest --fixtures", VerbExecute},
		{"pytest -h", VerbExecute},
		{"cargo test --no-run", VerbPackage},
		{"cargo test -- --list", VerbPackage},
		{"jest --listTests", VerbExecute},
		{"jest --watch", VerbExecute},
		{"jest --watchAll", VerbExecute},
		{"jest --showConfig", VerbExecute},
		{"npm test -- --watch", VerbPackage},
		{"vitest watch", VerbExecute},
		{"vitest list", VerbExecute},
		{"vitest --watch", VerbExecute},
		{"mocha --watch", VerbExecute},
		{"rspec --dry-run", VerbExecute},
		{"phpunit --list-tests", VerbExecute},
		{"ctest -N", VerbExecute},
		{"ctest --show-only=json-v1", VerbExecute},
		{"dotnet test --list-tests", VerbExecute},
		{"mvn test -DskipTests", VerbExecute},
		{"mvn test -Dmaven.test.skip=true", VerbExecute},
		{"mvn test -Dmaven.test.skip", VerbExecute},
		{"make test -n", VerbExecute},
		{"make test --just-print", VerbExecute},
		{"make test --dry-run", VerbExecute},
		{"make test -q", VerbExecute},
		{"make test --question", VerbExecute},
		{"make test -v", VerbExecute},
		{"mvn test -v", VerbExecute},
		{"gradle test -v", VerbExecute},
		{"gradle test -t", VerbExecute},
		{"gradle test --continuous", VerbExecute},
		{"tox --notest", VerbExecute},
		{"tox devenv", VerbExecute},
		{"tox d", VerbExecute},
		{"nox --install-only", VerbExecute},
		{"jest --watchAll=true", VerbExecute},
		{"jest --watch=true", VerbExecute},
		{"go test -c=true ./pkg", VerbPackage},
		{"PYTEST_ADDOPTS=--co pytest", VerbExecute},

		// A wrapped runner is refused on its own runner's list.
		{"./gradlew test --dry-run", VerbExecute},
		{"./gradlew test --continuous", VerbExecute},
		{"./mvnw test -DskipTests", VerbExecute},
		{"npm t -- --watch", VerbPackage},
		{"npm t --help", VerbPackage},
		{"npx jest --listTests", VerbPackage},
		{"npx vitest list", VerbPackage},
		{"uv run pytest --co", VerbPackage},
		{"PYTEST_ADDOPTS=--co poetry run pytest", VerbPackage},
		{"bundle exec rspec --dry-run", VerbPackage},
		{"timeout 60 go test -c ./pkg", VerbExecute},
		{"time jest --watch", VerbExecute},
		{`PYTEST_ADDOPTS="--co -q" python -m pytest`, VerbExecute},
		{"cd sub && PYTEST_ADDOPTS=-x pytest", VerbExecute},

		// The neighbours that do run the tests stay test.
		{"go test -v -run TestX ./...", VerbTest},
		{"go test -count=1 -json ./...", VerbTest},
		{"pytest -n 4", VerbTest},
		{"pytest -x tests/test_c.py", VerbTest},
		{"jest --watchAll=false", VerbTest},
		{"jest --watch=0", VerbTest},
		{"go test -c=false ./...", VerbTest},
		{"make test", VerbTest},
		{"gradle test", VerbTest},
		{"CI=1 pytest", VerbTest},
		{"jest -u", VerbTest},
		{"vitest run", VerbTest},
		{"cargo test --release", VerbTest},
		{"tox", VerbTest},
		{"tox -p", VerbTest},
		{"nox", VerbTest},
	} {
		if got := verbOf(t, tc.cmd).VerbClass; got != tc.want {
			t.Errorf("%q: verb class %q, want %q", tc.cmd, got, tc.want)
		}
	}

	// The lines a flag before the row's words could reach, with the masked
	// side asserted beside the class: kind is the runner_kind, "" for null.
	// Unless a comment says otherwise, each row reads as it did before
	// leadingFlags existed.
	for _, tc := range []struct{ cmd, class, masked, kind string }{
		// An unlisted flag stops the match.
		{"make -n test", VerbExecute, MaskedNone, ""},
		{"make -j4 -n test", VerbExecute, MaskedNone, ""},
		{"mvn -B -DskipTests test", VerbExecute, MaskedNone, RunnerBuild},
		{"make -Cdir test", VerbExecute, MaskedNone, RunnerBuild},
		{"make --directory=sub test", VerbExecute, MaskedNone, RunnerBuild},
		{"go -C=sub test ./...", VerbPackage, MaskedNone, ""},
		{"go -C sub test ./...", VerbPackage, MaskedNone, ""},
		{"python -W ignore -m pytest", VerbExecute, MaskedNone, ""},
		{"bazel --bazelrc=ci.bazelrc test //...", VerbExecute, MaskedNone, ""},
		{"make +x test", VerbExecute, MaskedNone, RunnerBuild},     // the toolchain word is cargo's alone
		{"mvn -B=true test", VerbExecute, MaskedNone, RunnerBuild}, // a listed word with =value is unlisted
		{"make -j$N test", VerbExecute, MaskedNone, RunnerBuild},   // an expansion may be more words than one
		{`make "-j4" test`, VerbExecute, MaskedNone, RunnerBuild},  // a quoted word is not read as a listed flag
		// An expansion under a prefix entry may be more words than one, or none
		// (make's digits entry rejects it on its own).
		{"cargo +$TC test", VerbPackage, MaskedNone, ""},
		{"bazel --output_base=$D test //...", VerbExecute, MaskedNone, ""},
		// make reads a separate job count as -j's (`-j [N]`), so the 4 is
		// passed over with the flag; a word that is not digits is not a
		// count, and stops the match as any unlisted word does.
		{"make -j 4 test", VerbTest, MaskedNone, RunnerTest},
		{"make -j 4x test", VerbExecute, MaskedNone, RunnerBuild},
		{"make -j $N test", VerbExecute, MaskedNone, RunnerBuild},
		{`make -j "4" test`, VerbExecute, MaskedNone, RunnerBuild},
		// make exits 2 before any recipe on these, so a line that ran no
		// test would record a failed test run if -j admitted whatever
		// follows it (version 5's `-j*`). The digits entry stops the match
		// at them, as before leadingFlags.
		{"make -j4x test", VerbExecute, MaskedNone, RunnerBuild},
		{"make -j=4 test", VerbExecute, MaskedNone, RunnerBuild},
		{"make -jx test", VerbExecute, MaskedNone, RunnerBuild},
		// The count may be left out (GNU make's `-j [N]`).
		{"make -j test", VerbTest, MaskedNone, RunnerTest},
		{"make -j16 test", VerbTest, MaskedNone, RunnerTest},
		// ACCEPTED GAP: the same false red where cargo cannot run the
		// toolchain: rustup's proxy on one it does not have, and a cargo
		// installed without rustup on any toolchain word, which it reads as
		// an unknown command. A toolchain name has no grammar a manual could
		// give the table. Before leadingFlags it read package and null.
		{"cargo +nonesuch test", VerbTest, MaskedNone, RunnerTest},

		// A value spelled like the row's word is not reached: its flag is
		// unlisted.
		{"make -C test test", VerbExecute, MaskedNone, RunnerBuild},
		{"go -C test test ./...", VerbPackage, MaskedNone, ""},
		{"gradle -p test test", VerbExecute, MaskedNone, RunnerBuild},
		{"mvn -P test compile", VerbExecute, MaskedNone, RunnerBuild},
		{"mvn -pl test compile", VerbExecute, MaskedNone, RunnerBuild},

		// A run that ends 0 whatever the tests or the build did, a hiding
		// flag (leadingFlags, an entry ending in !), before or after the
		// row's words: the runner ran and its status never reached the
		// line, so the line keeps its program's class and records the
		// runner masked, with its kind, as `make test | tail` does. Until
		// version 8 these were refused as notARun and recorded no runner.
		{"make -i test", VerbExecute, MaskedTest, RunnerTest},
		{"make test -i", VerbExecute, MaskedTest, RunnerTest},
		{"make --ignore-errors test", VerbExecute, MaskedTest, RunnerTest},
		{"make test --ignore-errors", VerbExecute, MaskedTest, RunnerTest},
		{"make -i build", VerbExecute, MaskedBuild, RunnerBuild},
		{"mvn -fn test", VerbExecute, MaskedTest, RunnerTest},
		{"mvn test -fn", VerbExecute, MaskedTest, RunnerTest},
		{"mvn --fail-never test", VerbExecute, MaskedTest, RunnerTest},
		{"mvn test --fail-never", VerbExecute, MaskedTest, RunnerTest},
		{"mvn -fn package", VerbExecute, MaskedBuild, RunnerBuild},
		{"npm --if-present test", VerbPackage, MaskedTest, RunnerTest},
		{"npm test --if-present", VerbPackage, MaskedTest, RunnerTest},
		{"mvn test -Dmaven.test.failure.ignore", VerbExecute, MaskedTest, RunnerTest},
		{"mvn -Dmaven.test.failure.ignore test", VerbExecute, MaskedTest, RunnerTest},
		{"mvn -Dmaven.test.failure.ignore=true test", VerbExecute, MaskedTest, RunnerTest},
		{"mvn -Dmaven.test.failure.ignore=true package", VerbExecute, MaskedBuild, RunnerBuild},
		{"mvn package -Dmaven.test.failure.ignore=true", VerbExecute, MaskedBuild, RunnerBuild},
		// A false value hides nothing: Maven reads only true as true, so
		// `=1` runs the tests as `=false` does.
		{"mvn test -Dmaven.test.failure.ignore=false", VerbTest, MaskedNone, RunnerTest},
		{"mvn test -Dmaven.test.failure.ignore=1", VerbTest, MaskedNone, RunnerTest},
		{"npm test --if-present=false", VerbTest, MaskedNone, RunnerTest},
		// A refusal after a hiding flag still refuses: the run is not one.
		{"make -i test -n", VerbExecute, MaskedNone, ""},

		// ACCEPTED GAP: an unlisted flag whose value is a separate word.
		{"bazel --output_base /tmp test //...", VerbExecute, MaskedNone, ""},
		{"npm --prefix web test", VerbPackage, MaskedNone, ""},
		// A directory option after the row's words: the runner's identity
		// carries it, since the runner digest is the runner's words, so each
		// is a runner apart from the bare row and from `cd sub && RUNNER`;
		// and the location is the line's start directory, which is the
		// directory a gate declared with the same words names. make reads
		// its options anywhere, cargo after the subcommand only, go before
		// the command or first after it.
		{"make test -C sub", VerbTest, MaskedNone, RunnerTest},
		{"cargo test --manifest-path sub/Cargo.toml", VerbTest, MaskedNone, RunnerTest},
		{"npm test --prefix web", VerbTest, MaskedNone, RunnerTest},
		{"go test -C sub ./...", VerbTest, MaskedNone, RunnerTest},
	} {
		s := verbOf(t, tc.cmd)
		masked, kind := "", ""
		if s.StatusMasked != nil {
			masked = *s.StatusMasked
		}
		if s.RunnerKind != nil {
			kind = *s.RunnerKind
		}
		if s.VerbClass != tc.class || masked != tc.masked || kind != tc.kind {
			t.Errorf("%q: class %q, masked %q, kind %q; want %q, %q, %q", tc.cmd, s.VerbClass, masked, kind, tc.class, tc.masked, tc.kind)
		}
	}
}

// TestLeadingFlagsAreNotRefusals: runnerOn passes over a listed flag without
// reading it, so a flag that refuses a row of its program (refusesRun: its
// notARun entry, notARunAny, mavenSkip) would make a line that runs no tests
// read as a test run. Every word notARun, notARunAny, mavenSkipProperties
// and leadingFlags name is tried as both, for every program the table keys,
// aliases among them. Break: list make's -n, or `-D*` for mvn.
func TestLeadingFlagsAreNotRefusals(t *testing.T) {
	var words []string
	for _, list := range notARun {
		words = append(words, list...)
	}
	words = append(words, notARunAny...)
	words = append(words, mavenSkipProperties...)
	for _, list := range leadingFlags {
		words = append(words, list...)
	}
	for prog := range leadingFlags {
		for _, rows := range [][][]string{testCommands, buildCommands} {
			for _, c := range rows {
				if c[0] != prog {
					continue
				}
				for _, w := range words {
					tok := []token{{text: strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(w, "*"), "#"), "!"), quotedAt: -1}}
					if strings.HasSuffix(w, "#") {
						tok[0].text += "4"
					}
					if leadingFlag(tok, 0, prog) && refusesRun(refusalsOf(c), nil, tok) {
						t.Errorf("%s's leadingFlags %q admit %q, which refuses the row %q", prog, leadingFlags[prog], tok[0].text, c)
					}
				}
			}
		}
	}
}

// TestLeadingOptionsOneMatcher: the verb class and the masked parser find a
// line's runner through one matcher (runnerOn), so a listed flag before the
// row's words moves the class and the runner kind together, and the lines
// the two read apart for other reasons stay apart. Each row is the line's
// verb_class, status_masked, runner_kind ("" for null) and whether
// runner_digest is set.
func TestLeadingOptionsOneMatcher(t *testing.T) {
	for _, tc := range []struct {
		cmd, class, masked, kind string
		digest                   bool
	}{
		{"make -j4 test", VerbTest, MaskedNone, RunnerTest, true},
		{"mvn -B test", VerbTest, MaskedNone, RunnerTest, true},
		{"gradle --no-daemon test", VerbTest, MaskedNone, RunnerTest, true},
		{"bazel --output_base=/tmp test //...", VerbTest, MaskedNone, RunnerTest, true},
		{"cargo +nightly test", VerbTest, MaskedNone, RunnerTest, true},
		{"python -u -m pytest", VerbTest, MaskedNone, RunnerTest, true},
		{"python3 -u -m pytest", VerbTest, MaskedNone, RunnerTest, true},
		{"./mvnw -B test", VerbTest, MaskedNone, RunnerTest, true},
		{"./gradlew --no-daemon test", VerbTest, MaskedNone, RunnerTest, true},

		// The program search does not step past `!` or `if`: the negation
		// inverts the runner's status and the condition swallows it, and
		// status_masked records both.
		{"! go test ./...", VerbExecute, MaskedTest, RunnerTest, true},
		{"if go test ./...; then :; fi", VerbExecute, MaskedTest, RunnerTest, true},

		// The program search folds the subshell's cd, and the masked parser
		// passes over the same fold, so the runner keeps its identity; where
		// it ran is the cwd digest's to say.
		{"(cd /tmp && go test ./...)", VerbTest, MaskedNone, RunnerTest, true},

		// ACCEPTED GAP: a chain with the runner last is not a test run,
		// though the runner's kind and digest are recorded. status_masked
		// answers "when the runner fails, does the line?", and the test
		// class needs "when the line passes, did the runner run and pass?":
		// the last row is masked none and runs nothing when x is absent, so
		// no check shared with status_masked can grant the class.
		{"echo hi && go test ./...", VerbExecute, MaskedNone, RunnerTest, true},
		{"true && go test ./...", VerbExecute, MaskedNone, RunnerTest, true},
		{"go build ./... && go test ./...", VerbPackage, MaskedNone, RunnerTest, true},
		{"make lint; go test ./...", VerbExecute, MaskedBuild, RunnerBuild, true},
		{"if [[ -d x ]]; then go test ./...; fi", VerbExecute, MaskedNone, RunnerTest, true},
	} {
		s := verbOf(t, tc.cmd)
		masked, kind := "", ""
		if s.StatusMasked != nil {
			masked = *s.StatusMasked
		}
		if s.RunnerKind != nil {
			kind = *s.RunnerKind
		}
		if s.VerbClass != tc.class || masked != tc.masked || kind != tc.kind || (s.RunnerDigest != nil) != tc.digest {
			t.Errorf("%q: class %q, masked %q, kind %q, digest set %v; want %q, %q, %q, %v",
				tc.cmd, s.VerbClass, masked, kind, s.RunnerDigest != nil, tc.class, tc.masked, tc.kind, tc.digest)
		}
	}
}

// TestLeadingOptionsKeepRunnerIdentity: the runner digest keys on the
// runner's words, its options with them, so passing over -j4 moves `make -j4
// test`'s kind and not its identity: it stays a runner apart from `make
// test`.
func TestLeadingOptionsKeepRunnerIdentity(t *testing.T) {
	a, b := verbOf(t, "make -j4 test").RunnerDigest, verbOf(t, "make test").RunnerDigest
	if a == nil || b == nil || *a == *b {
		t.Errorf("make -j4 test digest set %v, make test digest set %v, equal %v: want both set and unequal",
			a != nil, b != nil, a != nil && b != nil && *a == *b)
	}
}

// TestBackgroundLaunchIsNotATestRun: Claude Code records a Bash call with
// run_in_background true when the shell is launched, so its outcome is the
// launch's, and its digest -- the command line alone -- equals the foreground
// run's. It keeps the class its program gives it; only the boolean true
// changes that.
func TestBackgroundLaunchIsNotATestRun(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{`{"command":"make test","run_in_background":true}`, VerbExecute},
		{`{"command":"go test ./...","run_in_background":true,"description":"x"}`, VerbPackage},
		{`{"command":"go test ./...","run_in_background":false}`, VerbTest},
		{`{"command":"go test ./...","run_in_background":"true"}`, VerbTest},
		{`{"command":"go test ./..."}`, VerbTest},
	} {
		if got := Derive("Bash", json.RawMessage(tc.input), []byte("key")); got.VerbClass != tc.want {
			t.Errorf("%s: verb class %q, want %q", tc.input, got.VerbClass, tc.want)
		}
	}
}

// TestTestClassCarriesNoContent is the guarantee test. Deciding the class
// reads the words after the program, which nothing in this package did
// before; the only thing between that and a leak is that the result is a
// fixed word. So the whole marshalled shape of a test command full of canaries
// must hold none of them, and equal commands up to their arguments must
// differ only in the digest and the count.
func TestTestClassCarriesNoContent(t *testing.T) {
	const canary = "HUNTER2_CANARY"
	for _, cmd := range []string{
		"go test ./" + canary + "/...",
		"cd /home/" + canary + " && go test -run " + canary,
		"python -m pytest -k " + canary + " tests/test_" + canary + ".py",
		"npm run test -- --grep=" + canary,
		canary + "=1 make test",
	} {
		s := verbOf(t, cmd)
		if s.VerbClass != VerbTest {
			t.Fatalf("%q: class %q, want test; the premise of this test is broken", cmd, s.VerbClass)
		}
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(strings.ToLower(string(b)), strings.ToLower(canary)) {
			t.Errorf("%q: the shape carries the canary: %s", cmd, b)
		}
		if s.Program == nil || strings.Contains(*s.Program, "test") && *s.Program != "pytest" {
			t.Errorf("%q: program %v; the argument that decided the class must not become the program", cmd, s.Program)
		}
	}
}

// TestNoCorpusLineIsATestRun: every line of the leak corpus is a construct the
// program search could not vouch for, or one it read correctly. None of them
// runs a test runner, so none may come back test -- a class decided from a
// word inside something the search did not parse is the same leak by another
// field, even though the field can only ever say one word.
func TestNoCorpusLineIsATestRun(t *testing.T) {
	for _, cmd := range leakCorpus {
		raw, err := json.Marshal(map[string]string{"command": cmd})
		if err != nil {
			t.Fatal(err)
		}
		if got := Derive("Bash", raw, []byte("key")).VerbClass; got == VerbTest {
			t.Errorf("%q: verb class test", cmd)
		}
	}
}

// TestVerbClassesAreClosed: every class Derive can return is in VerbClasses(),
// and every class there is one some input produces.
func TestVerbClassesAreClosed(t *testing.T) {
	vocab := map[string]bool{}
	for _, v := range VerbClasses() {
		vocab[v] = true
	}
	for _, v := range programVerb {
		if !vocab[v] {
			t.Errorf("programVerb emits %q, which is not in VerbClasses()", v)
		}
	}
	for _, c := range testCommands {
		if len(c) == 0 {
			t.Fatal("an empty test command matches every program")
		}
	}
	seen := map[string]bool{}
	for _, tool := range []string{"Read", "Write", "WebFetch", "Bash", "Agent", "mcp__x__y", "Other"} {
		seen[verbForTool(tool)] = true
	}
	for _, cmd := range []string{"git status", "npm install", "go test"} {
		seen[verbOf(t, cmd).VerbClass] = true
	}
	for v := range vocab {
		if !seen[v] {
			t.Errorf("VerbClasses() carries %q, which nothing here produced", v)
		}
	}
}

// TestMayWrite: a shell line that may write files carries may_write whatever
// class its program gives it, and one that only reads does not. Break: miss
// one of these, and a read-class call that rewrote the tree lets a
// test-bending pair complete across it; set it on a plain read, and every
// `ls` between two runs stops the pair.
func TestMayWrite(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		want bool
	}{
		// The review's reproductions, all class read or network.
		{"find . -name '*.snap' -delete", true},
		{"find . -name '*.go' -exec sed -i s/a/b/ {} +", true},
		{"find . -type f -execdir chmod -x {} ;", true},
		{"find . -name x -ok rm {} ;", true},
		{"grep -rl foo . | xargs sed -i s/foo/bar/", true},
		{"rsync -a ../fixtures/ testdata/", true},
		{"scp host:f .", true},
		// Redirects to a file, in every spelling.
		{"cat a > b", true},
		{"cat a >> b", true},
		{"ls >| b", true},
		{"ls &> b", true},
		{"ls 2> err.log", true},
		{"cat <> f", true},
		{"cat a | tee b", true},
		// Downloads.
		{"curl -o f https://example.com/x", true},
		{"curl -sSLO https://example.com/x", true},
		{"curl --output=f https://example.com/x", true},
		{"wget https://example.com/x", true},
		// curl's attached -o: the rest of the word is the file it names.
		{"curl -o./x https://example.com/x", true},
		{"curl -sSLotestdata/x.json https://example.com/x", true},
		{"curl -ofoo https://example.com/x", true},
		// curl's digits, # and : take no argument, so the walk goes on past
		// them.
		{"curl -#O https://example.com/x", true},
		{"curl -#o f https://example.com/x", true},
		{"curl -:O https://example.com/x", true},
		{"curl -4sSLO https://example.com/x", true},
		{"curl -0o calc.go https://example.com/x", true},
		// A write inside a command or process substitution.
		{"ls $(rm -rf build)", true},
		{"cat `touch x`", true},
		{"grep foo <(rm x)", true},
		{"cat a > >(tee b)", true},
		{`cat "$(touch x)"`, true},
		{"ls $( (rm x) )", true},
		// A $(( that does not close as )) is a substitution of a subshell.
		{"ls $((rm x) )", true},
		{"cat $((touch x) 2>&1)", true},
		// A quoted ) inside is a word, and closes nothing.
		{`ls $((touch ")") )`, true},
		// A substitution inside arithmetic still runs.
		{`echo "$(( $(rm x) + 1 ))"`, true},
		// A writer word is compared wherever it stands.
		{"grep -rn xargs .", true},
		// A later stage outside the read class, or that cannot be named.
		{"ls && rm -rf build", true},
		{"cat a; touch b", true},
		{"grep x f || $CMD", true},
		{"ls\nrm f", true},
		// A line whose program cannot be named.
		{"$EDITOR file", true},
		// Reads, and redirects that write no file.
		{"ls", false},
		{"cat a | head -5", false},
		{"grep -rn foo . | wc -l", false},
		{"ls 2>&1 | head", false},
		{"cat a > /dev/null", false},
		{"cat a 2>/dev/null", false},
		{"ls >&2", false},
		{"cd sub && ls", false},
		{"curl https://example.com/x", false},
		{"curl -sS https://example.com/x | head", false},
		// An option that takes an argument: the rest of the word is its
		// value, not more flags.
		{"curl -XPOST https://example.com/x", false},
		{"curl -Hcontent-type:json https://example.com/x", false},
		{"curl -dfoo=bar https://example.com/x", false},
		{"go test ./... 2>&1 | tail -20", false},
		{`echo "a > b"`, false},
		// A $( that opens nothing: quoted literally, or arithmetic.
		{"grep -rn '$(' scripts", false},
		{"echo $((1+2))", false},
		{"head -n $((n+1)) file", false},
		{`echo "$((1+2))"`, false},
		{`head -n "$((n+1))" file`, false},
		// A ( inside arithmetic opens a group, which closes before the )).
		{"echo $(( (1+2) * 3 ))", false},
		{`echo "$(( (n+1) * 2 ))"`, false},
	} {
		if got := verbOf(t, tc.cmd).MayWrite; got != tc.want {
			t.Errorf("%q: may_write %v, want %v", tc.cmd, got, tc.want)
		}
	}
	// Not a shell: the class says what it does, and the bit stays false.
	if Derive("Write", json.RawMessage(`{"file_path":"/r/a","content":"> b"}`), []byte("key")).MayWrite {
		t.Error("a Write call carries may_write")
	}
}

// The directories a runner's line moves through before it starts, when the
// line begins with plain literal `cd DIR &&` steps, `~` and `~/...` among
// them: the hook keys the run on where they lead rather than on the payload's
// cwd, which is the shell's directory before the cds. The hook reads them
// through DeriveWithDirs, so each case holds it to LeadingDirectory's answer:
// a line with a control byte names no directory to either, since the shell
// does not split it where the tokenizer does.
func TestLeadingDirectory(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		want []string // nil for none
		cut  bool     // DeriveWithDirs's cut
	}{
		{"cd /repo/web && go test ./...", []string{"/repo/web"}, false},
		{"cd sub && go test ./...", []string{"sub"}, false},
		{"cd ../x && npm test", []string{"../x"}, false},
		{"cd /repo/web/ && go test ./...", []string{"/repo/web/"}, false},
		// Every step of the leading run, in order.
		{"cd /a && cd b && go test", []string{"/a", "b"}, false},
		// The home, `~` alone or before a slash, as written: the hook
		// expands it with its own home.
		{"cd ~/x && go test ./...", []string{"~/x"}, false},
		{"cd ~ && go test ./...", []string{"~"}, false},
		// Not a plain literal step: the shell computes the directory, or the
		// runner may run without the cd having happened. A named home and
		// the directory stack's `~+` and `~-` are the shell's to resolve.
		{"cd $HOME/x && go test ./...", nil, false},
		{"cd ~user/x && go test ./...", nil, false},
		{"cd ~+/x && go test ./...", nil, false},
		{"cd ~-/x && go test ./...", nil, false},
		{"cd - && go test ./...", nil, false},
		{"cd \"/repo/web\" && go test ./...", nil, false},
		{"cd 'a b' && go test ./...", nil, false},
		{"cd `pwd`/x && go test ./...", nil, false},
		{"cd *web && go test ./...", nil, false},
		{"cd /repo; go test ./...", nil, false},
		{"cd /repo || go test ./...", nil, false},
		{"cd /repo", nil, false},
		{"cd && go test ./...", nil, false},
		{"go test ./...", nil, false},
		{"X=1 cd /repo && go test ./...", nil, false},
		// A later step that cannot be resolved spoils the whole run: the
		// first target is not where the runner starts.
		{"cd /a && cd - && go test", nil, false},
		{"cd /a && cd ~user && go test", nil, false},
		{"cd /a && popd && go test", nil, false},
		{"cd /a && pushd ~+ && go test", nil, false},
		// A cd behind an assignment is still a directory change.
		{"cd /a && CDPATH= cd - && go test ./...", nil, false},
		{"cd /a && X=1 cd b && go test ./...", nil, false},
		// So is one behind a redirection, a subshell's ( or an
		// assignment-only step.
		{"cd /a && >/dev/null cd - && go test ./...", nil, false},
		{"cd /a && (cd - && go test ./...)", nil, false},
		{"cd /a && X=1 && cd - && go test ./...", nil, false},
		// An assignment in front of no cd ends the run where it stands.
		{"X=1", nil, false},
		{"cd /a && X=1", []string{"/a"}, false},
		{"cd /repo/web && GOFLAGS=-count=1 go test ./...", []string{"/repo/web"}, false},
		// A later step into the home is read like any other.
		{"cd /a && cd ~ && go test", []string{"/a", "~"}, false},
		// A control byte anywhere on the line.
		{"cd /repo\r && ls", nil, false},
		{"cd /repo\x00 && ls", nil, false},
		{"cd /repo && ls \x07", nil, false},
		{"cd /repo && \x1b[0m ls", nil, false},
		{"cd /repo\x7f && ls", nil, false},
		// The lines the runner matcher and the masked parser have read
		// apart: the directories are not read through runnerOn, so none of
		// them moves when it does.
		{"make -j4 test", nil, false},
		{"mvn -B test", nil, false},
		{"bazel --output_base=/tmp test //...", nil, false},
		{"cargo +nightly test", nil, false},
		{"make -Cdir test", nil, false},
		{"! go test ./...", nil, false},
		{"if go test ./...; then :; fi", nil, false},
		{"(cd /tmp && go test ./...)", nil, false},
		{"echo hi && go test ./...", nil, false},
		{"true && go test ./...", nil, false},
		{"go build ./... && go test ./...", nil, false},
		{"make lint; go test ./...", nil, false},
		{"cd /tmp && echo hi && go test ./...", []string{"/tmp"}, false},
		// MaxLeadingCd steps are all folded and the line is not cut; at one
		// more the read stops, holding that step too, and the line is cut.
		{strings.Repeat("cd a && ", MaxLeadingCd) + "go test ./...", slices.Repeat([]string{"a"}, MaxLeadingCd), false},
		{strings.Repeat("cd a && ", MaxLeadingCd+1) + "go test ./...", slices.Repeat([]string{"a"}, MaxLeadingCd+1), true},
		{strings.Repeat("cd ~/a && ", MaxLeadingCd+1) + "go test ./...", slices.Repeat([]string{"~/a"}, MaxLeadingCd+1), true},
	} {
		in := json.RawMessage(`{"command":` + quoteJSON(tc.cmd) + `}`)
		got, ok := LeadingDirectory("Bash", in)
		if ok != (tc.want != nil) || !slices.Equal(got, tc.want) {
			t.Errorf("%q: LeadingDirectory = %q, %v; want %q", tc.cmd, got, ok, tc.want)
		}
		_, cut, dirs, _ := DeriveWithDirs("Bash", in, []byte("key"))
		if !slices.Equal(dirs, tc.want) {
			t.Errorf("%q: DeriveWithDirs dirs = %q; want %q, as LeadingDirectory", tc.cmd, dirs, tc.want)
		}
		if cut != tc.cut {
			t.Errorf("%q: DeriveWithDirs cut = %v; want %v", tc.cmd, cut, tc.cut)
		}
	}
	if _, ok := LeadingDirectory("mcp__x__run", json.RawMessage(`{"command":"cd /repo && make test"}`)); ok {
		t.Error("a tool that is not a shell has no leading directory")
	}
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// TestUnittestIsATestRunner: python -m unittest runs tests, as python -m
// pytest does; three of the benchmark's failing runs were written with it.
func TestUnittestIsATestRunner(t *testing.T) {
	for _, cmd := range []string{"python -m unittest", "python3 -m unittest tests.test_mass -v"} {
		if got := verbOf(t, cmd).VerbClass; got != VerbTest {
			t.Errorf("%q: verb class %s, want test", cmd, got)
		}
	}
	if got := verbOf(t, "python3 -m unittest --help").VerbClass; got == VerbTest {
		t.Error("python3 -m unittest --help is a test run")
	}
}
