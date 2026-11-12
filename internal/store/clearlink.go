package store

import "time"

// ClearSide is which hook of a /clear is calling: the old session's
// SessionEnd or the new session's SessionStart.
type ClearSide uint8

const (
	ClearEnd ClearSide = iota + 1
	ClearStart
)

// ClearLink is a /clear's pair: New continues Old.
type ClearLink struct {
	Old, New string
	Wrote    bool
}

const (
	ClearWindow = 10 * time.Second
	ClearKeep   = time.Hour
)

func meetClear(dir string, side ClearSide, self string, now time.Time) (ClearLink, bool, error) {
	return ClearLink{}, false, nil
}
