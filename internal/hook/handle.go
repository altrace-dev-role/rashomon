package hook

import (
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/altrace-dev-role/altrace-attest/internal/fault"
	"github.com/altrace-dev-role/altrace-attest/internal/shape"
	"github.com/altrace-dev-role/altrace-attest/internal/store"
)

// MaxPayloadBytes bounds how much of stdin is read.
//
// This is a panic-barrier concern, not a tidiness one. Go's response to
// exhausting memory is a runtime fatal error, which recover cannot catch and
// which exits 2 -- and exit 2 from a PreToolUse hook blocks the user's tool
// call. An unbounded ReadAll over an attacker-influenced tool_input is
// therefore a way to turn this recorder into an enforcer, and the limit is what
// closes it.
const MaxPayloadBytes = 8 << 20

// UnattributedSession buckets records for a payload too malformed to name its
// own session. Losing the run is bad; silently attributing it to a session that
// did not produce it would be worse.
const UnattributedSession = "unattributed"

var errPayloadTooLarge = errors.New("hook: payload exceeds the read limit")

// Handler captures one tool call and then closes it out.
//
// Capture and Close are separate because Close has to run on paths where
// Capture did not return normally. Once a declaration is on disk it must be
// followed by a terminal record on every exit this process controls, and a
// single function cannot promise that about its own panic.
type Handler struct {
	st  *store.Store
	now func() time.Time

	sessionID string
	toolUseID string
	opened    bool
}

// New builds a Handler. now is injectable so a test can pin the clock and
// compare two records byte for byte.
func New(st *store.Store, now func() time.Time) *Handler {
	return &Handler{st: st, now: now, sessionID: UnattributedSession}
}

// Capture reads one payload and writes the declaration.
func (h *Handler) Capture(in io.Reader) error {
	fault.Inject(fault.PointHookStart)

	raw, err := readPayload(in)
	if err != nil {
		return err
	}

	var p Payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return err
	}

	// Attribute before anything else can fail. A fault after this point is
	// still recorded against the session that produced it; only a payload too
	// malformed to name its session goes to the unattributed bucket.
	if p.SessionID != "" {
		h.sessionID = p.SessionID
	}
	h.toolUseID = p.ToolUseID

	fault.Inject(fault.PointHookParsed)

	decl := store.Declaration{
		Type:           store.TypeDeclaration,
		SchemaVersion:  store.SchemaVersion,
		RecordedAtMS:   h.now().UnixMilli(),
		ToolUseID:      p.ToolUseID,
		SessionID:      h.sessionID,
		PromptID:       nilIfEmpty(p.PromptID),
		AgentID:        nilIfEmpty(p.AgentID),
		AgentType:      nilIfEmpty(p.AgentType),
		TranscriptPath: p.TranscriptPath,
		PermissionMode: p.PermissionMode,
		ToolName:       p.ToolName,
		Shape:          shape.Derive(p.ToolName, p.ToolInput, h.st.Key()),
	}

	fault.Inject(fault.PointStoreWrite)

	if err := h.st.AppendDeclaration(decl); err != nil {
		return err
	}
	h.opened = true

	fault.Inject(fault.PointHookAfterDeclaration)
	return nil
}

// Close writes the terminal record for a captured declaration and the coverage
// record for this invocation. It is called on every path Capture can leave by,
// including a recovered panic.
//
// sig reports signal delivery. SIGTERM is what a hook timeout cancellation
// delivers and it is catchable, so it is a controlled exit and gets a terminal
// record. SIGKILL is not catchable: nothing runs, no terminal record appears,
// and that absence is what the end-of-run probe reads as an unterminated entry.
func (h *Handler) Close(sig *Signals, captureErr error) {
	outcome, reason := store.OutcomeOK, ""
	switch {
	case sig.Delivered():
		outcome, reason = store.OutcomeSignal, store.ReasonTerminatedBySignal
	case errors.Is(captureErr, store.ErrLockTimeout):
		outcome, reason = store.OutcomeError, store.ReasonLockTimeout
	case captureErr != nil:
		outcome, reason = store.OutcomeError, store.ReasonInternalError
	}

	// A terminal closes a declaration that landed, whatever its id. It is also
	// written when the declaration itself could not be written for want of the
	// lock: it then carries the only trace of that tool_use_id, and goes
	// straight to the spill file -- the ordered stream's lock has already cost
	// its full budget once, and a second wait would push the handler past the
	// hook timeout.
	if h.opened || (h.toolUseID != "" && reason == store.ReasonLockTimeout) {
		term := store.Terminal{
			Type:          store.TypeTerminal,
			SchemaVersion: store.SchemaVersion,
			RecordedAtMS:  h.now().UnixMilli(),
			ToolUseID:     h.toolUseID,
			SessionID:     h.sessionID,
			Outcome:       outcome,
			Reason:        nilIfEmpty(reason),
		}
		if reason == store.ReasonLockTimeout {
			_ = h.st.SpillTerminal(term)
		} else {
			_ = h.st.AppendTerminal(term)
		}
	}

	_ = h.st.AppendCoverage(BuildCoverage(h.st, h.sessionID, store.PhaseCall, reason, h.now()))
}

func readPayload(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxPayloadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxPayloadBytes {
		return nil, errPayloadTooLarge
	}
	return b, nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
