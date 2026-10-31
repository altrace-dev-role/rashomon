package steer

import (
	"bytes"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Channel evidence (card CHANNEL-EVIDENCE1, D226). The floor table says a
// version is trusted to forward a note to the model; this is the check that
// the trust held in a session. At Stop, each note the session delivered is
// looked for in Claude Code's own transcript, as the text of a
// hook_additional_context attachment, which is where a delivered note
// appears (test/live/p-tru-1.sh). The note is rashomon's own text, matched in
// memory and never stored; nothing is added to any note to find it.
//
// The outcome per note is a label, never a score: seen, absent, or
// unreadable. It goes on the steer log as a line of its own, keyed by the
// note's id (EvidenceLine), because the log is append-only and a note's line
// is long since written when the Stop reads the transcript. One absent
// reading on a version the event's floor trusts shadows that event for the
// sessions that follow (store.TakeChannelEvidence); an unreadable reading
// changes nothing.

// Evidence is one delivered note's outcome.
type Evidence string

const (
	// EvidenceSeen: the note's text is in a hook_additional_context
	// attachment of the transcript.
	EvidenceSeen Evidence = "seen"
	// EvidenceAbsent: the whole transcript was read, as far as the note's
	// time reaches, and no attachment carries the text.
	EvidenceAbsent Evidence = "absent"
	// EvidenceUnreadable: the transcript is missing, could not be read to the
	// note's time within the bound, or holds no record; it says nothing of
	// the channel.
	EvidenceUnreadable Evidence = "unreadable"
)

// EvidenceLineType is the "type" of an EvidenceLine in steer.ndjson.
const EvidenceLineType = "steer_evidence"

// evidenceSettle is how old a note must be before a Stop judges it: Claude
// Code writes its transcript after the turn it records, so a note delivered
// in the last moments may not be there yet, and reading it as absent would
// shadow a channel that works. A note younger than this waits for the next
// Stop.
const evidenceSettle = 2 * time.Second

// EvidenceLine is the evidence of one delivered note, appended to the run's
// steer.ndjson at the Stop that read the transcript. Its shape is the steer
// line's own bookkeeping (type, v, session_id, prompt_id, at_unix_ms) and
// the note's id, so a reader of the log that counts a prompt's lines finds
// it with that prompt's; it has no state, pattern or delivery, so no reader
// of notes finds a note in it. LineVersion stays 1: the log gains a type of
// line, and no key of a steer line changes.
type EvidenceLine struct {
	Type      string   `json:"type"`
	V         int      `json:"v"`
	SteerID   string   `json:"steer_id"`
	SessionID string   `json:"session_id"`
	PromptID  *string  `json:"prompt_id"`
	AtUnixMS  int64    `json:"at_unix_ms"`
	Event     string   `json:"event"`
	Evidence  Evidence `json:"evidence"`
}

// CapText is store.SteerLine's one method: an evidence line has no text.
func (EvidenceLine) CapText() bool { return false }

// evidenceNote is a delivered note a Stop has not judged yet.
type evidenceNote struct {
	id, event, needle, version string
	prompt                     *string
	at                         time.Time
	trusted                    bool
}

var (
	stateDelivered = []byte(`"state":"delivered"`)
	evidenceType   = []byte(`"type":"` + EvidenceLineType + `"`)
)

// pendingNotes are the session's main-agent notes that were delivered with a
// model text, have no evidence line yet, and are older than evidenceSettle at
// now, in log order. A subagent's note is in the subagent's own transcript,
// which this check does not read, so it is not judged.
func pendingNotes(lines [][]byte, sessionID string, now time.Time) []evidenceNote {
	judged := map[string]bool{}
	var notes []evidenceNote
	var out [nkCount]jsonscan.Span
	for _, b := range lines {
		isEvidence := bytes.Contains(b, evidenceType)
		if !isEvidence && !bytes.Contains(b, stateDelivered) {
			continue
		}
		if jsonscan.Top(b, noteKeys(), out[:]) != nil || textOf(b, out[nkSession]) != sessionID {
			continue
		}
		if isEvidence {
			judged[textOf(b, out[nkSteerID])] = true
			continue
		}
		needle := firstLine(textOf(b, out[nkModel]))
		if textOf(b, out[nkType]) != LineType || textOf(b, out[nkState]) != string(LineStateDelivered) || needle == "" ||
			textOf(b, out[nkAgent]) != "" {
			continue
		}
		at := time.UnixMilli(int64Of(b, out[nkAt]))
		if now.Sub(at) < evidenceSettle {
			continue
		}
		n := evidenceNote{id: textOf(b, out[nkID]), event: textOf(b, out[nkEvent]), needle: needle,
			version: textOf(b, out[nkVersion]), at: at}
		if out[nkPrompt].Kind == jsonscan.String {
			p := textOf(b, out[nkPrompt])
			n.prompt = &p
		}
		n.trusted = trustedBasis(n.version, textOf(b, out[nkFloor]), stringsOf(b, out[nkExcluded]))
		notes = append(notes, n)
	}
	return slices.DeleteFunc(notes, func(n evidenceNote) bool { return judged[n.id] })
}

// firstLine is text's first line that is not blank, trimmed: the part of a
// note the check keys on.
func firstLine(text string) string {
	for line := range strings.SplitSeq(text, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// trustedBasis reports whether a line's own basis (Line.ClientVersion,
// VerifiedFloor, Excluded) trusted the channel: an event with a floor, and a
// version at or above it that no exclusion names. An exempt event, which has
// no floor, trusts nothing by this test, so an absent note there shadows
// nothing.
func trustedBasis(version, floor string, excluded []string) bool {
	if floor == "" || !digitRuns(version, '.') || !digitRuns(floor, '.') || compareVersions(version, floor) < 0 {
		return false
	}
	return !channelBasis{floor: floor, excluded: excluded}.excludes(version)
}

// CheckEvidence judges the session's undecided delivered notes against the
// transcript at path and records the outcomes: one EvidenceLine each on the
// run's steer log, then, for each event with a note absent on a trusted
// version, the store's marker holding that version. It returns the lines it
// wrote. It reads nothing, and writes nothing, when no note is waiting, so a
// Stop with nothing to judge never opens the transcript. The caller has made
// the Stop's own record durable first.
func CheckEvidence(st *store.Store, sessionID, path string, now time.Time, lim ScanLimits) []EvidenceLine {
	lines, _, err := st.ReadSteer(sessionID)
	if err != nil {
		return nil
	}
	notes := pendingNotes(lines, sessionID, now)
	if len(notes) == 0 {
		return nil
	}
	needles := make([]string, len(notes))
	oldest := notes[0].at
	for i, n := range notes {
		needles[i] = n.needle
		if n.at.Before(oldest) {
			oldest = n.at
		}
	}
	found, complete := ScanTranscript(path, needles, oldest, lim)
	out := make([]EvidenceLine, len(notes))
	for i, n := range notes {
		e := EvidenceUnreadable
		switch {
		case found[i]:
			e = EvidenceSeen
		case complete:
			e = EvidenceAbsent
		}
		out[i] = EvidenceLine{Type: EvidenceLineType, V: LineVersion, SteerID: n.id, SessionID: sessionID, PromptID: n.prompt,
			AtUnixMS: now.UnixMilli(), Event: n.event, Evidence: e}
	}
	err = st.AppendSteer(sessionID, func(_ io.ReaderAt) ([]store.SteerLine, error) {
		batch := make([]store.SteerLine, len(out))
		for i := range out {
			batch[i] = out[i]
		}
		return batch, nil
	})
	if err != nil {
		return nil
	}
	_ = st.Flush()
	for i, n := range notes {
		if out[i].Evidence == EvidenceAbsent && n.trusted {
			st.TakeChannelEvidence(n.event, n.version)
		}
	}
	return out
}

// evidenceShadow reports whether event's marker shadows the call, for a call
// that is otherwise verified: the event has a floor, a marker exists, and the
// client is not newer than the version that shadowed. A client newer than it
// removes the marker and is not shadowed, so a release the Stop never judged
// is trusted again on the floor's word. A marker with no readable version
// stays until a probe pass removes it.
func evidenceShadow(ctx *Context, c Client, table map[string]channelBasis) bool {
	event := string(ctx.Event)
	if table[event].floor == "" {
		return false
	}
	marked, ok := ctx.Store.ChannelEvidence(event)
	if !ok {
		return false
	}
	if digitRuns(marked, '.') && digitRuns(c.Version, '.') && compareVersions(c.Version, marked) > 0 {
		ctx.Store.ClearChannelEvidence(event)
		return false
	}
	return true
}
