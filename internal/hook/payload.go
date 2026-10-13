// Package hook turns a PreToolUse or PostToolUse payload into records.
package hook

import (
	"encoding/json"
	"errors"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Payload is the PreToolUse hook input.
//
// The fields are filled from the spans hookio.Intake recorded; nothing
// decodes the payload. ToolInput stays raw, a subslice of the
// payload: only shape is allowed to look inside it. scratchpad_dir, which an
// earlier struct declared and never read, is not among the keys scanned.
type Payload struct {
	SessionID      string
	PromptID       string
	TranscriptPath string
	CWD            string
	PermissionMode string
	HookEventName  string
	ToolName       string
	ToolInput      json.RawMessage
	ToolUseID      string
	AgentID        string
	AgentType      string
}

// from fills p from an intake. A string field holding another type fails the
// whole payload before any field is used, as the struct decode did.
func (p *Payload) from(in *hookio.Payload) error {
	if err := in.Strs(
		hookio.Into(hookio.SessionID, &p.SessionID), hookio.Into(hookio.PromptID, &p.PromptID),
		hookio.Into(hookio.TranscriptPath, &p.TranscriptPath), hookio.Into(hookio.CWD, &p.CWD),
		hookio.Into(hookio.PermissionMode, &p.PermissionMode), hookio.Into(hookio.HookEventName, &p.HookEventName),
		hookio.Into(hookio.ToolName, &p.ToolName), hookio.Into(hookio.ToolUseID, &p.ToolUseID),
		hookio.Into(hookio.AgentID, &p.AgentID), hookio.Into(hookio.AgentType, &p.AgentType)); err != nil {
		return err
	}
	p.ToolInput = in.Value(hookio.ToolInput)
	return nil
}

// sessionPayload is the part of a SessionStart or SessionEnd payload this
// program reads. The rest -- source, reason -- is not needed and not kept.
// CWD is read because the novelty baseline is per project and the project is
// derived from this path.
type sessionPayload struct {
	SessionID string
	CWD       string
}

func (p *sessionPayload) from(in *hookio.Payload) error {
	return in.Strs(hookio.Into(hookio.SessionID, &p.SessionID), hookio.Into(hookio.CWD, &p.CWD))
}

// refusedSession is the session and cwd the head of an over-cap payload
// names (hookio.TooLargeError), so that its refusal is recorded in the run
// that sent it. ok is false for any other error, and for a head that names
// no session, which stays unattributed.
func refusedSession(err error) (sessionID, cwd string, ok bool) {
	var tl *hookio.TooLargeError
	if !errors.As(err, &tl) || tl.SessionID == "" {
		return "", "", false
	}
	return tl.SessionID, tl.CWD, true
}

// closeReason maps a capture error to its coverage reason.
func closeReason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, store.ErrLockTimeout):
		return store.ReasonLockTimeout
	case errors.Is(err, hookio.ErrTooLarge):
		return store.ReasonPayloadTooLarge
	case errors.Is(err, store.ErrRecordOversize):
		return store.ReasonRecordOversize
	}
	return store.ReasonInternalError
}

// attribute is the session a payload's id names: UnattributedSession for a
// payload that names none, and for an id over store.MaxIDBytes, which a
// record cannot carry and which must not be cut into another session's id
// (REC-52).
func attribute(id string) string {
	if id == "" || len(id) > store.MaxIDBytes {
		return UnattributedSession
	}
	return id
}
