package peakrss

import "testing"

// TestRES28i_MaxrssIsReadInEachPlatformsUnit: wait4's ru_maxrss is bytes on
// darwin and ios and KiB on every other Unix, so the same 6.5 MiB peak reads
// as 6.5 MiB on both. Break: read every platform's figure as bytes (or as
// KiB), and one side of this table is off by a factor of 1024, which would
// put every RSS gate (RES-7, RES-28) a thousand times loose or tight.
func TestRES28i_MaxrssIsReadInEachPlatformsUnit(t *testing.T) {
	const peak = 6.5 * (1 << 20) // bytes
	for _, c := range []struct {
		goos   string
		maxrss int64
	}{
		{"darwin", peak},
		{"ios", peak},
		{"linux", peak / 1024},
		{"freebsd", peak / 1024},
		{"netbsd", peak / 1024},
		{"openbsd", peak / 1024},
	} {
		if got := fromMaxrss(c.maxrss, c.goos); got != 6.5 {
			t.Errorf("%s: maxrss %d reads as %v MiB, want 6.5", c.goos, c.maxrss, got)
		}
	}
}
