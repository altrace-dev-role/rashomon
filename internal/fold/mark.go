package fold

import (
	"crypto/sha256"
	"encoding/hex"
)

// LineMark names one line of a file by its length in bytes, its line end not
// counted, and the SHA-256 of those bytes as 64 hex digits. It holds a digest
// of the line and its length, never the line.
type LineMark struct {
	Len    int64  `json:"len"`
	SHA256 string `json:"sha256"`
}

// MarkOf is line's mark: line is the bytes before its line end, as the file
// holds them, a '\r' included.
func MarkOf(line []byte) *LineMark {
	h := sha256.Sum256(line)
	return &LineMark{Len: int64(len(line)), SHA256: hex.EncodeToString(h[:])}
}

// CursorTailValid is CursorValid with the line that ends at c.Offset checked
// too, in the same open and the same read: that line must be m.Len bytes,
// start the file or follow a line end, and hash to m.SHA256. So a file whose
// bytes before the cursor were replaced is refused even when its size and the
// line end at the cursor are what the cursor saw. A zero offset is valid, as
// in CursorValid; a positive one with no mark is not. It costs one open, one
// stat and one read of at most m.Len+2 bytes ending at c.Offset, which
// openCursor has checked is no larger than the file.
//
// Its limit: a file whose line ending at c.Offset is byte for byte the line
// the mark was taken from, at the same offset, passes whatever the bytes
// before that line are.
func CursorTailValid(path string, c Cursor, m *LineMark) (bool, error) {
	if c.Offset == 0 {
		return true, nil
	}
	if m == nil || m.Len < 0 || c.Offset-1-m.Len < 0 {
		return false, nil
	}
	f, ok, err := openCursor(path, c)
	if !ok {
		return false, err
	}
	defer f.Close() //nolint:errcheck // read-only
	start := c.Offset - 1 - m.Len
	from := start
	if from > 0 {
		from--
	}
	b := make([]byte, c.Offset-from)
	if _, err := f.ReadAt(b, from); err != nil {
		return false, err
	}
	if b[len(b)-1] != '\n' || (start > 0 && b[0] != '\n') {
		return false, nil
	}
	h := sha256.Sum256(b[start-from : len(b)-1])
	return hex.EncodeToString(h[:]) == m.SHA256, nil
}
