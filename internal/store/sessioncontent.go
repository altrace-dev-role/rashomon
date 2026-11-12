package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// FileSessionContent is the marker under the store root that says session
// content may be written to the graph file (ruling 15, W-M2.11): the consent
// on file covers scope_version 6 at record scope v5. It holds "1".
//
// It is a view of consent.json, the third beside FileRecordScope and
// FileSteerDisclosure (W-M2.11 step 1d): consent.Append removes it before it
// touches consent.json and writes it, from consent.SessionContent's answer,
// once the line is durable, and the SessionStart probe puts it right once a
// session. It lags consent.json and never leads: a crash in between leaves
// it absent, which reads as no consent, and its readers (the hook's overlay
// group, the prompt's unit briefs and conflict base) write nothing. The hook
// reads this one small file, never consent.json.
const FileSessionContent = "session_content"

// SessionContentMarker reports whether the marker holds "1". Absent,
// unreadable or any other content is false: nothing is written.
func SessionContentMarker(root string) bool {
	b, err := os.ReadFile(filepath.Join(root, FileSessionContent))
	return err == nil && strings.TrimSpace(string(b)) == "1"
}

// WriteSessionContentMarker sets root's marker to "1": written under a
// temporary name beside it, synced and renamed over it, as the record-scope
// marker is (WriteRecordScopeMarker). consent.Append is its writer.
func WriteSessionContentMarker(root string) error {
	return ReplaceFile(filepath.Join(root, FileSessionContent), []byte("1\n"))
}

// RemoveSessionContentMarker removes root's marker, so the hook keeps none of
// the session's content until it is written again. No marker is not an
// error.
func RemoveSessionContentMarker(root string) error {
	if err := os.Remove(filepath.Join(root, FileSessionContent)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
