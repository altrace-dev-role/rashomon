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
