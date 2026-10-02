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
		{"npx --yes jest", VerbPackage, "the word after npx must be the runner"},
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
		if prog, ok := wantProgram[tc.cmd]; ok && (got.Program == nil || *got.Program != prog) {
			t.Errorf("%q: program %v, want %q: the report reads a fired timeout's 124 by it", tc.cmd, got.Program, prog)
		}
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
		{"curl -4sSLO https://example.com/x", true},
		{"curl -0o calc.go https://example.com/x", true},
		// A write inside a command or process substitution.
		{"ls $(rm -rf build)", true},
		{"cat `touch x`", true},
		{"grep foo <(rm x)", true},
		{"cat a > >(tee b)", true},
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

// The directory a runner starts in, when the line begins with a plain literal
// `cd DIR &&`: the hook keys the run on it rather than on the payload's cwd,
// which is the shell's directory before the cd.
func TestLeadingDirectory(t *testing.T) {
	for _, tc := range []struct {
		cmd  string
		want string // "" for none
	}{
		{"cd /repo/web && go test ./...", "/repo/web"},
		{"cd sub && go test ./...", "sub"},
		{"cd ../x && npm test", "../x"},
		{"cd /repo/web/ && go test ./...", "/repo/web/"},
		// Not a plain literal step: the shell computes the directory, or the
		// runner may run without the cd having happened.
		{"cd $HOME/x && go test ./...", ""},
		{"cd ~/x && go test ./...", ""},
		{"cd - && go test ./...", ""},
		{"cd \"/repo/web\" && go test ./...", ""},
		{"cd 'a b' && go test ./...", ""},
		{"cd `pwd`/x && go test ./...", ""},
		{"cd *web && go test ./...", ""},
		{"cd /repo; go test ./...", ""},
		{"cd /repo || go test ./...", ""},
		{"cd /repo", ""},
		{"cd && go test ./...", ""},
		{"go test ./...", ""},
		{"X=1 cd /repo && go test ./...", ""},
	} {
		got, ok := LeadingDirectory("Bash", json.RawMessage(`{"command":`+quoteJSON(tc.cmd)+`}`))
		if ok != (tc.want != "") || got != tc.want {
			t.Errorf("%q: LeadingDirectory = %q, %v; want %q", tc.cmd, got, ok, tc.want)
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
