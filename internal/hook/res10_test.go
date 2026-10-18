package hook

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
)

// cdLine is n leading `cd a &&` steps and then go test.
func cdLine(n int, sep string) string {
	return strings.Repeat("cd a"+sep, n) + "go test ./..."
}

// fastest is the least time any of three runs of f took: a deadline test
// compares the work, and the least of three keeps a busy machine's pauses out
// of the comparison.
func fastest(f func()) time.Duration {
	best := time.Duration(1 << 62)
	for i := 0; i < 3; i++ {
		start := time.Now()
		f()
		best = min(best, time.Since(start))
	}
	return best
}

// TestRES10_LeadingCdFoldIsBounded: the leading-cd fold takes 64 steps and no
// more. At 64 the call's start directory is the payload's cwd with 64 steps
// folded on; at 65 it is not known, which CWDDigest records as unknown,
// rather than a directory the fold stopped short of. Each step joined and
// re-cleaned the whole growing path, so the fold was quadratic: a 480 KB
// payload took 10.9 s against the hook's 5 s timeout. A 1 MB `cd a&&`
// payload, and the longest fold the shape's token bound lets through, each
// allocate a few MiB and no more, and with more than 64 steps each start in
// a directory that is not known.
//
// The bound below is TotalAlloc, not wall time: this project's gates are
// allocated bytes and bounded work, never wall clock (RES-9's own rule).
// Wall time is logged only. Measured 2026-10-14, darwin/arm64, go1.27.2: the
// 1 MB case allocates 5,810,288 B whether or not the race detector is
// instrumenting the binary (it also took 6.6 ms plain and 93.0 ms under
// -race -- an unrelated CPU-time inflation from the detector's
// instrumentation, not from this fold doing more work, which a wall-clock
// gate here could not tell apart from a real regression). A quadratic
// re-break would show up as allocation growing with the payload, which this
// catches regardless of how fast or slow the machine running it is.
//
// A line that runs on past the shape's token bound (RES-9) still has its
// leading steps read: they come first, and 65 of them are 195 tokens, far
// inside the bound. So a cd in front of a long script still moves the start
// directory, and more than 64 in front of one still make it unknown, rather
// than either keeping the payload's cwd, a directory the line moved away
// from.
func TestRES10_LeadingCdFoldIsBounded(t *testing.T) {
	in := func(cmd string) json.RawMessage {
		raw, err := json.Marshal(map[string]string{"command": cmd})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	dirsOf := func(cmd string) []string {
		dirs, _ := shape.LeadingDirectory("Bash", in(cmd))
		return dirs
	}
	want := "/repo" + strings.Repeat("/a", 64)
	if got := startDirectory("/repo", dirsOf(cdLine(64, " && "))); got != want {
		t.Errorf("64 steps: start directory %q, want %q", got, want)
	}
	if got := startDirectory("/repo", dirsOf(cdLine(65, " && "))); got != "" {
		t.Errorf("65 steps: start directory %q, want unknown", got)
	}
	for _, tc := range []struct {
		name  string
		cmd   string
		bound uint64 // TotalAlloc bytes over the 3 runs fastest makes; measured well under this
	}{
		{"1 MB of cd a&&", strings.Repeat("cd a&&", (1<<20)/len("cd a&&")), 36 << 20},
		{"14,000 steps, under the token bound", cdLine(14_000, "&&"), 12 << 20},
	} {
		raw := in(tc.cmd)
		var got string
		var m0, m1 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m0)
		// LeadingDirectory's own tokenizing is inside the timed and measured
		// closure, alongside the fold: this bound is on the two of them
		// together, the same cost startDirectory's caller pays in production
		// (shape.DeriveWithDirs tokenizes once; this measures that reading,
		// not a second one of its own).
		took := fastest(func() {
			dirs, _ := shape.LeadingDirectory("Bash", raw)
			got = startDirectory("/repo", dirs)
		})
		runtime.ReadMemStats(&m1)
		t.Logf("%s: the fold took %v", tc.name, took)
		if alloc := m1.TotalAlloc - m0.TotalAlloc; alloc > tc.bound {
			t.Errorf("%s: the fold allocated %d KiB over 3 runs, want at most %d KiB: a quadratic re-break grows allocation, not just wall time", tc.name, alloc>>10, tc.bound>>10)
		}
		if got != "" {
			t.Errorf("%s: start directory %q, want unknown", tc.name, got)
		}
	}

	// Past the token bound: a long line after the leading steps, of 200,000
	// words, three times the bound (shape's TestRES10_LeadingStepsPastTheTokenBound
	// pins the line it reads past).
	long := " " + strings.TrimSpace(strings.Repeat("word ", 200_000))
	for _, tc := range []struct {
		name, cmd, want string
	}{
		{"one cd, then 200,000 words", "cd /elsewhere && go test" + long, "/elsewhere"},
		{"one cd, then a 200,000-word here-document",
			"cd sub && python3 - <<'EOF'\n" + long + "\nEOF\nmake test", "/repo/sub"},
		{"64 steps, then 200,000 words", cdLine(64, " && ") + long, "/repo" + strings.Repeat("/a", 64)},
		{"100 steps, then 200,000 words", cdLine(100, " && ") + long, ""},
	} {
		if got := startDirectory("/repo", dirsOf(tc.cmd)); got != tc.want {
			t.Errorf("%s: start directory %q, want %q", tc.name, got, tc.want)
		}
	}
}
