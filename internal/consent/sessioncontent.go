package consent

import (
	"errors"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// ErrSessionContentMarker wraps Append's error when the consent line is
// durable and the session-content marker after it could not be written: the
// marker is then absent, which the hook reads as no consent to keep the
// session's content, until a later consent write or the next session start's
// sync writes it.
var ErrSessionContentMarker = errors.New("the session_content marker was not written")

// writeSessionContentMarker is store.WriteSessionContentMarker, which
// TestSessionContent_MarkerFailureKeepsTheLine replaces to fail it.
var writeSessionContentMarker = store.WriteSessionContentMarker

// SessionContent is the answer the session-content marker
// (store.FileSessionContent) holds (ruling 15, W-M2.11 step 1d): whether the
// consent on file lets the hook keep the session's content under the store's
// graph directory. It reads the two facts ScopeCovered and RecordScope read,
// from one read of the file: the highest scope_version on file is at least
// disclose.SessionContentScope, the scope whose screen names that content,
// and the latest record that names a record scope names v5. A later v4
// record turns it off; a unit record, which names neither, moves neither.
// Compaction keeps the records both facts come from (keepForCompaction), so
// it keeps this answer. An unreadable file is false, with the error.
func SessionContent(root string) (bool, error) {
	recs, err := Read(root)
	if err != nil {
		return false, err
	}
	best, ok := maxBy(recs, func(r Record) int { return r.ScopeVersion })
	if !ok || best < disclose.SessionContentScope {
		return false, nil
	}
	for i := len(recs) - 1; i >= 0; i-- {
		if s := recs[i].RecordScope; s != "" {
			return s == RecordScopeV5, nil
		}
	}
	return false, nil
}

// syncSessionContentMarker writes root's session-content marker where
// SessionContent says the consent on file covers the session's content, and
// removes it where it does not. Append has removed it before, so a file
// SessionContent cannot read leaves it absent.
func syncSessionContentMarker(root string) error {
	on, err := SessionContent(root)
	if err != nil {
		return err
	}
	if !on {
		return store.RemoveSessionContentMarker(root)
	}
	return writeSessionContentMarker(root)
}
