package launch

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strings"
)

// The session tag is `rt_<random>.<mac>`.
//
// THE MAC IS THE POINT, and the first version of this had only the random
// half. A tag that is merely unguessable answers "could another process
// invent this one?" -- but the question that decides what the report may do
// with a FOREIGN tag is "did rashomon issue this at all?", and entropy cannot
// answer it. Three reviews converged on the same consequence: without a way to
// verify issuance, any non-empty run_id that was not ours had to be treated as
// another rashomon session and excluded, and two things then followed.
//
// First, the observed agent reads HTTPS_PROXY out of its own environment --
// it runs arbitrary shell, that is the whole premise -- substitutes any
// password, and routes its traffic through the same proxy. Those rows became
// "another session's", which dropped them from the reached-but-never-named
// finding and from novelty. The feature handed the subject a one-line opt-out
// from the product's central claim.
//
// Second, and not hypothetical: the proxy this reads already populates run_id
// from an X-Altrace-Run-ID header (comp/proxy/interceptor/connect_correlation.go).
// The column is a shared namespace, not a rashomon-private one, so any
// SDK-instrumented client sharing the proxy would have had its rows deleted
// from the report and attributed to a session that never existed.
//
// With a MAC under the install key, a tag can be TESTED. Anything that does
// not verify is not evidence about anybody, and falls back to the clock --
// which is exactly as informed as before this feature existed.
const (
	tokenPrefix    = "rt_"
	tokenRandBytes = 16 // 128 bits of unguessability
	tokenMACBytes  = 16 // 128 bits of unforgeability
	tokenDomain    = "rashomon-session-tag\x00"
)

// NewToken mints a tag this install can later prove it issued.
//
// crypto/rand, not math/rand: a predictable random half could be replayed by
// another process to have its traffic recorded as this session's. rand.Read
// cannot fail on any supported platform -- it panics internally rather than
// returning a short read -- so there is no error to hand back.
//
// Base64 URL-alphabet, unpadded, and a '.' separator: the value travels as the
// password half of a userinfo component, so it must contain no ':', '@', '/'
// or '%'. Standard base64 would emit '+' and '/', and '=' padding is a
// delimiter to some parsers -- each of which survives our tests and breaks on
// somebody else's client.
func NewToken(key []byte) string {
	buf := make([]byte, tokenRandBytes)
	rand.Read(buf) //nolint:errcheck // documented above: cannot fail
	body := tokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	return body + "." + mac(body, key)
}

func mac(body string, key []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(tokenDomain))
	h.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil)[:tokenMACBytes])
}

// IsOurs reports whether this install issued the tag.
//
// The ONLY question it answers. It does not say which run, and it is not an
// authorisation: a tag that verifies came from a `rashomon run` under this
// install, which is what earns a row the right to be attributed to a run other
// than this one and therefore excluded from it.
//
// An empty key means this install has no key to test against, so nothing can
// be proven and everything falls back to the clock. Failing closed here means
// failing to the PREVIOUS behaviour, never to a stronger claim.
//
// crypto/subtle for the comparison. The value is not a password and an
// attacker who can time this can read the store outright -- but a MAC check is
// the one comparison in this program where the constant-time habit costs
// nothing and its absence is a genuine smell to the next reader.
func IsOurs(token string, key []byte) bool {
	if len(key) == 0 || !strings.HasPrefix(token, tokenPrefix) {
		return false
	}
	body, sum, ok := strings.Cut(token, ".")
	if !ok {
		return false
	}
	want := mac(body, key)
	return subtle.ConstantTimeCompare([]byte(sum), []byte(want)) == 1
}

// TokenFromProxyURL reads the tag back out of a proxy URL.
//
// Parsed by hand rather than with net/url: the shape is fixed and we wrote it,
// so a general URL parser would be accepting inputs this never produces.
//
// Returns "" for anything that is not our credential, INCLUDING a proxy URL
// carrying somebody else's userinfo. A user who has real corporate proxy
// credentials configured must not have them read as a session tag.
func TokenFromProxyURL(raw string) string {
	rest := raw
	// Case-insensitive: the scheme is case-insensitive per RFC 3986, and a
	// caller who wrote HTTP:// would otherwise have a valid credential read as
	// somebody else's.
	lower := strings.ToLower(rest)
	for _, scheme := range []string{"http://", "https://"} {
		if strings.HasPrefix(lower, scheme) {
			rest = rest[len(scheme):]
			break
		}
	}
	// The authority ends at the first '/', '?' or '#'. Without this the search
	// for '@' below runs into the path, and `http://host/a@b` parses its path
	// as userinfo.
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		rest = rest[:i]
	}
	at := strings.LastIndex(rest, "@")
	if at < 0 {
		return ""
	}
	user, pass, ok := strings.Cut(rest[:at], ":")
	if !ok || user != ProxyUser {
		return ""
	}
	return pass
}
