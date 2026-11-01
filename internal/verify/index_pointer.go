package verify

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The project's last two checkpoints (card REPORT1b) are a file of their own
// beside the project index, not keys in it (INDEX-POINTER-FILE1): alpha.1
// reads the index with DisallowUnknownFields, so a pointer there would make
// it rebuild the index and lose the learned gates. The file is a hint, which
// is why it is cheap to keep apart: a reader that finds none, or one that does
// not read, falls back to the verify line, and receipt.Pointed checks the
// checkpoint it names before anything is printed.
const (
	// CheckpointsDir is the directory of checkpoint files within DirName, one
	// per project, named as the index files are (baseline.FileName), so
	// IsIndexFile names its files and their replacements too.
	CheckpointsDir = "checkpoints"
	// MaxCheckpointsBytes caps the file on read and on write: two session ids
	// and four integers fit in it many times over.
	MaxCheckpointsBytes = 4 << 10
)

// Checkpoints is a project's last two checkpoints by session, which the Stop
// that cuts one notes: Last the newest, Prior the newest of a different
// session. SessionStart reads them to name the previous session in one small
// read instead of walking the store.
type Checkpoints struct {
	Last  *LastCheckpoint `json:"last,omitempty"`
	Prior *LastCheckpoint `json:"prior,omitempty"`
}

// LastCheckpoint names a checkpoint record: its session, its point seq and
// when it was recorded.
type LastCheckpoint struct {
	SessionID string `json:"session_id"`
	PointSeq  int64  `json:"point_seq"`
	AtMS      int64  `json:"at_unix_ms"`
}

// CheckpointsPath is the checkpoint file of projectKey under the store root:
// <root>/verify/checkpoints/<name>, name as IndexPath's.
func CheckpointsPath(root, projectKey string) string {
	return filepath.Join(root, DirName, CheckpointsDir, baseline.FileName(projectKey))
}

// ReadCheckpoints reads the checkpoint file at path. A missing file is (nil,
// nil). A file over MaxCheckpointsBytes, one that cannot be read, and one that
// is not one JSON value are (nil, error), which a caller reads as no pointer.
// A key it does not name is read and dropped, so a key a later build adds
// costs this one nothing.
func ReadCheckpoints(path string) (*Checkpoints, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only
	// One byte past the cap, so that a file over it is seen to be.
	lr := &io.LimitedReader{R: f, N: MaxCheckpointsBytes + 1}
	dec := json.NewDecoder(lr)
	var c Checkpoints
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("verify: checkpoints %s: %w", path, err)
	}
	// Token reads to the end, so lr.N is final after it.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("verify: checkpoints %s: data after its one value", path)
	}
	if lr.N == 0 {
		return nil, fmt.Errorf("verify: checkpoints %s is over its %d-byte cap", path, MaxCheckpointsBytes)
	}
	return &c, nil
}

// WriteCheckpoints writes c to path as one JSON line: under a temporary name
// beside it, synced and renamed over it at 0600 (store.ReplaceFile), with its
// directory created at 0700 where it is absent. An encoding over
// MaxCheckpointsBytes is an error, and nothing is written.
func WriteCheckpoints(path string, c *Checkpoints) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if b = append(b, '\n'); len(b) > MaxCheckpointsBytes {
		return fmt.Errorf("verify: checkpoints %s would be over its %d-byte cap", path, MaxCheckpointsBytes)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return store.ReplaceFile(path, b)
}

// NoteCheckpoint records that session cut a checkpoint at point seq at atMS:
// it becomes Last, and the Last of another session becomes Prior (a clear
// that continues one session leaves Prior alone, so Prior stays the
// previous session's). The Stop calls it only after the checkpoint was
// appended, so the pointer never names a record that was dropped.
//
// Two sessions that Stop at once each read the file, note their checkpoint
// and write it back with no lock between, so the last writer wins and the
// other's Last (or Prior) is lost. That is safe: the pointer is only a hint,
// and receipt.Pointed checks the checkpoint it names (project and point seq)
// before anything is printed, so a lost pointer leaves the verify line and a
// stale one is never shown as current.
func (c *Checkpoints) NoteCheckpoint(session string, seq, atMS int64) {
	if c.Last != nil && c.Last.SessionID != session {
		prior := *c.Last
		c.Prior = &prior
	}
	c.Last = &LastCheckpoint{SessionID: session, PointSeq: seq, AtMS: atMS}
}

// PreviousOf is the checkpoint the file names for the session before
// current: Last, unless that is current's own (a clear), then Prior. Nil
// when it names none, or c is nil.
func (c *Checkpoints) PreviousOf(current string) *LastCheckpoint {
	if c == nil {
		return nil
	}
	if c.Last != nil && c.Last.SessionID != current {
		return c.Last
	}
	if c.Last != nil && c.Prior != nil && c.Prior.SessionID != current {
		return c.Prior
	}
	return nil
}
