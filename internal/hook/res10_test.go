package hook

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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
// finish inside 50 ms.
func TestRES10_LeadingCdFoldIsBounded(t *testing.T) {
	in := func(cmd string) json.RawMessage {
		raw, err := json.Marshal(map[string]string{"command": cmd})
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	want := "/repo" + strings.Repeat("/a", 64)
	if got := startDirectory("/repo", "Bash", in(cdLine(64, " && "))); got != want {
		t.Errorf("64 steps: start directory %q, want %q", got, want)
	}
	if got := startDirectory("/repo", "Bash", in(cdLine(65, " && "))); got != "" {
		t.Errorf("65 steps: start directory %q, want unknown", got)
	}
	for _, tc := range []struct {
		name string
		cmd  string
	}{
		{"1 MB of cd a&&", strings.Repeat("cd a&&", (1<<20)/len("cd a&&"))},
		{"14,000 steps, under the token bound", cdLine(14_000, "&&")},
	} {
		raw := in(tc.cmd)
		var got string
		took := fastest(func() { got = startDirectory("/repo", "Bash", raw) })
		if took > 50*time.Millisecond {
			t.Errorf("%s: the fold took %v, want at most 50ms", tc.name, took)
		}
		if strings.Count(got, "/a") > 64 {
			t.Errorf("%s: %d steps folded, want at most 64", tc.name, strings.Count(got, "/a"))
		}
	}
}
