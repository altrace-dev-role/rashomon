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
		{"set -e; make test; echo done", MaskedNone, "errexit stops the line at the failure"},
		{"set -o errexit; make test; echo done", MaskedNone, ""},
		{"bash -o pipefail -c 'make test | tail'", MaskedNone, "the quoted line is one argument, not looked into"},
		{"make test || exit 1", MaskedNone, "exits failed"},
		{"make test || exit", MaskedNone, "exits with the runner's status"},
		{"make test; exit $?", MaskedNone, ""},
		{"make test || false", MaskedNone, "false is a failure too"},
		{"make test; if [ $? -ne 0 ]; then exit 1; fi", "null", "a compound command may read $?: not followed"},
		{"make test || { echo failed; exit 1; }", "null", "a group may end in exit: not followed"},
		{"make test &", MaskedNone, "backgrounded: its status is no one's"},
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

	// The first hidden runner's, test before build.
	if got := digestOf("make release | tail; make test | tail"); got != plain {
		t.Error("a line hiding a build and a test carries the build runner's digest, want the test's")
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
	// Not the last command, and not hidden: no runner digest.
	if _, d := maskedOf(t, "make test && git status"); d != nil {
		t.Error("`make test && git status` carries a runner digest; its status is git's when make passed")
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
