package intent

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// pathDomain is the domain of a named path's keyed digest: a name no tool
// can have, as shape.CWDDigest's is (it starts with a NUL).
const pathDomain = "\x00preflight-path"

// PathDigest is the keyed digest of a named project path: HMAC-SHA256 under
// the store's per-install key, as shape.CWDDigest keys a cwd, never a plain
// hash. Project paths are low in entropy, and a plain hash is reversed by
// hashing the repository's own file list; under the key a second install
// cannot read this one's set, and paths still dedupe within one install.
// It is what a steer line holds in place of a path at record scope v4.
func PathDigest(key []byte, path string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(pathDomain))
	m.Write([]byte{0})
	m.Write([]byte(path))
	return hex.EncodeToString(m.Sum(nil))
}

// saidDomain is the domain of a quote's keyed digest, apart from pathDomain,
// so one string never digests the same as a path and as a quote.
const saidDomain = "\x00continuity-said"

// SaidDigest is the keyed digest of the person's words a continuity briefing
// quoted (COMPACT-SLOT1): PathDigest's HMAC-SHA256 under the store's
// per-install key, in a domain of its own. Typed words are low in entropy,
// and a plain hash of a short phrase is reversed by guessing the phrase;
// under the key it is not, and the steer line holds this in place of the
// words at every record scope (D160).
func SaidDigest(key []byte, words string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(saidDomain))
	h.Write([]byte{0})
	h.Write([]byte(words))
	return hex.EncodeToString(h.Sum(nil))
}
