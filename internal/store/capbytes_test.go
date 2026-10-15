package store

import (
	"os"
	"testing"
)

// TestRubricHLD7_CapIsADecimalByteCount: RASHOMON_STORE_CAP_BYTES decides
// when runs are deleted, and was never checked. "1GiB" and "1e9" fell back
// to the default without a word, and "-1" turned eviction off. Now a plain
// decimal byte count is the only value taken as one, zero is eviction off,
// and anything else is invalid and says so, with the default in force.
func TestRubricHLD7_CapIsADecimalByteCount(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		bytes   int64
		invalid bool
	}{
		{"", DefaultCapBytes, false},
		{"1048576", 1 << 20, false},
		{"0", 0, false},
		{"2GB", DefaultCapBytes, true},
		{"1GiB", DefaultCapBytes, true},
		{"1e9", DefaultCapBytes, true},
		{"-1", DefaultCapBytes, true},
		{"+5", DefaultCapBytes, true},
		{" 5", DefaultCapBytes, true},
		{"9223372036854775808", DefaultCapBytes, true},
	} {
		got := parseCap(tc.raw)
		if got.Bytes != tc.bytes || got.Invalid != tc.invalid || got.Raw != tc.raw {
			t.Errorf("parseCap(%q) = %+v, want %d bytes, invalid %v", tc.raw, got, tc.bytes, tc.invalid)
		}
		if got.Off() != (tc.bytes == 0) {
			t.Errorf("parseCap(%q).Off() = %v", tc.raw, got.Off())
		}
		if (got.Problem() != "") != tc.invalid {
			t.Errorf("parseCap(%q).Problem() = %q, invalid %v", tc.raw, got.Problem(), tc.invalid)
		}
	}
}

// TestRubricHLD7_CapBytesReadsTheEnvironment: CapBytes is what the probe
// hands Evict, so the parse has to reach it.
func TestRubricHLD7_CapBytesReadsTheEnvironment(t *testing.T) {
	for raw, want := range map[string]int64{"0": 0, "4096": 4096, "-1": DefaultCapBytes, "2GB": DefaultCapBytes} {
		t.Setenv(EnvCapBytes, raw)
		if got := CapBytes(); got != want {
			t.Errorf("CapBytes() with %s=%q is %d, want %d", EnvCapBytes, raw, got, want)
		}
	}
}

// TestHLD7_ResolvedCapRoundTripsWhatAHookRecorded: ResolvedCap is status's
// half of the fix -- eviction resolves the cap inside a hook's own process,
// and a later status call, in a different shell, must read that value back
// rather than its own environment (hld-7's second half).
func TestHLD7_ResolvedCapRoundTripsWhatAHookRecorded(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := st.ResolvedCap(); err != nil || ok {
		t.Fatalf("ResolvedCap on a store no hook has touched: ok=%v, err=%v, want ok=false", ok, err)
	}

	if err := st.RecordResolvedCap(parseCap("1048576")); err != nil {
		t.Fatal(err)
	}
	rc, ok, err := st.ResolvedCap()
	if err != nil || !ok || rc.Bytes != 1<<20 || rc.Problem != "" {
		t.Fatalf("ResolvedCap() = %+v, ok=%v, err=%v, want 1048576 bytes, no problem", rc, ok, err)
	}

	if err := st.RecordResolvedCap(parseCap("2GB")); err != nil {
		t.Fatal(err)
	}
	rc, ok, err = st.ResolvedCap()
	if err != nil || !ok || rc.Bytes != DefaultCapBytes || rc.Problem == "" {
		t.Fatalf("ResolvedCap() after a malformed value = %+v, ok=%v, err=%v, want the default and a problem", rc, ok, err)
	}
}

// TestRubricPERF7_RunsBytesIsWhatTheCapCounts: status prints the store's
// size against its cap, and the size is the one eviction compares.
func TestRubricPERF7_RunsBytesIsWhatTheCapCounts(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if n, unlisted, err := st.RunsBytes(); err != nil || n != 0 || unlisted != 0 {
		t.Fatalf("RunsBytes on an empty store: %d, %d, %v", n, unlisted, err)
	}
	if err := st.AppendDeclaration(testDecl("s", "toolu_1")); err != nil {
		t.Fatal(err)
	}
	plan, err := evictCandidates(st.Root() + "/" + dirRuns)
	if err != nil {
		t.Fatal(err)
	}
	if n, unlisted, err := st.RunsBytes(); err != nil || n != plan.total || n == 0 || unlisted != 0 {
		t.Errorf("RunsBytes = %d, %d, %v; eviction counts %d", n, unlisted, err, plan.total)
	}
}

// TestRunsBytes_CountsWhatItCouldNotList: a run eviction cannot list is left
// out of the sum and named in the error, same as Evict's own plan. The
// total of what could be listed is still useful on its own, so it comes
// back together with how many runs were left out, rather than forcing a
// caller to throw away a correct partial total because one run's size is
// unknown.
func TestRunsBytes_CountsWhatItCouldNotList(t *testing.T) {
	permissionsBite(t)
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"sess-fine", "sess-unlistable"} {
		if err := st.AppendDeclaration(testDecl(s, "toolu_"+s)); err != nil {
			t.Fatal(err)
		}
	}
	fine, err := evictCandidates(st.Root() + "/" + dirRuns)
	if err != nil {
		t.Fatal(err)
	}
	var fineSize int64
	for _, c := range fine.cands {
		if c.name == segment("sess-fine") {
			fineSize = c.size
		}
	}
	if err := os.Chmod(st.RunDir("sess-unlistable"), 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(st.RunDir("sess-unlistable"), dirMode) //nolint:errcheck // best-effort cleanup

	n, unlisted, err := st.RunsBytes()
	if err == nil {
		t.Fatalf("RunsBytes with an unlistable run: no error, want one naming it")
	}
	if unlisted != 1 {
		t.Errorf("RunsBytes with one unlistable run reports unlisted=%d, want 1", unlisted)
	}
	if n != fineSize {
		t.Errorf("RunsBytes with one unlistable run = %d, want the listable runs' total %d", n, fineSize)
	}
}

// TestFormatBytes_RoundingNeverCrossesAUnitWithoutMovingTheSuffix:
// FormatBytes picks its unit before rounding to one decimal place, so a
// value just under a unit's boundary could round up to look like a whole
// unit of the next one while keeping the smaller suffix: 1048575 B is one
// byte under 1 MiB, but printed "1024 KiB" instead of advancing to MiB.
func TestFormatBytes_RoundingNeverCrossesAUnitWithoutMovingTheSuffix(t *testing.T) {
	for _, tc := range []struct {
		n    int64
		want string
	}{
		{1048575, "1 MiB"}, // one byte under 1 MiB; rounds to 1024.0 KiB unfixed
		{1048576, "1 MiB"},
		{1023, "1023 B"},
		{1024, "1 KiB"},
	} {
		if got := FormatBytes(tc.n); got != tc.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
