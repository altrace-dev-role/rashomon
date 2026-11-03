package gateout

import (
	"os"
	"path/filepath"
	"testing"
)

// WRAPPER-RUN1: a wrapper's output (a script's, a shell's) and a delegate's
// (make's, npm's) are read only by the runner rows whose signature is in
// them. Every fixture under testdata/wr1 is a stream of a real command
// recorded on this machine on 11-01 (darwin/arm64) by se3's wr1-fx.sh and
// wr1-fx-go.sh, stdout and stderr kept apart; each case names the command,
// the tool and its version, and the stream. The projects were two-test ones:
// a passing pair, or one passing and one failing.

func wr1Fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "wr1", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestWR1_AWrapperIsReadByTheRunnerThatSignedIt: each runner's own output,
// read as a wrapper's, is that runner's pass, fail or empty run. Break: drop
// a row's signature, or let a wrapper's output be read by an unsigned row.
func TestWR1_AWrapperIsReadByTheRunnerThatSignedIt(t *testing.T) {
	for _, tc := range []struct{ file, kind, parser string }{
		// go test ./... (a module m with a passing test and a package b with
		// none), go1.27.3, stdout; stderr empty.
		{"go-pass.stdout", "summary_pass", "go"},
		// the same command again, served from the build cache: "(cached)".
		{"go-pass-cached.stdout", "summary_pass", "go"},
		// go test -count=1 -v ., go1.27.3, stdout.
		{"go-pass-v.stdout", "summary_pass", "go"},
		// go test -count=1 -cover ., go1.27.3, stdout.
		{"go-pass-cover.stdout", "summary_pass", "go"},
		// go test -count=1 -run '^TestNone$' ., go1.27.3, stdout.
		{"go-norun.stdout", "empty_run", "go"},
		// go test -count=1 ./b, a package with no test files, go1.27.3, stdout.
		{"go-notestfiles.stdout", "empty_run", "go"},
		// go test -count=1 ./... with TestA failing, go1.27.3, stdout.
		{"go-fail.stdout", "summary_fail", "go"},
		// go test -count=1 ./... with a_test.go not compiling, go1.27.3,
		// stdout; the compiler's lines are on stderr.
		{"go-build.stdout", "summary_fail", "go"},
		// go test -count=1 . importing a package that does not exist,
		// go1.27.3, stdout.
		{"go-setup.stdout", "summary_fail", "go"},
		// pytest -p no:cacheprovider, pytest 9.0.3 on Python 3.12.8, stdout;
		// stderr empty.
		{"pytest-pass.stdout", "summary_pass", "pytest"},
		{"pytest-fail.stdout", "summary_fail", "pytest"},
		// pytest -q -p no:cacheprovider, the same pytest, stdout.
		{"pytest-q-pass.stdout", "summary_pass", "pytest"},
		{"pytest-q-fail.stdout", "summary_fail", "pytest"},
		// pytest -p no:cacheprovider in a directory with no tests (exit 5),
		// stdout.
		{"pytest-empty.stdout", "empty_run", "pytest"},
		// cargo test --offline, cargo 1.98.1, stdout; "Compiling" and
		// "error: test failed" are on stderr.
		{"cargo-pass.stdout", "summary_pass", "cargo"},
		{"cargo-fail.stdout", "summary_fail", "cargo"},
		// node vitest.mjs run --globals --root ., vitest 1.6.1 on node
		// v22.19.0 with NO_COLOR=1, stdout; its labels are right-aligned, so
		// "Tests" is indented.
		{"vitest-pass.stdout", "summary_pass", "vitest"},
		{"vitest-fail.stdout", "summary_fail", "vitest"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			want(t, Classify("test", Wrapper, wr1Fixture(t, tc.file)), tc.kind, tc.parser, nil)
		})
	}
}

// TestWR1_AnOutputNoRunnerSignedIsNoRun: output with no runner's signature,
// read as a wrapper's, names no run (no_summary under Wrapper), and a
// delegate's own marker is no runner's. Each is a line some row's pass or
// fail pattern would take without its signature: TAP's "ok 1 - adds" is go's
// `^ok\s`. Break: let an unsigned row, or a delegate's marker, read a
// wrapper's output.
func TestWR1_AnOutputNoRunnerSignedIsNoRun(t *testing.T) {
	for _, file := range []string{
		// node --test --test-reporter=tap, node v22.19.0, stdout: TAP, as
		// bats prints it ("ok 1 - adds").
		"node-tap-pass.stdout",
		// python3 -m unittest test_u with test_x failing, Python 3.12.8,
		// stderr: "FAIL: test_x (...)", "FAILED (failures=1)".
		"unittest-fail.stderr",
		// make test over a recipe that exits 2, GNU Make 3.81, stderr:
		// "make: *** [test] Error 2".
		"make-fail.stderr",
		// cargo test --offline with a failing test, cargo 1.98.1, stderr:
		// "error: test failed, ..." with the summary on stdout, not here.
		"cargo-fail.stderr",
		// go test -count=1 -json ., go1.27.3, stdout: test2json events, no
		// summary line, which a wrapper leaves unknown.
		"go-json-pass.stdout",
	} {
		t.Run(file, func(t *testing.T) {
			want(t, Classify("test", Wrapper, wr1Fixture(t, file)), "no_summary", Wrapper, nil)
		})
	}
	// A runner that signed the output and printed no outcome line ran there;
	// how it ended is not in the output (`--- PASS` kept, the summary cut).
	want(t, Classify("test", Wrapper, []byte("=== RUN   TestA\n--- PASS: TestA (0.00s)\n")), "no_summary", "go", nil)
}

// TestWR1_ADelegateCountsARunnerOnlyWhereItSigned (D3): make's and npm's
// output is read by a signed runner as before, while TAP's "ok 1 - adds",
// which today read as go's pass, is no run of go's; a delegate's own marker
// and an unsigned row (jest, which has no recorded signature here) count as
// they did. Break: count go's `^ok\s` in a delegate's output go never
// signed.
func TestWR1_ADelegateCountsARunnerOnlyWhereItSigned(t *testing.T) {
	for _, tc := range []struct {
		program, file, kind, parser string
	}{
		{"make", "go-pass.stdout", "summary_pass", "go"},
		{"make", "go-fail.stdout", "summary_fail", "go"},
		{"npm", "vitest-pass.stdout", "summary_pass", "vitest"},
		{"npm", "vitest-fail.stdout", "summary_fail", "vitest"},
		{"make", "pytest-q-fail.stdout", "summary_fail", "pytest"},
		{"make", "node-tap-pass.stdout", "no_summary", "make"},
		{"make", "make-fail.stderr", "summary_fail", "make"},
	} {
		t.Run(tc.program+"/"+tc.file, func(t *testing.T) {
			want(t, Classify("test", tc.program, wr1Fixture(t, tc.file)), tc.kind, tc.parser, nil)
		})
	}
	want(t, Classify("test", "npm", []byte("Tests:       4 passed, 4 total\n")), "summary_pass", "jest", nil)
}

// TestWR1_TheRowsFitTheSignedSet: signedRows holds a row per bit of a uint32.
func TestWR1_TheRowsFitTheSignedSet(t *testing.T) {
	if n := len(tables()); n > 32 {
		t.Fatalf("%d rows, signedRows holds 32", n)
	}
}
