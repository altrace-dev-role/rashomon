package store

import "testing"

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

// TestRubricPERF7_RunsBytesIsWhatTheCapCounts: status prints the store's
// size against its cap, and the size is the one eviction compares.
func TestRubricPERF7_RunsBytesIsWhatTheCapCounts(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if n, err := st.RunsBytes(); err != nil || n != 0 {
		t.Fatalf("RunsBytes on an empty store: %d, %v", n, err)
	}
	if err := st.AppendDeclaration(testDecl("s", "toolu_1")); err != nil {
		t.Fatal(err)
	}
	plan, err := evictCandidates(st.Root() + "/" + dirRuns)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := st.RunsBytes(); err != nil || n != plan.total || n == 0 {
		t.Errorf("RunsBytes = %d, %v; eviction counts %d", n, err, plan.total)
	}
}
