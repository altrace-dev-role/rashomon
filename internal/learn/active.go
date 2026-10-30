package learn

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Unchecked says why the prompt path gave no lesson from a ledger it found,
// "" when the state checked.
type Unchecked string

const (
	// UncheckedSignature: the state files did not match state.sum on two
	// reads, or there is no key to check them with: a write the ledger did
	// not make.
	UncheckedSignature Unchecked = "signature"
	// UncheckedSize: a state file is over the cap the prompt path reads it
	// under (StateMaxBytes, or ActiveMaxBytes for active.json): a large
	// ledger, never called a tampered one (D171 LD8).
	UncheckedSize Unchecked = "size"
)

// StateMaxBytes caps lessons.json and labels.ndjson where the prompt path
// hashes them (L11), until LEARN-BOUNDS1 bounds the ledger itself.
const StateMaxBytes = 4 << 20

// sumMaxBytes caps the state.sum read: Save writes 64 hex digits and a line
// feed, so a longer file is no manifest and fails the compare.
const sumMaxBytes = 128

// Active is what the prompt path reads of a project's ledger
// (LEARN-DELIVER1): the probation and proven rows of active.json, in its
// order, or why none was read.
type Active struct {
	Lessons   []Lesson
	Unchecked Unchecked
}

// Any reports whether a has something for the prompt: a lesson to give or
// the unchecked flag to raise.
func (a Active) Any() bool { return len(a.Lessons) > 0 || a.Unchecked != "" }

// betweenReads runs in each pass of ReadActive after the state.sum read,
// before the state files: a test lands a Save there. Nil outside tests.
var betweenReads func()

// ReadActive is the ledger's second reader, on the prompt path (recap
// pending): project's active.json under root, checked against state.sum and
// never written. No active.json is the zero Active, for one Lstat.
//
// Otherwise it reads the key, then state.sum and the three state files
// (active.json whole, lessons.json and labels.ndjson hashed as streams), and
// runs that read once more when the sum does not sign the files: Save
// replaces the files and then the sum, so a read between the two sees a
// mismatch no outside write made (D171 LD7). Files the sum signs are one
// state whatever is written after them, so the sum is read once a pass. A
// second mismatch, or no key, is UncheckedSignature; a file over its cap is
// UncheckedSize; either gives no lesson. A matched state gives its
// probation and proven rows, none when active.json says quarantined.
//
// It never calls Save: quarantining a mismatch is the learn verb's, at its
// next Open, so a hook never writes the ledger.
func ReadActive(root, project string) (Active, error) {
	if err := projectElement(project); err != nil {
		return Active{}, err
	}
	dir := filepath.Join(root, DirLearn, project)
	if _, err := os.Lstat(filepath.Join(dir, FileActive)); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Active{}, nil
		}
		return Active{}, err
	}
	key, err := os.ReadFile(filepath.Join(root, DirLearn, FileKey))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Active{}, err
	}
	if len(key) == 0 {
		return Active{Unchecked: UncheckedSignature}, nil
	}
	var active []byte
	unchecked := UncheckedSignature
	buf := make([]byte, 32<<10)
	for try := 0; try < 2 && unchecked == UncheckedSignature; try++ {
		if active, unchecked, err = readSigned(dir, key, buf); err != nil {
			return Active{}, err
		}
	}
	if unchecked != "" {
		return Active{Unchecked: unchecked}, nil
	}
	if active == nil { // a signed state with no active.json: Save writes one, so nothing to give
		return Active{}, nil
	}
	var f activeFile
	if err := decodeActive(active, &f); err != nil {
		return Active{}, fmt.Errorf("learn: %s does not parse: %w", FileActive, err)
	}
	if f.Quarantined {
		return Active{}, nil
	}
	var a Active
	for _, ls := range f.Lessons {
		if ls.Tier == TierProbation || ls.Tier == TierProven {
			a.Lessons = append(a.Lessons, ls)
		}
	}
	return a, nil
}

// readSigned is one pass of ReadActive's read: state.sum, then the state
// files, with buf as the streams' copy buffer. It returns active.json's
// bytes (nil when absent) and "" when the sum signs the files; UncheckedSize
// when a file is over its cap; UncheckedSignature otherwise.
func readSigned(dir string, key, buf []byte) ([]byte, Unchecked, error) {
	sum, err := readSum(dir)
	if err != nil {
		return nil, "", err
	}
	if betweenReads != nil {
		betweenReads()
	}
	digests := make([]string, len(stateFiles))
	var active []byte
	for i, name := range stateFiles {
		var over bool
		if name == FileActive {
			active, digests[i], over, err = readCapped(filepath.Join(dir, name), ActiveMaxBytes)
		} else {
			digests[i], over, err = hashCapped(filepath.Join(dir, name), StateMaxBytes, buf)
		}
		if err != nil {
			return nil, "", err
		}
		if over {
			return nil, UncheckedSize, nil
		}
	}
	if !hmac.Equal(sum, []byte(signDigests(key, digests))) {
		return nil, UncheckedSignature, nil
	}
	return active, "", nil
}

// readSum is state.sum's digest, nil when absent.
func readSum(dir string) ([]byte, error) {
	f, err := os.Open(filepath.Join(dir, FileSum))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only
	b, err := io.ReadAll(io.LimitReader(f, sumMaxBytes+1))
	return bytes.TrimSpace(b), err
}

// readCapped reads path whole when it is at most limit bytes and returns it
// with its digest; an absent file is nil and "-", the manifest's absent
// form; over the cap, over is true.
func readCapped(path string, limit int64) (b []byte, digest string, over bool, err error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "-", false, nil
	}
	if err != nil {
		return nil, "", false, err
	}
	defer f.Close() //nolint:errcheck // read-only
	if b, err = io.ReadAll(io.LimitReader(f, limit+1)); err != nil {
		return nil, "", false, err
	}
	if int64(len(b)) > limit {
		return nil, "", true, nil
	}
	return b, digestHex(b), false, nil
}

// hashCapped is readCapped's digest for a file it does not keep, streamed
// through buf so a 4 MiB file costs no 4 MiB allocation.
func hashCapped(path string, limit int64, buf []byte) (digest string, over bool, err error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "-", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer f.Close() //nolint:errcheck // read-only
	h := sha256.New()
	n, err := io.CopyBuffer(h, io.LimitReader(f, limit+1), buf)
	if err != nil {
		return "", false, err
	}
	if n > limit {
		return "", true, nil
	}
	return hex.EncodeToString(h.Sum(nil)), false, nil
}

// decodeActive is the prompt path's one JSON decode, RES-5's second learn
// allowance: active.json, which ReadActive has read under ActiveMaxBytes
// and checked against state.sum, limited again here so no caller hands the
// decoder more.
func decodeActive(b []byte, f *activeFile) error {
	if len(b) > ActiveMaxBytes {
		return fmt.Errorf("learn: %s is over %d bytes", FileActive, ActiveMaxBytes)
	}
	return json.Unmarshal(b, f)
}
