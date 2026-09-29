package hook

import (
	"encoding/json"
	"io"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// RecordPaused writes the coverage record for one invocation that found
// recording paused. It is the paused counterpart of Handler.Close, Post.Close
// and RunProbe's own coverage write, and it writes nothing else: no
// declaration, no execution, no terminal, because the whole point of pausing
// is that none of those happen here on purpose. The reason is fixed --
// store.ReasonRecordingPaused -- and BuildCoverage's own rule that an
// invocation's own reason outranks anything resolved from configuration is
// what keeps it from being overwritten by whatever hook_entry or probe
// happen to resolve to.
//
// The payload is still read, for the session id and cwd it carries: a paused
// call inside a real session must render as a gap IN that session's own
// report, not as a fact about some unrelated one. A payload that cannot be
// parsed, or that names no session, falls back to store.UnattributedSession,
// exactly as the declaration and probe paths do.
func RecordPaused(in io.Reader, phase string, st *store.Store, now func() time.Time) {
	sessionID := store.UnattributedSession
	cwd := ""
	if raw, err := readPayload(in); err == nil {
		var p sessionPayload
		if json.Unmarshal(raw, &p) == nil && p.SessionID != "" {
			sessionID = p.SessionID
			cwd = p.CWD
		}
	}
	_ = st.AppendCoverage(BuildCoverage(st, sessionID, phase, store.ReasonRecordingPaused, cwd, now()))
}
