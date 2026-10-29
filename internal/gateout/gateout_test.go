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
		{"5 passed in 0.2s", "summary_pass"}, {"2 failed, 1 passed", "summary_fail"}, {"2 error", "summary_fail"},
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
	const pin = "21b4d3f8976494232404a913dc875ab42adb57d3c679a3d9b07d065aa9ecbef6"
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
