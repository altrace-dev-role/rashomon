package shape

import (
	"encoding/json"
	"strings"
	"testing"
)

// The commands marked "benchmark" are the shapes a coding agent wrote in a
// 100-run benchmark, where 47 of the 48 failing builds and tests one model
// ran were recorded ok because of what followed the runner on the line.

// heredocEdit is the benchmark's edit-then-check shape: a here-document
// whose body holds quotes, parens and a line that reads like a command, then
// the check.
const heredocEdit = "python3 - <<'E'\n" +
	"s=open('ledger.go').read()\n" +
	"s=s.replace('\"1.8.0\"','\"1.8.1\"')\n" +
	"make test | tail\n" +
	"open('ledger.go','w').write(s)\n" +
	"E\n"

func maskedOf(t *testing.T, cmd string) (string, *string) {
	t.Helper()
	s := verbOf(t, cmd)
	if s.StatusMasked == nil {
		return "null", s.RunnerDigest
	}
	return *s.StatusMasked, s.RunnerDigest
}

// TestStatusMasked: a recognised build or test runner whose exit status the
// line does not return is named, by kind; one whose status the line returns
// is not; and a line that cannot be read to its end says nothing.
func TestStatusMasked(t *testing.T) {
	for _, tc := range []struct {
		cmd, want, why string
	}{
		// A pipe after the runner: the pipeline's status is the last stage's.
		{"make test 2>&1 | tail -40", MaskedTest, "benchmark"},
		{"make test 2>&1 | grep -E \"^(FAIL|ERROR)\"", MaskedTest, "benchmark: grep's status, inverted"},
		{"python3 -m unittest tests.test_temperature -v 2>&1 | tail -20", MaskedTest, "benchmark"},
		{"timeout 300 make test 2>&1 | tail -30", MaskedTest, "benchmark: a prefix that passes the status through"},
		{"go test ./... | tee test.log", MaskedTest, "tee writes the log and exits 0"},
		{"make test |& cat", MaskedTest, "|& is a pipe too"},
		{"cd /repo && make test 2>&1 | tail -5", MaskedTest, "a leading cd"},
		{"make check 2>&1 | tail -60", MaskedBuild, "benchmark: make is any target"},
		{"cargo build 2>&1 | head", MaskedBuild, ""},
		{"make test | grep -v ok", MaskedTest, "the -v is grep's, not make's"},
		{"cat Makefile; make test 2>&1 | grep -E \"^(FAIL|ERROR)\"", MaskedTest, "benchmark: a later command of the list"},
		{"(make test) | tail", MaskedTest, "a subshell's status is its last command's"},

		// What follows the runner's pipeline runs either way.
		{"go test ./... ; echo $?", MaskedTest, "the status is printed, and the call exits 0"},
		{"make check 2>&1; echo \"EXIT: $?\"", MaskedBuild, "benchmark"},
		{"make integration > /tmp/integ.log 2>&1; echo \"EXIT=$?\"; tail -30 /tmp/integ.log", MaskedBuild, "benchmark"},
		{"make test 2>&1 | tail -40; git log --stat | head; ls", MaskedTest, "benchmark"},
		{"make validate\ncat docs/retry-policies.md", MaskedBuild, "a newline is a separator"},
		{"make test 2>&1 | tail -4 && git commit -qam x", MaskedTest, "benchmark: the pipe hides it before the &&"},
		{"make test && echo ok; echo done", MaskedTest, "&& passes the failure on, and ; then hides it"},

		// || runs exactly when the runner failed.
		{"npm test || true", MaskedTest, ""},
		{"make ci || :", MaskedBuild, ""},
		{"make test || echo failed", MaskedTest, ""},
		{"make test || false; echo done", MaskedTest, "false keeps the failure, and ; hides it"},
		{"make test && echo ok || echo fail", MaskedTest, ""},

		// A here-document before the check, as the benchmark wrote it.
		{heredocEdit + "gofmt -l .; make release 2>&1 | tail -20", MaskedBuild, "benchmark: the body is skipped, not read as commands"},
		{"cat >> geo.py <<'EOF'\ndef f(a, b):\n    return (a, b)\nEOF\nmake test > /tmp/t.log 2>&1; echo rc=$?; tail -5 /tmp/t.log", MaskedTest, "benchmark"},
		{"# run the tests\nmake test | tail", MaskedTest, "a comment line is nothing"},
		{"# don't stop\nmake test | tail", MaskedTest, "a quote in a comment opens nothing"},

		// The runner's status is the line's.
		{"make test", MaskedNone, ""},
		{"make test 2>&1", MaskedNone, ""},
		{"cd /repo && go test ./...", MaskedNone, ""},
		{"make test && git status --short", MaskedNone, "&&: a failure stops the list, and the line exits with it"},
		{"make lint && make test", MaskedNone, ""},
		{"git status; make test", MaskedNone, "the runner is last"},
		{"make test;", MaskedNone, "nothing after the ;"},
		{"echo start | make test", MaskedNone, "the runner is the last stage"},
		{"set -o pipefail; make test 2>&1 | tail -40", MaskedNone, "pipefail returns the runner's failure"},
		{"set -euo pipefail\nmake test | tail -5", MaskedNone, "a cluster that holds o names pipefail"},
		{"set -e; make test; echo done", "null", "bash 1/0, but zsh under Claude Code's wrapper 0/0: errexit never fires there"},
		{"set -o errexit; make test; echo done", "null", "as set -e"},
		{"bash -o pipefail -c 'make test | tail'", "null", "a shell's -c line is not looked into"},
		{"make test || exit 1", MaskedNone, "exits failed"},
		{"make test || exit", MaskedNone, "exits with the runner's status"},
		{"make test || exit 1; echo done", MaskedNone, "the exit ends the line before echo"},
		{"make release | tail; make test | tail", MaskedTest, "a hidden test runner is named over a hidden build"},
		{"make test || (echo failed; exit 1); echo done", MaskedTest, "a subshell's exit ends the subshell, and echo runs"},
		{"make test; for i in 1; do exit $?; done", "null", "a loop that holds an exit is not run"},
		{"false && make test | tail", "null", "the runner never runs in the abstract run"},
		{"make test; exit $?", MaskedNone, ""},
		{"make test || false", MaskedNone, "false is a failure too"},
		{"make test; if [ $? -ne 0 ]; then exit 1; fi", "null", "a compound command may read $?: not followed"},
		{"make test || { echo failed; exit 1; }", MaskedNone, "the group ends in exit 1, which it runs when make failed"},
		{"make -n test | tail", MaskedNone, "make -n runs nothing"},
		{"go test -c ./pkg | tail", MaskedNone, "go test -c compiles only"},
		{"ls | tail; echo $?", MaskedNone, "no runner"},
		{"python3 tools/migrate.py configs/*.json; git status", MaskedNone, "not a recognised runner"},
		{"echo 'make test | tail'", MaskedNone, "quoted text"},

		// Read to its end, or nothing is said.
		{"cat <<EOF\nmake test | tail\n", "null", "a here-document whose delimiter never comes"},
		{"make test | tail; echo \"unterminated", "null", "a quote that never closes"},
		{"x=$(cat <<EOF\nhi\nEOF\n); make test | tail", "null", "a here-document inside a substitution"},
		{"make test | tail\x00 ; exit", "null", "a control byte"},

		// The rows of the PR #48 review, each checked in bash 3.2.57 with the
		// runner stubbed failing, then passing: 0 on failure, or the same
		// status either way, is test; 1/0 is none.
		{"set -e; make test 2>&1 | tail -40", MaskedTest, "0/0: errexit sees the pipeline's status, tail's"},
		{"set -euo pipefail; npm test || true", MaskedTest, "0/0: errexit ignores a command before ||"},
		{"set -e; make test && echo ok; echo done", MaskedTest, "0/0: errexit ignores a command before &&"},
		{"set -e; set +e; make test; echo done", MaskedTest, "0/0: set +e turns it off"},
		{"set -o pipefail; set +o pipefail; make test | tail", MaskedTest, "0/0: set +o pipefail turns it off"},
		{"make test 2>&1 | tail -40; exit ${PIPESTATUS[0]}", "null", "bash 1/0, zsh 0/0: zsh has no PIPESTATUS"},
		{"make test; rc=$?; echo done; exit $rc", MaskedNone, "1/0: the status is kept and returned"},
		{"{ make test; }", MaskedNone, "1/0: a group's status is its last command's"},
		{"(make test || exit 1)", MaskedNone, "1/0"},
		{"(make test; exit $?)", MaskedNone, "1/0"},
		{"if make test; then echo ok; fi", MaskedTest, "0/0: a failed condition with no else is 0"},
		{"! make test", MaskedTest, "0/1: inverted"},
		{"if [ -f Makefile ]; then make test 2>&1 | tail -20; fi", MaskedTest, "0/0"},
		{"# run the tests \\\nmake test | tail", MaskedTest, "0/0: a backslash ends no comment, and the second line runs"},
		{"make test &", MaskedTest, "0/0: backgrounded, its status is no one's"},
		{"make test | tail; make test", MaskedNone, "1/0: the same runner, run again last"},
		{"make test && if true; then echo ok; fi", MaskedNone, "1/0"},
		{"set -o pipefail; make test | while read l; do :; done", MaskedNone, "1/0: pipefail, and the loop ends 0"},
		{"bash -c 'make test | tail'", "null", "0/0, but a shell's -c line is not looked into"},
		{"{ make test; } | tail", MaskedTest, "0/0"},
		{"make test | tail &", MaskedTest, "0/0"},
		{"make test && echo ok", MaskedNone, "1/0"},

		// The parser's other branches, each checked in bash. In the
		// here-document rows the runner in the body never runs.
		{"make test || exit 0", MaskedTest, "0/0"},
		{"make test || exit 256", MaskedTest, "0/0: exit takes its status modulo 256"},
		{"make test || exit 1 | cat", MaskedTest, "0/0: an exit in a pipe ends its own subshell"},
		{"make test || false | cat", MaskedTest, "0/0: cat's status"},
		{"make test && echo a | cat; echo b", MaskedTest, "0/0"},
		{"cat <<< x\nmake test | tail", MaskedTest, "a here-string has no body"},
		{"cat <<-EOF\n\tmake test | tail\n\tEOF\ngit status", MaskedNone, "<<- strips the tabs before the delimiter"},
		{"cat <<\"EOF\"\nmake test | tail\nEOF\ngit status", MaskedNone, "a quoted delimiter"},
		{"cat <<A <<B\nmake test | tail\nA\nmake test | tail\nB\ngit status", MaskedNone, "both bodies are skipped"},
		{"echo ${HOME} <<EOF\nmake test | tail\nEOF\ngit status", "null", "a here-document after an expansion: its extent is not certain"},
		{"mvn -q -DskipTests package 2>&1 | tail -30", MaskedBuild, "0/0: skipping the tests still builds"},
		{"mvn -q package 2>&1 | tail -30", MaskedBuild, "0/0"},

		// The shell Claude Code runs a line in on macOS is zsh, inside
		// `zsh -c "... && eval '<line>' < /dev/null && ..."`, where errexit
		// never fires and PIPESTATUS is empty. The record does not say which
		// shell ran the line, so where bash and that shell disagree it is null.
		{"set -euo pipefail; make test 2>&1 | tail -40; echo \"exit=$?\"", "null", "bash 1/0, zsh in the wrapper 0/0"},
		{"set -eo pipefail; go test ./... 2>&1 | tail -20; echo finished", "null", "bash 1/0, zsh in the wrapper 0/0"},
		{"set -e\nmake test\necho done", "null", "bash 1/0, zsh in the wrapper 0/0"},
		{"set -e; make test | tail; exit ${PIPESTATUS[0]}", "null", "PIPESTATUS is not read"},
		{"make test | tail; rc=${PIPESTATUS[0]}; exit $rc", "null", "PIPESTATUS is not read"},
		{"go test ./... 2>&1 | tee out.log; exit ${PIPESTATUS[0]}", "null", "PIPESTATUS is not read"},
		{"{ make test | tail; }; exit ${PIPESTATUS[0]}", "null", "PIPESTATUS is not read"},
		{"set -e; { make test && true; }; echo done", "null", "0/0 in every shell, but the abstract bash run fires errexit after the group (1/0): the two runs disagree"},
		{"set -euo pipefail; make test 2>&1 | tail -40", MaskedNone, "1/0 in every shell: pipefail returns make's failure"},
		{"set -e; make test || true; echo done", MaskedTest, "0/0 in every shell"},

		// `set -opipefail`: bash 3.2 rejects the word (1/1), zsh reads
		// pipefail (1/0). The shells disagree: null.
		{"set -opipefail; make test | tail", "null", "o not last in its cluster"},
		{"set -euopipefail; make test | tail", "null", "o not last in its cluster"},
	} {
		got, _ := maskedOf(t, tc.cmd)
		if got != tc.want {
			t.Errorf("%q: status_masked %s, want %s (%s)", tc.cmd, got, tc.want, tc.why)
		}
	}
}

// TestStatusMaskedNotOnOtherTools: only a shell line is read, and anything
// else says nothing rather than "none".
func TestStatusMaskedNotOnOtherTools(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"command": "make test | tail", "file_path": "x"})
	for _, tool := range []string{"Read", "Edit", "mcp__x__run", "Task"} {
		s := Derive(tool, raw, []byte("key"))
		if s.StatusMasked != nil || s.RunnerDigest != nil {
			t.Errorf("%s: status_masked %v, runner_digest %v; want both null", tool, s.StatusMasked, s.RunnerDigest)
		}
	}
}

// TestRunnerDigest: the runner's own words, without its redirections or a
// timeout in front, so that the hidden run and a later plain one compare
// equal; a different target, another key or another runner does not.
func TestRunnerDigest(t *testing.T) {
	digestOf := func(cmd string) string {
		t.Helper()
		_, d := maskedOf(t, cmd)
		if d == nil {
			t.Fatalf("%q: no runner digest", cmd)
		}
		if len(*d) != 64 {
			t.Fatalf("%q: runner digest %q is not 64 hex characters", cmd, *d)
		}
		return *d
	}
	plain := digestOf("make test")
	for _, same := range []string{
		"make test 2>&1 | tail -40",
		"make test > /tmp/test.log 2>&1; echo rc=$?",
		"make test &>/tmp/log; echo $?",
		"timeout 300 make test 2>&1 | tail -30",
		"cd /repo && make test",
		"git status; make test 2>&1 | tail -5",
		"(make test 2>&1) | tail -40",
	} {
		if got := digestOf(same); got != plain {
			t.Errorf("%q: runner digest differs from plain `make test`'s", same)
		}
	}
	for _, other := range []string{"make release 2>&1 | tail", "make test-py38 | tail", "go test ./... | tail", "make test V=1 | tail"} {
		if digestOf(other) == plain {
			t.Errorf("%q: runner digest equals `make test`'s", other)
		}
	}
	raw, _ := json.Marshal(map[string]string{"command": "make test"})
	if d := Derive("Bash", raw, []byte("other key")).RunnerDigest; d == nil || *d == plain {
		t.Error("the runner digest is not keyed")
	}
	if sd := verbOf(t, "make test").Digest; sd == plain {
		t.Error("the runner digest equals the shape digest of the same line: the domains are not apart")
	}

	// Two runners hidden: no one later run follows both up, so none.
	if _, d := maskedOf(t, "make release | tail; make test | tail"); d != nil {
		t.Error("a line hiding a build and a test carries a runner digest; a later `make test` would clear `make release` too")
	}
	for _, none := range []string{"ls | tail", "make test | tail; ls", "git status"} {
		_, d := maskedOf(t, none)
		if none == "make test | tail; ls" {
			if d == nil {
				t.Errorf("%q: no runner digest for its hidden runner", none)
			}
			continue
		}
		if d != nil {
			t.Errorf("%q: runner digest %s, want null", none, *d)
		}
	}
	// Not the last command, and its failure is still the line's: make's.
	if got := digestOf("make test && git status"); got != plain {
		t.Error("`make test && git status` does not carry make's digest; when make fails the line does")
	}
	// The same words, with an expansion in them, are the same runner.
	if digestOf("go test $(go list ./...) | tail") != digestOf("go test $(go list ./...)") {
		t.Error("`go test $(go list ./...)` piped and plain digest apart")
	}
	if digestOf("go test $(go list ./...)") == digestOf("go test $(go list ./cmd/...)") {
		t.Error("two runners whose expansions differ digest equal")
	}
	// After a cd the start directory does not hold, where the runner ran is
	// not known: no digest, hidden or not.
	for _, cmd := range []string{"cd sub; make test | tail", "cd sub; make test", "cd $D && make test"} {
		if _, d := maskedOf(t, cmd); d != nil {
			t.Errorf("%q carries a runner digest after a cd its cwd does not hold", cmd)
		}
	}
	if got := digestOf("cd /repo && make test"); got != plain {
		t.Error("a folded leading cd loses the runner digest")
	}
}

// TestStatusMaskedCarriesNoContent: the two fields are a closed word and a
// digest; no word of the line reaches either.
func TestStatusMaskedCarriesNoContent(t *testing.T) {
	cmd := "cd /home/alice/secret && make deploy-SECRET 2>&1 | tail -40"
	s := verbOf(t, cmd)
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"alice", "secret", "SECRET", "deploy", "tail"} {
		if strings.Contains(string(b), leak) {
			t.Errorf("shape %s carries %q", b, leak)
		}
	}
	if s.StatusMasked == nil || *s.StatusMasked != MaskedBuild {
		t.Errorf("status_masked = %v, want build", s.StatusMasked)
	}
}
