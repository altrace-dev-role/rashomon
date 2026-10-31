package gateout

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

func TestGO1_GoTestOkIsSummaryPass(t *testing.T) {
	want(t, Classify("test", "go", []byte("ok\tpkg\t0.01s\n")), "summary_pass", "go", nil)
}

func TestGO2_GoTestFailLineIsFailLine(t *testing.T) {
	want(t, Classify("test", "go", []byte("--- FAIL: TestBad (0.00s)\n")), "fail_line", "go", nil)
}

func TestGO3_NoTestFilesIsEmptyRun(t *testing.T) {
	want(t, Classify("test", "go", []byte("?\tpkg\t[no test files]\n")), "empty_run", "go", nil)
}

func TestGO4_PytestCounts(t *testing.T) {
	for _, tc := range []struct{ output, kind string }{
		{"5 passed in 0.2s", "summary_pass"}, {"2 failed, 1 passed in 0.31s", "summary_fail"}, {"2 errors in 0.05s", "summary_fail"},
		{"no tests ran in 0.0s", "empty_run"},
	} {
		want(t, Classify("test", "pytest", []byte(tc.output)), tc.kind, "pytest", nil)
	}
}

func TestGO5_JestSummary(t *testing.T) {
	for _, tc := range []struct{ output, kind string }{
		{"Tests: 4 passed, 4 total", "summary_pass"}, {"Tests: 2 failed, 2 passed, 4 total", "summary_fail"},
	} {
		want(t, Classify("test", "jest", []byte(tc.output)), tc.kind, "jest", nil)
	}
}

func TestGO6_CargoResult(t *testing.T) {
	for _, tc := range []struct{ output, kind string }{
		{"test result: ok. 1 passed; 0 failed", "summary_pass"}, {"test result: FAILED. 0 passed; 1 failed", "summary_fail"},
	} {
		want(t, Classify("test", "cargo", []byte(tc.output)), tc.kind, "cargo", nil)
	}
}

func TestGO7_Delegates(t *testing.T) {
	for _, tc := range []struct{ program, output, kind, parser string }{
		{"make", "ok\tpkg\t0.01s", "summary_pass", "go"},
		{"npm", "Tests: 4 passed, 4 total", "summary_pass", "jest"},
		{"yarn", "Tests 1 passed", "summary_pass", "vitest"},
		{"yarn", "Tests 1 failed | 2 passed", "summary_fail", "vitest"},
		{"pnpm", "no recognizable result", "no_summary", "pnpm"},
	} {
		want(t, Classify("test", tc.program, []byte(tc.output)), tc.kind, tc.parser, nil)
	}
}

func TestGO8_UnknownProgramIsUnattributable(t *testing.T) {
	want(t, Classify("test", "mocha", []byte("1 passing")), "unattributable", "", nil)
}

func TestGO9_CapBoundary(t *testing.T) {
	exact := make([]byte, MaxScanBytes)
	copy(exact, []byte("ok\tpkg\t0.01s"))
	want(t, Classify("test", "go", exact), "summary_pass", "go", nil)
	over := append(exact, 'x')
	want(t, Classify("test", "go", over), "unread", "", stringPointer("too_large"))
}

func TestGO10_TablesPinned(t *testing.T) {
	got := tableDigest()
	const pin = "e110082cbc677ed5e5cce54637a1fe39d6a359ad6f4eb60cddddbaa0ad9f9cd8"
	if got != pin {
		t.Fatalf("table digest %s, want %s", got, pin)
	}
	for _, row := range tables() {
		for _, group := range [][]*regexp.Regexp{row.pass, row.fail, row.failLine, row.empty} {
			for _, pattern := range group {
				if _, reason, _ := spec.CheckRegex(pattern.String()); reason != "" {
					t.Errorf("%s pattern %q rejected: %s", row.program, pattern, reason)
				}
			}
		}
	}
}

func TestGO11_GoTestFailSummaryIsSummaryFail(t *testing.T) {
	want(t, Classify("test", "go", []byte("--- FAIL: TestBad (0.00s)\nFAIL\tpkg\t0.01s\n")), "summary_fail", "go", nil)
}

func TestGO12_FailBeatsPassAcrossPackages(t *testing.T) {
	want(t, Classify("test", "go", []byte("ok\tpkg/a\t0.01s\nFAIL\tpkg/b\t0.01s\nok\tpkg/c\t0.01s\n")), "summary_fail", "go", nil)
}

func TestGO13_PassBeatsEmptyAcrossPackages(t *testing.T) {
	want(t, Classify("test", "go", []byte("ok\tpkg/a\t0.01s\n?\tpkg/z\t[no test files]\n")), "summary_pass", "go", nil)
}

func TestGO14_OkWithNoTestsToRunIsEmptyRun(t *testing.T) {
	want(t, Classify("test", "go", []byte("ok\tpkg\t0.01s [no tests to run]\n")), "empty_run", "go", nil)
}

func TestGO15_AttributableWithoutSummaryIsNoSummary(t *testing.T) {
	want(t, Classify("test", "go", []byte("# pkg\n./pkg.go:2: undefined: nope\n")), "no_summary", "go", nil)
}

func TestGO16_BuildKindIsNoSummary(t *testing.T) {
	want(t, Classify("build", "go", []byte("ok\tpkg\t0.01s")), "no_summary", "go", nil)
}

func TestGO17_ReasonIsNilExceptTooLarge(t *testing.T) {
	for _, tc := range []struct{ kind, program, output string }{
		{"test", "go", "ok\tpkg\t0.01s"}, {"test", "pytest", "no tests ran"},
		{"test", "unknown", ""}, {"build", "go", "anything"},
	} {
		if got := Classify(tc.kind, tc.program, []byte(tc.output)); got.Reason != nil {
			t.Errorf("%s/%s Reason = %v, want nil", tc.kind, tc.program, got.Reason)
		}
	}
}

func TestGO18_MultipleRunnerRowsRetainTheDecidingParser(t *testing.T) {
	got := Classify("test", "make", []byte("Tests: 1 passed\n"))
	want(t, got, "summary_pass", "jest", nil)
}

func TestGO19_GoFixtures(t *testing.T) {
	for _, tc := range []struct{ name, kind string }{
		{"pass.txt", "summary_pass"}, {"fail-line.txt", "fail_line"}, {"empty.txt", "empty_run"},
		{"fail-summary.txt", "summary_fail"}, {"multi-package.txt", "summary_fail"},
		{"empty-after-pass.txt", "summary_pass"},
	} {
		data, err := os.ReadFile(filepath.Join("testdata", "go", tc.name))
		if err != nil {
			t.Fatal(err)
		}
		want(t, Classify("test", "go", data), tc.kind, "go", nil)
	}
}

func TestGO20_OtherProgramFixtures(t *testing.T) {
	for _, tc := range []struct{ program, filename, kind string }{
		{"pytest", "pass.txt", "summary_pass"}, {"pytest", "fail.txt", "summary_fail"}, {"pytest", "empty.txt", "empty_run"},
		{"jest", "pass.txt", "summary_pass"}, {"jest", "fail.txt", "summary_fail"},
		{"vitest", "pass.txt", "summary_pass"}, {"vitest", "fail.txt", "summary_fail"},
		{"cargo", "pass.txt", "summary_pass"}, {"cargo", "fail.txt", "summary_fail"},
	} {
		data, err := os.ReadFile(filepath.Join("testdata", tc.program, tc.filename))
		if err != nil {
			t.Fatal(err)
		}
		want(t, Classify("test", tc.program, data), tc.kind, tc.program, nil)
	}
}

func TestGO21_FailLineBeatsPassAcrossPackages(t *testing.T) {
	want(t, Classify("test", "go", []byte("--- FAIL: TestBad (0.00s)\nok\tpkg/other\t0.01s\n")), "fail_line", "go", nil)
}

func BenchmarkClassify(b *testing.B) {
	pass := []byte("ok\tpkg\t0.01s\n")
	fail := []byte("--- FAIL: TestBad (0.00s)\n" + strings.Repeat("x", MaxScanBytes-len("--- FAIL: TestBad (0.00s)\n")))
	b.Run("go_pass", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(pass)))
		_ = Classify("test", "go", pass) // Initialize regexp match state before measurement.
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = Classify("test", "go", pass)
		}
	})
	b.Run("go_fail_at_cap", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(fail)))
		_ = Classify("test", "go", fail) // Initialize regexp match state before measurement.
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = Classify("test", "go", fail)
		}
	})
}

func want(t *testing.T, got store.GateOutput, kind, parser string, reason *string) {
	t.Helper()
	if got.Kind != kind || got.Parser != parser || !sameStringPointer(got.Reason, reason) {
		t.Fatalf("got %+v, want kind=%s parser=%s reason=%v", got, kind, parser, reason)
	}
}

func tableDigest() string {
	h := sha256.New()
	for _, row := range tables() {
		fmt.Fprintf(h, "%s\x00%t\x00", row.program, row.delegate)
		for _, group := range [][]*regexp.Regexp{row.pass, row.fail, row.failLine, row.empty} {
			for _, pattern := range group {
				fmt.Fprintf(h, "%s\x00", pattern.String())
			}
			fmt.Fprint(h, "\x01")
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

func stringPointer(s string) *string { return &s }

func sameStringPointer(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// MASKED-OUTCOME1 (D229): a piped run's output reaches gateout as the pipe
// kept it, usually its last lines, and the runner is named by the run's own
// words.

func TestMO1_GoBareFailIsSummaryFail(t *testing.T) {
	// go prints one bare FAIL at the end of a failed multi-package run, which
	// a `| tail` may keep with no package line beside it.
	for _, name := range []string{"bare-fail-tail.txt", "bare-fail-only.txt"} {
		data, err := os.ReadFile(filepath.Join("testdata", "go", name))
		if err != nil {
			t.Fatal(err)
		}
		want(t, Classify("test", "go", data), "summary_fail", "go", nil)
	}
	// Only the bare line: not FAIL as a word of another line.
	want(t, Classify("test", "go", []byte("FAILURE\nFAIL: x\n")), "no_summary", "go", nil)
}

func TestMO1_CargoErrorLinesAreSummaryFail(t *testing.T) {
	// cargo test --no-fail-fast ends with a "test result: ok" for the last
	// target and reports the failure on stderr.
	for _, name := range []string{"error-test-failed.txt", "error-targets-failed.txt"} {
		data, err := os.ReadFile(filepath.Join("testdata", "cargo", name))
		if err != nil {
			t.Fatal(err)
		}
		want(t, Classify("test", "cargo", data), "summary_fail", "cargo", nil)
		both := append([]byte("test result: ok. 3 passed; 0 failed\n"), data...)
		want(t, Classify("test", "cargo", both), "summary_fail", "cargo", nil)
	}
	want(t, Classify("test", "cargo", []byte("error: 1 target failed\n")), "summary_fail", "cargo", nil)
	want(t, Classify("test", "cargo", []byte("error: could not compile `x`\n")), "no_summary", "cargo", nil)
}

func TestMO1_PytestUnderATail(t *testing.T) {
	for _, tc := range []struct{ name, kind string }{
		{"tail-pass.txt", "summary_pass"}, {"tail-fail.txt", "summary_fail"},
	} {
		data, err := os.ReadFile(filepath.Join("testdata", "pytest", tc.name))
		if err != nil {
			t.Fatal(err)
		}
		want(t, Classify("test", "pytest", data), tc.kind, "pytest", nil)
	}
}

func TestMO1_ProgramForNamesTheLaunchedRunner(t *testing.T) {
	for _, tc := range []struct{ program, runner, want string }{
		{"python3", "python3 -m pytest", "pytest"},
		{"python", "python -m pytest", "pytest"},
		{"uv", "uv run pytest", "pytest"},
		{"npx", "npx jest", "jest"},
		{"npx", "npx vitest", "vitest"},
		{"go", "go test", "go"},
		{"cargo", "cargo test", "cargo"},
		{"make", "make test", "make"},
		{"npm", "npm run test", "npm"},
		{"pytest", "pytest", "pytest"},
		// No table names these: the program stays, and Classify says
		// unattributable for it as it did.
		{"python3", "python3 -m unittest", "python3"},
		{"bun", "bun test", "bun"},
		{"python3", "", "python3"},
		{"python3", "something else", "python3"},
	} {
		if got := ProgramFor(tc.program, tc.runner); got != tc.want {
			t.Errorf("ProgramFor(%q, %q) = %q, want %q", tc.program, tc.runner, got, tc.want)
		}
	}
	want(t, Classify("test", ProgramFor("python3", "python3 -m pytest"), []byte("3 passed in 0.1s\n")), "summary_pass", "pytest", nil)
	want(t, Classify("test", ProgramFor("python3", ""), []byte("3 passed in 0.1s\n")), "unattributable", "", nil)
}

// TestMO1_EveryRunnerHasASummaryForm: each table row that reads a run itself
// has a fail pattern of its own, the runner's own verdict, so a fail line
// alone (`--- FAIL`, a substring a pipe may keep out of a run that then
// passed) decides for no row. A new row without one must say so here, and its
// readers then rest on its fail line.
func TestMO1_EveryRunnerHasASummaryForm(t *testing.T) {
	for _, row := range tables() {
		if row.delegate {
			continue
		}
		if len(row.pass) == 0 || len(row.fail) == 0 {
			t.Errorf("%s has no summary form (pass %d, fail %d patterns)", row.program, len(row.pass), len(row.fail))
		}
		if FailLineDecides(row.program) {
			t.Errorf("FailLineDecides(%q) is true, but its table has a summary form", row.program)
		}
	}
	for _, p := range []string{"make", "npm", "yarn", "pnpm", "unknown", ""} {
		if FailLineDecides(p) {
			t.Errorf("FailLineDecides(%q) = true: a delegate or unknown program is no row without a summary form", p)
		}
	}
}

// TestMO1_ADelegatedFailureMarkerIsSummaryFail: make, npm, yarn and pnpm run
// other runners, so a step's pass line is no verdict on the run. Each one's
// own failure marker decides, and fail outranks the pass line before it.
// Break: drop a row's fail pattern and its run reads summary_pass.
func TestMO1_ADelegatedFailureMarkerIsSummaryFail(t *testing.T) {
	for _, tc := range []struct{ name, program, output, kind string }{
		{"make after pytest", "make", "== 5 passed in 1s ==\nerror: lint failed\nmake: *** [lint] Error 2", "summary_fail"},
		{"make after go test", "make", "ok  \tx/a\t0.1s\nmake: *** [vet] Error 1", "summary_fail"},
		{"npm after jest", "npm", "Tests: 5 passed, 5 total\nnpm ERR! code ELIFECYCLE\nnpm ERR! errno 1", "summary_fail"},
		{"recursive make", "make", "ok  \tx/a\t0.1s\nmake[2]: *** [sub] Error 1", "summary_fail"},
		{"npm error", "npm", "Tests: 5 passed, 5 total\nnpm error code 1", "summary_fail"},
		{"npm lifecycle alone", "npm", "Tests: 5 passed, 5 total\nELIFECYCLE  Command failed.", "summary_fail"},
		{"yarn", "yarn", "Tests 5 passed (5)\nerror Command failed with exit code 1.", "summary_fail"},
		{"pnpm", "pnpm", "Tests 5 passed (5)\nERR_PNPM_RECURSIVE_RUN_FIRST_FAIL x", "summary_fail"},
		{"make running npm", "make", "Tests: 5 passed, 5 total\nnpm ERR! code ELIFECYCLE", "summary_fail"},
		{"a clean make test | tail", "make", "ok  \tx/a\t0.1s\nok  \tx/b\t0.2s", "summary_pass"},
		{"a make line that is not a failure", "make", "ok  \tx/a\t0.1s\nmake: Nothing to be done for 'x'.", "summary_pass"},
	} {
		got := Classify("test", tc.program, []byte(tc.output))
		if got.Kind != tc.kind {
			t.Errorf("%s: %s, want %s", tc.name, got.Kind, tc.kind)
		}
	}
	for p, want := range map[string]bool{"make": true, "npm": true, "yarn": true, "pnpm": true, "go": false, "pytest": false, "unknown": false, "": false} {
		if Delegated(p) != want {
			t.Errorf("Delegated(%q) = %v, want %v", p, !want, want)
		}
	}
}

// TestMO1_PytestPassIsItsSummaryLine: pytest's pass is its summary line, the
// -q form ("5 passed in 0.12s") or the banner form, never "N passed" inside
// another line, and a crash that leaves a stray count is red. Break: unanchor
// the pass pattern, or drop a crash marker.
func TestMO1_PytestPassIsItsSummaryLine(t *testing.T) {
	for _, tc := range []struct{ name, output, kind string }{
		{"q form", "5 passed in 0.12s", "summary_pass"},
		{"q form with skips and warnings", "3 skipped, 5 passed, 1 warning in 0.12s", "summary_pass"},
		{"q form with a duration", "5 passed in 0.12s (0:00:00)", "summary_pass"},
		{"banner", "============== 5 passed in 0.12s ==============", "summary_pass"},
		{"banner with warnings", "===== 5 passed, 2 warnings in 1.50s =====", "summary_pass"},
		{"a stray count in another line", "migrated 5 passed", "no_summary"},
		{"a stray count after a segfault", ".....\nmigrated 5 passed\nSegmentation fault (core dumped)", "no_summary"},
		{"a passed count without its timing", "5 passed", "no_summary"},
		{"fatal python error", "5 passed in 0.12s\nFatal Python error: Segmentation fault", "summary_fail"},
		{"oom kill", "5 passed in 0.12s\nKilled", "summary_fail"},
	} {
		got := Classify("test", "pytest", []byte(tc.output))
		if got.Kind != tc.kind {
			t.Errorf("%s: %s, want %s", tc.name, got.Kind, tc.kind)
		}
	}
}
