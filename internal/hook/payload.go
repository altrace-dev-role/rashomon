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
// The fields are filled from the one scan hookio.Intake made of the payload;
// nothing decodes it a second time. ToolInput stays raw, a subslice of the
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
	if err := in.Strs([]*string{&p.SessionID, &p.PromptID, &p.TranscriptPath, &p.CWD, &p.PermissionMode,
		&p.HookEventName, &p.ToolName, &p.ToolUseID, &p.AgentID, &p.AgentType},
		hookio.SessionID, hookio.PromptID, hookio.TranscriptPath, hookio.CWD, hookio.PermissionMode,
		hookio.HookEventName, hookio.ToolName, hookio.ToolUseID, hookio.AgentID, hookio.AgentType); err != nil {
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
	return in.Strs([]*string{&p.SessionID, &p.CWD}, hookio.SessionID, hookio.CWD)
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
	}
	return store.ReasonInternalError
}
