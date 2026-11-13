package store

import (
	"encoding/hex"
	"strings"
)

// Segment is the one path element an id from outside this program may
// become: the id itself when it is a plain segment, else "x-" and a digest
// of it. It is the run directory's rule, for a caller that names a file
// under the store by an id it did not make.
func Segment(id string) string { return segment(id) }

// IsSegment is whether name is a value Segment can return: a plain
// segment, or "x-" and 32 lowercase hex (digestID's form). It is the
// other side of Segment, for a reader that must tell a name this program
// wrote from one a person put there.
func IsSegment(name string) bool {
	if isPlainSegment(name) {
		return true
	}
	rest, ok := strings.CutPrefix(name, "x-")
	if !ok || len(rest) != 2*sha256Prefix || rest != strings.ToLower(rest) {
		return false
	}
	_, err := hex.DecodeString(rest)
	return err == nil
}

// sha256Prefix is the digest bytes digestID keeps.
const sha256Prefix = 16
