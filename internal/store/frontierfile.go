package store

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// FileFrontier is the session graph's frontier: the fold of a run's
// records.ndjson and steer.ndjson through a mark, kept so a later hook steps
// the lines after the mark instead of folding the run again. It sits in the
// run directory beside its two sources, at the store's one file mode, and
// goes when the run does. A string it holds is one that records.ndjson or
// steer.ndjson of the same run holds, or a SHA-256 digest, with a length in
// bytes, computed from bytes of those two files alone and named as such in
// the Holds text of its rule in the disclosure inventory. The store owns its
// name, its path and its write; the bytes are the session package's
// encoding, and the store never decodes them.
const FileFrontier = "frontier.json"

// MaxFrontierBytes bounds a frontier, both ways: 1 MiB, a literal of its own
// and no multiple of another cap, since nothing ties a frontier's size to a
// record line's. SaveFrontier refuses data past it and LoadFrontier refuses a
// file past it, so the store never writes a frontier its own reader would not
// take, and a file past it that came from elsewhere reads as unreadable: the
// caller folds from the record, as it does for an absent one. Past the cap
// costs a fold, never a wrong frontier.
const MaxFrontierBytes = 1 << 20

// errFrontierOverCap is the refusal of a frontier past MaxFrontierBytes, by
// SaveFrontier before it writes and by LoadFrontier after its bounded read.
var errFrontierOverCap = errors.New("store: frontier over its cap")

// SaveFrontier replaces the session's frontier with data through ReplaceFile:
// a temporary .frontier.json.tmp-* beside it at 0600, fully synced, then
// renamed over it, so a reader finds the old frontier or the new one and never
// part of either. The path is RunDir's, as NoteNoSpec's is, so an id unsafe
// as a path element is mapped to its digest, never refused and never joined
// raw. The run directory is not made: a session without one has no records to
// fold, and a save there returns the error and writes nothing. Data past
// MaxFrontierBytes is refused with errFrontierOverCap before anything is
// written; the frontier already there stays, and since its mark is older the
// caller catches up from it, which folds to the same frontier as the whole
// record (TestFold_IncrementalEqualsFull).
func (s *Store) SaveFrontier(sessionID string, data []byte) error {
	if err := s.opened(); err != nil {
		return err
	}
	if len(data) > MaxFrontierBytes {
		return errFrontierOverCap
	}
	return ReplaceFile(filepath.Join(s.RunDir(sessionID), FileFrontier), data)
}

// LoadFrontier returns the session's frontier as SaveFrontier wrote it. Its
// three outcomes: the bytes, true and no error; absent, nil, false and no
// error, when the file or the run directory is missing; or unreadable, nil,
// true and the error, when the path is not a regular file (regfile.Open), the
// file is past MaxFrontierBytes, or the read fails. Absent and unreadable both
// leave the caller to fold from the record. A Store that was never opened is
// refused first, with nil, false and errUnopenedStore, as Activity refuses it:
// its run directory would be relative to the working directory, and a read
// there would load some other tree's frontier as this session's.
func (s *Store) LoadFrontier(sessionID string) ([]byte, bool, error) {
	if err := s.opened(); err != nil {
		return nil, false, err
	}
	f, err := regfile.Open(filepath.Join(s.RunDir(sessionID), FileFrontier))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, true, err
	}
	defer f.Close() //nolint:errcheck // read-only
	b, err := io.ReadAll(io.LimitReader(f, MaxFrontierBytes+1))
	if err != nil {
		return nil, true, err
	}
	if len(b) > MaxFrontierBytes {
		return nil, true, errFrontierOverCap
	}
	return b, true, nil
}

// removeFrontier removes a run's saved frontier, and any temporary file a
// save left by dying between its write and its rename, for a forget about to
// drop lines of the run. Both forgets call it under the records lock
// openRunLines holds, before the gap and before any line goes. A frontier is
// folded from the run's records, so one left behind would keep state the
// forget removes; a run with no frontier costs only a fold from what forget
// kept. An absent file is no error. Any other error fails the run before
// anything else changes, so forget never reports a run forgotten with its
// frontier left. Forget does not rewrite the steer log, so what a frontier
// drew from it returns at the next fold; what it drew from the records does
// not.
func removeFrontier(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	tmp := "." + FileFrontier + ".tmp-"
	for _, e := range entries {
		if n := e.Name(); n == FileFrontier || strings.HasPrefix(n, tmp) {
			if err := os.Remove(filepath.Join(dir, n)); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}
