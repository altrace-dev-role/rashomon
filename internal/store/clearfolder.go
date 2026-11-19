package store

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"time"
)

// DirClear is the /clear rendezvous's directory under the store root. Each
// folder in it, named by the SHA-256 digest of one working directory's path,
// holds that directory's arms and links (MeetClear); purge owns a folder
// that holds only meetClear's names (IsClearDirName, IsClearName).
const DirClear = "clear"

// MeetClear is meetClear in root's rendezvous folder for the working
// directory cwd, DirClear/<hex of the SHA-256 of cwd>: no name under the
// store holds the cwd, and a /clear in one cwd neither pairs with nor sweeps
// another's. The digest is of cwd as the hook gives it, so the two hooks of
// one /clear meet only when they carry an equal cwd. A cwd that is empty or
// not absolute pairs nothing and writes nothing.
func MeetClear(root, cwd string, side ClearSide, self string, now time.Time) (ClearLink, bool, error) {
	if !filepath.IsAbs(cwd) {
		return ClearLink{}, false, nil
	}
	sum := sha256.Sum256([]byte(cwd))
	return meetClear(filepath.Join(root, DirClear, hex.EncodeToString(sum[:])), side, self, now)
}

// IsClearDirName reports whether name is a rendezvous folder's as MeetClear
// names it: a SHA-256 digest in 64 lowercase hex digits.
func IsClearDirName(name string) bool {
	if len(name) != hex.EncodedLen(sha256.Size) {
		return false
	}
	for i := 0; i < len(name); i++ {
		if c := name[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
