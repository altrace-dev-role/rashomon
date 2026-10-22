package shape

import (
	"strings"
	"testing"
	"time"
)

// TestAuthorityWindowBoundsTheScan: authority's terminator set does not
// include '@' (the whole authority has to reach host.Canonical for the
// CWE-522 refusal to see it) or, past an unclosed '[', ']', so a command
// line of nothing but one scheme repeated gave authority no terminator
// before the line's own end: collect calls it once per match, so a line of
// N matches scanned the whole remaining line N times.
//
// The gate here is wall time, not allocation: the extra work past the
// window is a scan over bytes already read, which costs no more heap than
// the bounded scan does (measured: both allocate about 104 KB on the git@
// case). Measured 2026-10-14, darwin/arm64, go1.27.2: bounded, 200 KB of
// git@ and 1.8 MB of an unclosed https://[ take 18.7 ms and 23.2 ms plain,
// 30.5 ms and 165 ms under -race; unbounded (authorityWindow raised past
// both lines), 4.45 s and 2.44 s. The bound below has over 5x the margin
// race adds and well under a tenth of either unbounded time.
func TestAuthorityWindowBoundsTheScan(t *testing.T) {
	const bound = time.Second
	for _, tc := range []struct{ name, line string }{
		{"git@ repeated", strings.Repeat("git@", 50_000)},                          // 200 KB
		{"https://[ repeated, never closed", strings.Repeat("https://[", 200_000)}, // 1.8 MB
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := bashInput(t, tc.line)
			start := time.Now()
			Hosts("Bash", in)
			if took := time.Since(start); took > bound {
				t.Errorf("%s: Hosts took %v, want at most %v: unbounded, authority scans to the line's end at every match", tc.name, took, bound)
			}
		})
	}
}

// TestAuthorityWindowRefusesACutAuthority: past the window, authority refuses
// the candidate rather than handing host.Canonical the window's prefix. A
// credential longer than the window -- a CI token or a JWT in a clone URL's
// userinfo runs to hundreds of bytes -- puts the '@' past it, so the prefix
// is "user:<part of the token>": no '@' left for Canonical to refuse,
// one colon for its port strip to cut at, and the username recorded as the
// host the call named, the CWE-522 outcome authority's doc rules out.
func TestAuthorityWindowRefusesACutAuthority(t *testing.T) {
	line := "git clone https://x-access-token:" + strings.Repeat("t", 300) + "@github.com/org/repo"
	wire, ssh := Hosts("Bash", bashInput(t, line))
	for _, h := range append(wire, ssh...) {
		if h == "x-access-token" {
			t.Errorf("Hosts(%q...) = %q, %q: the userinfo's username recorded as a host", line[:40], wire, ssh)
		}
	}
}
