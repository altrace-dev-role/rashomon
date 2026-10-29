//go:build !unix

package spec

import (
	"io/fs"
	"os"
)

// owner reports uid -1 where there are no uids, so a managed spec is never
// admin-owned there (README 3.1, rule 2); a missing path is still missing.
func owner(path string) (int, fs.FileMode, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return -1, 0, err
	}
	return -1, fi.Mode(), nil
}
