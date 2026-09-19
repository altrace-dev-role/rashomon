package posture

import "testing"

// TestIsLoopback is the guard against an escalation ACROSS SESSIONS.
//
// The status file is writable by the same user the agent runs as. An agent in
// one session writes a file naming a host it controls; the next `rashomon run`
// then sends every request -- and, because the session tag rides in the proxy
// URL as userinfo, the tag as well -- to that host in cleartext, before any
// report exists to notice. The refusal is what makes the file's contents
// non-load-bearing for where traffic goes.
func TestIsLoopback(t *testing.T) {
	for _, ok := range []string{
		"127.0.0.1:18080", "[::1]:18080", "localhost:18080",
		"LOCALHOST:18080", "127.0.0.1", "::1",
	} {
		if !isLoopback(ok) {
			t.Errorf("isLoopback(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{
		"10.0.0.5:8080",
		"evil.example:8080",
		// The one a prefix check would have accepted.
		"127.0.0.1.evil.example:80",
		"localhost.evil.example:80",
		// Userinfo smuggled into the address, which would re-point the
		// authority the client parses out of the proxy URL.
		"evil.example:80@127.0.0.1:18080",
		"0.0.0.0:18080", // binds everywhere; not loopback-only
		"",
	} {
		if isLoopback(bad) {
			t.Errorf("isLoopback(%q) = true, want false", bad)
		}
	}
}
