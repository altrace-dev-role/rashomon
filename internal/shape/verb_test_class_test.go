package shape

import (
	"encoding/json"
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
		{"gradle test", VerbTest, ""},
		{"make test", VerbTest, ""},
		{"python -m pytest -q", VerbTest, ""},
		{"python3 -m pytest", VerbTest, ""},
		{"/usr/local/go/bin/go test ./pkg", VerbTest, ""},

		// Under #29's rules: a directory change, an assignment, a separator.
		{"cd /repo && go test ./...", VerbTest, "cd is where a command runs, not what it runs"},
		{"cd a; cd b && npm test", VerbTest, ""},
		{"CGO_ENABLED=0 go test ./...", VerbTest, ""},
		{"( cd x && pytest )", VerbTest, ""},
		{"go test ./... 2>&1", VerbTest, "a redirection is part of the runner's command"},
		{"go test ./... > out.txt 2>&1", VerbTest, ""},
		{"go test ./...\n", VerbTest, "a trailing newline starts no command"},

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
		{"timeout 60 go test ./...", VerbExecute, "the program is timeout"},
		{"npx jest", VerbPackage, "npx is not on the list"},
		{"./gradlew test", VerbExecute, "the wrapper is not on the list"},
		{"echo go test", VerbExecute, "an argument that looks like a runner is an argument"},
		{"cd /x || go test", VerbExecute, "only && and ; are followed past cd; the program is cd"},
		{"$GO test", VerbUnknown + "-null", "the program is whatever the variable holds"},
		{"test -f go.mod", VerbExecute, "the test builtin is not a test runner"},
	} {
		got := verbOf(t, tc.cmd)
		want := tc.want
		if want == VerbUnknown+"-null" {
			if got.Program != nil || got.VerbClass != VerbExecute {
				t.Errorf("%q: program %v, class %q; want null and execute%s", tc.cmd, got.Program, got.VerbClass, because(tc.why))
			}
			continue
		}
		if got.VerbClass != want {
			t.Errorf("%q: verb class %q, want %q%s", tc.cmd, got.VerbClass, want, because(tc.why))
		}
	}
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
