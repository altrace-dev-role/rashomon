package store

import (
	"os"
	"path/filepath"
)

// FileLab is the marker under the store root that makes the store a
// measurement lab: `rashomon measure --prepare` writes it, and a hook honours
// the operator's forced arm (RASHOMON_STEER_ARM) only on a store that holds
// it (card MEASURE1). A forced arm skips the field holdout's draw, so on a
// normal store it would take the holdout's place. A repository's committed
// settings can set the variable for every session opened in it, and hooks
// inherit the session's environment; a repository cannot write under the
// store root, so the variable alone forces no arm.
//
// Exported so the disclosure inventory (internal/disclose) and purge name
// the same constant rather than a second copy of the literal.
const FileLab = "lab"

// labText is the marker's content, for whoever finds the file. Only its
// presence is read.
const labText = "rashomon measure --prepare: this store is a measurement lab, so RASHOMON_STEER_ARM is honoured here\n"

// Lab reports whether root holds the lab marker as a regular file. It
// stats one path directly, never through Open, for SteerDisclosureMarker's
// reason: a root must not acquire an install identity by being asked what
// it may do.
func Lab(root string) bool {
	fi, err := os.Lstat(filepath.Join(root, FileLab))
	return err == nil && fi.Mode().IsRegular()
}

// WriteLab writes root's lab marker, creating root as the store creates it
// when it does not exist yet, so `--prepare` can run before watch installs
// into the same root. Written as ReplaceFile writes, so a reader finds the
// marker whole or not at all.
func WriteLab(root string) error {
	if err := os.MkdirAll(root, dirMode); err != nil {
		return err
	}
	return ReplaceFile(filepath.Join(root, FileLab), []byte(labText))
}
