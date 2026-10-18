package fold

import (
	"os"
)

// Validate reports whether f is still a trustworthy resume point for one
// run: step 2 of the Stop algorithm (technical spec 8.2.3; Archive 85.3.5).
// recordsPath, spillPath, coveragePath and gapsPath are stat'd fresh, never
// trusted from an earlier read, because the whole point of this check is
// to catch what changed between the Stop that wrote f and the one asking
// now.
//
// It holds only when every one of these holds:
//   - f.Dev and f.Inode equal recordsPath's identity right now: a forget or
//     an eviction rewrite removes the old inode and creates a new one at
//     the same path, and every offset taken from the old file is void the
//     moment that happens (fold.go's doc on Dev/Inode);
//   - f.Records.Offset, f.Spill.Offset, f.Coverage.Offset and f.Gaps.Offset
//     are each no greater than their file's current size -- a file that
//     became SHORTER under a saved cursor (Cursor.SizeSeen's doc) is caught
//     here before that offset is trusted, rather than read from the middle
//     of whatever is now at that position;
//   - the byte immediately before each positive offset is '\n', so a fold
//     can only resume onto a line boundary, never into the middle of one
//     (an offset of 0 always holds: there is no byte before it to check,
//     and nothing has been folded yet);
//   - f.V is a schema this package understands and f.Writer is recorded.
//
// A fold that fails any one of these is not wrong, only stale or foreign:
// its caller runs the full streaming fold from offset 0 instead (8.2.3 step
// 6's fallback). Validate never guesses which half of a cursor is still
// good -- one check failing fails the whole file -- and it never writes
// anything.
func Validate(f *File, recordsPath, spillPath, coveragePath, gapsPath string) (bool, error) {
	if f.V != Version || f.Writer == "" {
		return false, nil
	}

	dev, inode, exists, err := identity(recordsPath)
	if err != nil {
		return false, err
	}
	if !exists || dev != f.Dev || inode != f.Inode {
		return false, nil
	}

	for _, c := range []struct {
		path   string
		cursor Cursor
	}{
		{recordsPath, f.Records},
		{spillPath, f.Spill},
		{coveragePath, f.Coverage},
		{gapsPath, f.Gaps},
	} {
		ok, err := cursorValid(c.path, c.cursor)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// cursorValid checks one cursor against the file it was taken from, stat'd
// and read fresh: its offset must be no greater than the file's current
// size, and, when the offset is positive, the byte immediately before it
// must be a line terminator. A zero offset is always valid -- it names the
// start of the file, never a position inside a line -- even when the file
// does not exist yet (a run with no spill or no gaps at all).
func cursorValid(path string, c Cursor) (bool, error) {
	if c.Offset == 0 {
		return true, nil
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close() //nolint:errcheck // read-only

	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	if c.Offset < 0 || c.Offset > info.Size() {
		return false, nil
	}

	var b [1]byte
	n, err := f.ReadAt(b[:], c.Offset-1)
	if err != nil || n != 1 {
		return false, err
	}
	return b[0] == '\n', nil
}
