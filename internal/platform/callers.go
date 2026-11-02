package platform

import "time"

// Site is where a Go edit lands for the blast-radius read (card
// ARCH-INLINE1): the sync root, the edited file's package and the log's
// sha256 the walk file vouches for.
type Site struct {
	Root, Package, Log string
}

// Caller is one other package with a PROVEN call into the site's package.
type Caller struct {
	Package, Dir string
	Tests        bool
}

// Locate is the site of an edit of file under root. Red stub: none.
func Locate(root, file string) (Site, bool) { return Site{}, false }

// Callers is the site's callers. Red stub: none.
func Callers(site Site, guard time.Duration) ([]Caller, error) { return nil, nil }
