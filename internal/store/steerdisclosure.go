package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FileSteerDisclosure is the marker under the store root that names the
// highest steer disclosure version consent.json holds: one non-negative
// decimal integer and a newline. Like FileRecordScope it is a view of
// consent.json, which stays the record of truth: every consent write
// removes it, appends its line, and writes it again from consent.json's own
// answer (consent.SteerDisclosure), and every session start puts it right,
// so it can lag the log and never lead it.
//
// It holds the fact, not the answer. Whether consent is current is the
// reading build's to decide, against its own steer.DisclosureVersion, so a
// build whose disclosure has grown since the user consented reads the
// marker as not current until the next consent. A yes written by one build
// would be read by a later build for a disclosure the user never saw.
//
// Exported so the disclosure inventory (internal/disclose) and purge name
// the same constant rather than a second copy of the literal.
const FileSteerDisclosure = "steer_disclosure"

// ErrSteerDisclosureWord is SteerDisclosureMarker's error for a marker whose
// content is not a non-negative decimal integer.
var ErrSteerDisclosureWord = errors.New("store: the steer_disclosure marker holds no non-negative integer")

// SteerDisclosureMarker is the steer disclosure version root's marker names:
// its integer when the content, white space aside, is ASCII digits that fit
// an int, and 0 for everything else. A non-nil error says why it is 0:
// errors.Is(err, fs.ErrNotExist) for no marker, ErrSteerDisclosureWord for
// other content, the read's own error otherwise. A caller that only applies
// the value may drop the error; 0 is already the value that withholds.
//
// It reads one small file directly, never through Open, for
// RecordScopeMarker's reason: a root must not acquire an install identity by
// being asked what it may say.
func SteerDisclosureMarker(root string) (int, error) {
	b, err := os.ReadFile(filepath.Join(root, FileSteerDisclosure))
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(b))
	if s == "" || strings.Trim(s, "0123456789") != "" {
		return 0, ErrSteerDisclosureWord
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, ErrSteerDisclosureWord
	}
	return n, nil
}

// WriteSteerDisclosureMarker sets root's marker to n, which must not be
// negative: written under a temporary name beside it, synced and renamed
// over it, so a reader finds the old marker or the new one and never a part
// of either. Like the reader, it never goes through Open.
func WriteSteerDisclosureMarker(root string, n int) error {
	if n < 0 {
		return fmt.Errorf("store: refusing to write steer disclosure %d, which is negative", n)
	}
	return ReplaceFile(filepath.Join(root, FileSteerDisclosure), []byte(strconv.Itoa(n)+"\n"))
}

// RemoveSteerDisclosureMarker removes root's marker, so the hook reads 0 until
// it is written again. No marker is not an error.
func RemoveSteerDisclosureMarker(root string) error {
	if err := os.Remove(filepath.Join(root, FileSteerDisclosure)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
