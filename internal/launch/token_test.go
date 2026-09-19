package launch

import (
	"strings"
	"testing"
)

// testKey stands in for the per-install HMAC key.
var testKey = []byte("install-key-for-tests")

// TestNewToken_IsURLSafe. The token travels as the password half of a userinfo
// component. A character that is a delimiter there does not break OUR parser --
// it breaks some client's, at the moment a user is trying to record a session,
// and the failure looks like the proxy being wrong.
func TestNewToken_IsURLSafe(t *testing.T) {
	for i := 0; i < 200; i++ {
		tok := NewToken(testKey)
		for _, bad := range []string{":", "@", "/", "%", "+", "=", "?", "#"} {
			if strings.Contains(tok, bad) {
				t.Fatalf("token %q contains %q, which is a delimiter in a URL userinfo",
					tok, bad)
			}
		}
	}
}

// TestNewToken_IsDistinct. Two runs on one machine must not share a tag, or
// the join it exists to make would merge them -- which is the ambiguity it was
// built to retire.
func TestNewToken_IsDistinct(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		tok := NewToken(testKey)
		if seen[tok] {
			t.Fatalf("token %q minted twice in 1000 draws", tok)
		}
		seen[tok] = true
	}
}

// TestEnv_CarriesTheTokenAsUserinfo, and both spellings, since curl reads only
// the lowercase one.
func TestEnv_CarriesTheTokenAsUserinfo(t *testing.T) {
	env := Env("127.0.0.1:18080", "rt_abc")

	var upper, lower string
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "HTTPS_PROXY="):
			upper = strings.TrimPrefix(kv, "HTTPS_PROXY=")
		case strings.HasPrefix(kv, "https_proxy="):
			lower = strings.TrimPrefix(kv, "https_proxy=")
		}
	}
	want := "http://rashomon:rt_abc@127.0.0.1:18080"
	if upper != want {
		t.Errorf("HTTPS_PROXY = %q, want %q", upper, want)
	}
	if lower != want {
		t.Errorf("https_proxy = %q, want %q -- curl reads only the lowercase form, so a "+
			"token on one and not the other records half a session", lower, want)
	}
}

// TestEnv_WithoutATokenIsUnchanged. The token is opt-in, and a run without one
// must produce exactly what it produced before this existed.
func TestEnv_WithoutATokenIsUnchanged(t *testing.T) {
	for _, kv := range Env("127.0.0.1:18080", "") {
		if strings.Contains(kv, "@") || strings.Contains(kv, ProxyUser) {
			t.Errorf("an untokened launch carries a credential: %q", kv)
		}
	}
}

// TestTokenFromProxyURL_RoundTrips.
func TestTokenFromProxyURL_RoundTrips(t *testing.T) {
	tok := NewToken(testKey)
	env := Env("127.0.0.1:18080", tok)
	got := TokenFromProxyURL(strings.TrimPrefix(env[0], "HTTPS_PROXY="))
	if got != tok {
		t.Errorf("round trip gave %q, want %q", got, tok)
	}
}

// TestTokenFromProxyURL_IgnoresSomebodyElsesCredentials is the one that
// matters for safety rather than function.
//
// A user may already have a corporate proxy configured with real credentials.
// Reading those as a session tag would put a live secret into the join, and --
// worse -- into anything that later digests or reports a tag. The username
// gate is what prevents it, so it is asserted directly.
func TestTokenFromProxyURL_IgnoresSomebodyElsesCredentials(t *testing.T) {
	for _, raw := range []string{
		"http://alice:hunter2@proxy.corp.example:3128",
		"http://svc-account:s3cr3t@10.0.0.1:8080",
		"https://rashomonn:notus@127.0.0.1:18080", // near-miss username
		"http://127.0.0.1:18080",                  // no userinfo at all
		"",
	} {
		if got := TokenFromProxyURL(raw); got != "" {
			t.Errorf("TokenFromProxyURL(%q) = %q; only our own username may yield a token, "+
				"or a user's real proxy password becomes a session tag", raw, got)
		}
	}
}

// TestIsOurs_RejectsWhatWeDidNotSign is the security property the whole
// three-state join now rests on.
//
// The observed agent runs arbitrary shell. It reads HTTPS_PROXY out of its own
// environment, substitutes any password, and routes its traffic through the
// same proxy. If an unequal tag were enough to mean "another session", that row
// would leave the counts AND become ineligible for the reached-but-never-named
// finding -- a one-line opt-out from the product's central claim, handed to the
// subject. The MAC is what makes the question answerable.
func TestIsOurs_RejectsWhatWeDidNotSign(t *testing.T) {
	ours := NewToken(testKey)
	if !IsOurs(ours, testKey) {
		t.Fatal("a tag this key minted did not verify under it")
	}

	for _, bad := range []struct{ name, tok string }{
		{"forged, right shape", "rt_AAAAAAAAAAAAAAAAAAAAAA.BBBBBBBBBBBBBBBBBBBBBB"},
		{"our body, wrong mac", strings.SplitN(ours, ".", 2)[0] + ".AAAAAAAAAAAAAAAAAAAAAA"},
		{"no mac at all", strings.SplitN(ours, ".", 2)[0]},
		{"empty", ""},
		{"not our prefix", "agent-run-42"},
		// The proxy's OWN correlation column value. run_id is a shared
		// namespace -- the proxy fills it from an X-Altrace-Run-ID header --
		// so this is the shape that would have silently deleted an
		// SDK-instrumented client's rows from every tokened report.
		{"proxy correlation id", "9f2c1b7e-4a3d-4f1a-9c2e-7b8a6d5e4f31"},
		{"whitespace", "   "},
		{"truncated ours", ours[:len(ours)-3]},
	} {
		if IsOurs(bad.tok, testKey) {
			t.Errorf("%s: %q verified under our key", bad.name, bad.tok)
		}
	}

	// Another install's key must not verify ours, or two users sharing a proxy
	// would each exclude the other's rows as "another session of mine".
	if IsOurs(ours, []byte("a-different-installs-key")) {
		t.Error("a tag verified under a key that did not mint it")
	}
	// No key means nothing can be proven, so nothing may be excluded. Failing
	// closed here means failing to the PREVIOUS behaviour, never to a stronger
	// claim.
	if IsOurs(ours, nil) {
		t.Error("a tag verified with no key at all")
	}
}

// TestTokenFromProxyURL_AuthorityBoundaries covers the two shapes the first
// version mis-parsed, both found by review rather than by use.
func TestTokenFromProxyURL_AuthorityBoundaries(t *testing.T) {
	// A path containing '@' must not be read as userinfo: the authority ends
	// at the first '/', and LastIndex over the whole string ran into the path.
	if got := TokenFromProxyURL("http://127.0.0.1:18080/p@th"); got != "" {
		t.Errorf("a path '@' was read as userinfo: %q", got)
	}
	if got := TokenFromProxyURL("http://rashomon:rt_x@127.0.0.1:18080/a@b"); got != "rt_x" {
		t.Errorf("got %q, want rt_x: the authority ends at the first '/'", got)
	}
	// The scheme is case-insensitive per RFC 3986. Case-sensitivity made a
	// valid credential read as somebody else's.
	if got := TokenFromProxyURL("HTTP://rashomon:rt_y@127.0.0.1:18080"); got != "rt_y" {
		t.Errorf("got %q, want rt_y: an uppercase scheme is still our URL", got)
	}
}
