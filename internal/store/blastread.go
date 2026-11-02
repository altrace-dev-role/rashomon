package store

// FileBlastReadPrefix begins the blast-radius read's markers (card
// ARCH-INLINE1). Red stub: no marker is taken.
const FileBlastReadPrefix = "blast-read-"

// IsBlastReadFile reports a marker's file name. Red stub: none.
func IsBlastReadFile(name string) bool { return false }

// TakeBlastRead takes the session's marker name. Red stub: never.
func (s *Store) TakeBlastRead(sessionID, name string) bool { return false }

// ReturnBlastRead removes the session's marker name. Red stub: nothing.
func (s *Store) ReturnBlastRead(sessionID, name string) {}
