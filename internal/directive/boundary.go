package directive

import "errors"

// Boundary is a compaction's mark in a transcript. Red: not read.
type Boundary struct {
	UUID string
	Line int
}

// ErrTooLarge is LastBoundary's refusal of a transcript over its read cap.
var ErrTooLarge = errors.New("directive: transcript over the read cap")

const maxBoundaryLine = 64 << 10

// LastBoundary is the transcript's last compact_boundary record. Red: none.
func LastBoundary(path string, capBytes int64) (Boundary, bool, error) {
	return Boundary{}, false, nil
}
