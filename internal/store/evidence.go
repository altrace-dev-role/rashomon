package store

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The channel-evidence marker (card CHANNEL-EVIDENCE1). When a Stop finds a
// delivered note absent from Claude Code's transcript, on a version the
// event's floor trusts, it writes channel-evidence-<event> under the store
// root: that event's notes are shadowed for the sessions that follow, until a
// probe passes or Claude Code is newer than the version that shadowed. It is
// at the root, not in a run, because the next session is a different run.
//
// The file holds the Claude Code version that shadowed, dotted digits, and
// nothing else; no transcript content, no note text. It is created with
// O_EXCL, so two Stops racing for one event write it once, and it is written
// without a sync: a crash that loses it costs one more session of notes on a
// channel that did not show them.

// ChannelEvidencePrefix begins the marker's name; the event follows it.
const ChannelEvidencePrefix = "channel-evidence-"

// maxEvidenceMarker bounds what a marker read takes in: a version is at most
// a few dozen bytes, and a marker that is longer is not one rashomon wrote.
const maxEvidenceMarker = 64

// IsChannelEvidenceMarker reports whether name is a marker's file name:
// the prefix and an event, one to 32 ASCII letters.
func IsChannelEvidenceMarker(name string) bool {
	event, ok := strings.CutPrefix(name, ChannelEvidencePrefix)
	return ok && evidenceEvent(event)
}

// evidenceEvent reports whether event is a hook event's name as a marker
// uses it, which keeps any other string out of a path.
func evidenceEvent(event string) bool {
	if len(event) == 0 || len(event) > 32 {
		return false
	}
	for i := 0; i < len(event); i++ {
		if c := event[i]; (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}

func (s *Store) evidencePath(event string) string {
	return filepath.Join(s.root, ChannelEvidencePrefix+event)
}

// TakeChannelEvidence writes event's marker holding version and reports
// whether this call created it: false when one exists, which keeps the
// version that shadowed first, and on any error.
func (s *Store) TakeChannelEvidence(event, version string) bool {
	if !evidenceEvent(event) || s.opened() != nil {
		return false
	}
	f, err := os.OpenFile(s.evidencePath(event), os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)
	if err != nil {
		return false
	}
	_, werr := f.WriteString(version + "\n")
	return f.Close() == nil && werr == nil
}

// ChannelEvidence reports whether event's marker exists and the version it
// holds, "" when the content is not a short single line.
func (s *Store) ChannelEvidence(event string) (version string, shadowed bool) {
	if !evidenceEvent(event) || s.opened() != nil {
		return "", false
	}
	f, err := os.Open(s.evidencePath(event))
	if err != nil {
		return "", false
	}
	defer f.Close() //nolint:errcheck // read only
	b, err := io.ReadAll(io.LimitReader(f, maxEvidenceMarker+1))
	if err != nil || len(b) > maxEvidenceMarker {
		return "", true
	}
	return strings.TrimSpace(string(b)), true
}

// ClearChannelEvidence removes event's marker, a probe pass or a newer
// Claude Code version having shown the channel again.
func (s *Store) ClearChannelEvidence(event string) {
	if evidenceEvent(event) {
		_ = os.Remove(s.evidencePath(event))
	}
}
