package hookio

import (
	"errors"
	"io"
	"strconv"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
)

// The keys a Payload holds spans for, by index into Payload.Spans. The order
// is frozen (P-10, 2026-10-13): later PRs append keys, never reorder them.
const (
	SessionID = iota
	PromptID
	AgentID
	AgentType
	HookEventName
	ToolName
	ToolUseID
	CWD
	PermissionMode
	TranscriptPath
	ToolInput
	ToolResponse
	Error
	IsInterrupt
	DurationMS
	CursorVersion
	ConversationID
	GenerationID
	TurnID
	Model
	UserEmail // presence only: no consumer may read its value
	MCPServer
	NumKeys
)

// KeyNames returns the payload keys in Spans order.
func KeyNames() []string {
	return []string{
		"session_id", "prompt_id", "agent_id", "agent_type", "hook_event_name",
		"tool_name", "tool_use_id", "cwd", "permission_mode", "transcript_path",
		"tool_input", "tool_response", "error", "is_interrupt", "duration_ms",
		"cursor_version", "conversation_id", "generation_id", "turn_id", "model",
		"user_email", "mcp_server",
	}
}

// ErrType reports a key whose value has a type its consumer cannot take, the
// case in which decoding into the old payload structs failed.
var ErrType = errors.New("hookio: payload value has the wrong type")

// Payload is one intake: the raw bytes and the spans of every top-level key
// any consumer in this entry may read. It is built once and passed on; no
// consumer may call json.Unmarshal on Raw.
type Payload struct {
	Raw   []byte
	Spans [NumKeys]jsonscan.Span
}

// Intake reads one payload (Read), validates it and scans it once
// (jsonscan.Top). It serves the buffered entries: hook, post and probe.
func Intake(r io.Reader) (*Payload, error) {
	raw, err := Read(r)
	if err != nil {
		return nil, err
	}
	p := &Payload{Raw: raw}
	if err := jsonscan.Top(raw, jsonscan.NewKeySet(KeyNames()...), p.Spans[:]); err != nil {
		return nil, err
	}
	return p, nil
}

// Has reports whether key k was present with a value other than null or
// false: the presence bit harness detection reads.
func (p *Payload) Has(k int) bool { return p.Spans[k].Set() }

// Value returns key k's raw encoded bytes, a subslice of Raw, or nil.
func (p *Payload) Value(k int) []byte { return p.Spans[k].Raw(p.Raw) }

// Str decodes a small string field, as decoding it into a string did: null
// and absent are empty, any other non-string is ErrType. It is for metadata
// (ids, names, paths); large fields are read through Head, Tail or Lines.
func (p *Payload) Str(k int) (string, error) {
	s := p.Spans[k]
	switch s.Kind {
	case jsonscan.Absent, jsonscan.Null:
		return "", nil
	case jsonscan.String:
	default:
		return "", ErrType
	}
	// An invalid byte decodes to three, so 3*Len bounds the decoded size.
	out, _, err := jsonscan.Head(p.Raw, s, 3*int(s.Len), nil)
	return string(out), err
}

// Bool decodes an optional boolean: nil when absent or null.
func (p *Payload) Bool(k int) (*bool, error) {
	var b bool
	switch p.Spans[k].Kind {
	case jsonscan.Absent, jsonscan.Null:
		return nil, nil
	case jsonscan.True:
		b = true
	case jsonscan.False:
	default:
		return nil, ErrType
	}
	return &b, nil
}

// Int decodes an optional integer: nil when absent or null, ErrType for a
// number that is not an int64, as decoding into *int64 failed.
func (p *Payload) Int(k int) (*int64, error) {
	switch p.Spans[k].Kind {
	case jsonscan.Absent, jsonscan.Null:
		return nil, nil
	case jsonscan.Number:
	default:
		return nil, ErrType
	}
	n, err := strconv.ParseInt(string(p.Value(k)), 10, 64)
	if err != nil {
		return nil, ErrType
	}
	return &n, nil
}

// Strs decodes the string fields ks into dst, in order, and returns on the
// first one of the wrong type. Fields before it may already be written to
// dst, which is safe because every caller discards dst on error (handle.go,
// post.go, probe.go, paused.go): one bad field fails the whole intake, as it
// failed the struct decode.
func (p *Payload) Strs(dst []*string, ks ...int) error {
	for i, k := range ks {
		v, err := p.Str(k)
		if err != nil {
			return err
		}
		*dst[i] = v
	}
	return nil
}
