package store

import (
	"os"
	"path/filepath"
	"strings"
)

// FileSessionContent is the marker under the store root that says session
// content may be written to the graph file (ruling 15, W-M2.11): the consent
// on file covers scope_version 6 at record scope v5. It holds "1".
//
// Nothing in this tree writes it yet. The step that lands consent for
// scope_version 6 (W-M2.11, step 1d) will write it, after the consent line,
// and remove it before one, so it is a view of consent.json that lags it and
// never leads, and a crash leaves it absent, which reads as no consent.
// Until that step ships the marker is absent on every install, and the
// overlay writers that read it (internal/hook) write no group. The hook
// reads this one small file, never consent.json.
const FileSessionContent = "session_content"

// SessionContentMarker reports whether the marker holds "1". Absent,
// unreadable or any other content is false: nothing is written.
func SessionContentMarker(root string) bool {
	b, err := os.ReadFile(filepath.Join(root, FileSessionContent))
	return err == nil && strings.TrimSpace(string(b)) == "1"
}
