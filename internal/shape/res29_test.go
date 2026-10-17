package shape

import (
	"strings"
	"testing"
	"time"
)

// TestRES29_NestingAtAndPastMaxNest: maxNest is the only barrier between a
// line of nested substitutions and a stack overflow, which is a fatal error
// that exits 2 -- and exit 2 from PreToolUse blocks the call. One row per
// nesting construct that recurses, at the deepest level each accepts and one
// past it. Each line is `cd "..." && ls`: read to its end, the cd's argument
// closes, the && after it is found, and the program is ls; past the bound the
// reading gives up inside the word, where the command after cd cannot be
// found, and the program is null. Either way the answer comes at once.
//
// A "$(echo " pair costs two levels, the substitution and the quoted string
// inside it, so the flip is between 16 and 17 pairs; a bare $( ) or a $(( ))
// inside another costs one, so it is between 33 and 34.
//
// The gate below is TotalAlloc, not wall time, as this package's gates
// always are (RES-9's own rule): wall time is logged only. Measured
// 2026-10-14, darwin/arm64, go1.27.2: each case allocates about 11 KiB.
func TestRES29_NestingAtAndPastMaxNest(t *testing.T) {
	quoted := func(n int) string {
		return `cd "` + strings.Repeat(`$(echo "`, n) + "x" + strings.Repeat(`")`, n) + `" && ls`
	}
	bare := func(n int) string {
		return `cd "` + strings.Repeat("$(echo ", n) + "x" + strings.Repeat(")", n) + `" && ls`
	}
	arith := func(n int) string {
		return `cd "` + strings.Repeat("$(( 1 + ", n) + "1" + strings.Repeat(" ))", n) + `" && ls`
	}
	for _, tc := range []struct {
		name     string
		line     func(int) string
		deepest  int
		perLevel int
	}{
		{"a quoted string inside each substitution", quoted, 16, 2},
		{"a substitution inside a substitution", bare, 33, 1},
		{"arithmetic inside arithmetic", arith, 33, 1},
	} {
		if (tc.deepest-1)*tc.perLevel > maxNest {
			t.Fatalf("%s: %d levels is past maxNest %d before the last one", tc.name, tc.deepest, maxNest)
		}
		for _, n := range []int{tc.deepest, tc.deepest + 1} {
			in := bashInput(t, tc.line(n))
			const bound = 1 << 20
			var s Shape
			var took time.Duration
			alloc := allocated(func() {
				start := time.Now()
				s = Derive("Bash", in, benchKey)
				took = time.Since(start)
			})
			t.Logf("%s, %d levels: %v, allocated %d KiB", tc.name, n, took, alloc>>10)
			if alloc > bound {
				t.Errorf("%s, %d levels: allocated %d KiB, want at most %d", tc.name, n, alloc>>10, bound>>10)
			}
			accepted := n == tc.deepest
			switch {
			case accepted && (s.Program == nil || *s.Program != "ls"):
				t.Errorf("%s, %d levels (the deepest accepted): program %v, want ls", tc.name, n, show(s.Program))
			case !accepted && s.Program != nil:
				t.Errorf("%s, %d levels (one past maxNest): program %s, want null", tc.name, n, *s.Program)
			}
		}
	}
}
