package gateout

import "testing"

// GATEOUT-ZERO1 (R29): a count of zero, or a log line that holds a count, is
// not a runner's failure. pytest's fail form was a bare `\d+ (failed|error)`,
// matched anywhere in a line and on a count of 0, and a delegated run (make,
// npm, yarn, pnpm) is matched against every table, so the success line of
// another tool in a green `make test` read summary_fail and the gate red.

// TestGZ1_AZeroCountOrALogLineIsNotAFailure: each output is a run that
// passed. Break: unanchor pytest's fail form, or let its count be 0 (the
// "in 12 files" row).
func TestGZ1_AZeroCountOrALogLineIsNotAFailure(t *testing.T) {
	for _, tc := range []struct{ name, program, output, kind, parser string }{
		{"cargo's green summary under make", "make",
			"running 5 tests\ntest result: ok. 5 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.00s\n",
			"summary_pass", "cargo"},
		{"a checker's clean line under npm", "npm", "svelte-check found 0 errors and 0 warnings\n", "no_summary", "npm"},
		{"a type checker's clean summary under make", "make", "0 errors, 0 warnings, 0 informations\n", "no_summary", "make"},
		{"a zero count with a pytest-like tail under make", "make", "0 errors in 12 files\n", "no_summary", "make"},
		{"a test's own log under pytest", "pytest",
			"retry: 2 failed attempts\n2 failed attempts, giving up on the mirror\n========== 3 passed in 0.12s ==========\n",
			"summary_pass", "pytest"},
	} {
		got := Classify("test", tc.program, []byte(tc.output))
		if got.Kind != tc.kind || got.Parser != tc.parser {
			t.Errorf("%s: %s parser %q, want %s parser %q", tc.name, got.Kind, got.Parser, tc.kind, tc.parser)
		}
	}
}

// TestGZ1_PytestSummariesStillFail: the anchored forms keep every summary
// pytest prints for a failure, in both its forms, and a delegate still reads
// a real pytest failure. Break: anchor the forms past them.
func TestGZ1_PytestSummariesStillFail(t *testing.T) {
	for _, tc := range []struct{ program, output, parser string }{
		{"pytest", "1 failed, 2 passed in 0.12s\n", "pytest"},
		{"pytest", "2 passed, 1 failed in 0.12s\n", "pytest"},
		{"pytest", "3 passed, 2 warnings, 1 error in 1.03s\n", "pytest"},
		{"pytest", "3 errors in 0.05s\n", "pytest"},
		{"pytest", "1 failed, 1 passed, 2 warnings, 1 error in 61.20s (0:01:01)\n", "pytest"},
		{"pytest", "========== 1 failed, 2 passed in 0.12s ==========\n", "pytest"},
		{"pytest", "=================== 10 errors in 2.31s ===================\n", "pytest"},
		{"make", "..F\n========== 1 failed, 2 passed in 0.12s ==========\n", "pytest"},
	} {
		got := Classify("test", tc.program, []byte(tc.output))
		if got.Kind != "summary_fail" || got.Parser != tc.parser {
			t.Errorf("%s %q: %s parser %q, want summary_fail parser %q", tc.program, tc.output, got.Kind, got.Parser, tc.parser)
		}
	}
}
