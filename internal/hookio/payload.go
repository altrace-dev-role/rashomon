package hookio

import (
	"bytes"
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

// payloadKeys matches the keys as the payload structs matched their fields:
// case-insensitively, the last match winning.
var payloadKeys = jsonscan.NewFoldKeySet(KeyNames()...)

// Intake reads one payload (Read), validates it with json.Valid and records
// the spans of its top-level keys in one walk (jsonscan.Top). It serves the
// buffered entries: hook, post and probe. A payload over the cap is refused
// with a *TooLargeError. A reader whose own error already is one -- the
// host guard's replay of Read's refusal -- has it passed on as it is: that
// refusal read the head, and this read, which sees no bytes, has none.
func Intake(r io.Reader) (*Payload, error) {
	raw, head, err := read(r)
	var tl *TooLargeError
	if errors.As(err, &tl) {
		return nil, tl
	}
	if errors.Is(err, ErrTooLarge) {
		return nil, refused(head)
	}
	if err != nil {
		return nil, err
	}
	return IntakeBytes(raw)
}

// IntakeBytes is Intake for a caller that already has its payload's bytes
// in hand -- internal/hostguard.EvaluateRaw, whose own doc explains why,
// and Intake above, which now defers to this for everything past its own
// Read. It never reads raw through Read a second time: a caller holding
// raw once must not be charged a second full copy of it just to turn it
// into a *Payload, the exact doubling internal/hookio's RES-28 doc on Read
// describes on the host-guard path this exists to close.
func IntakeBytes(raw []byte) (*Payload, error) {
	p := &Payload{Raw: raw}
	if err := jsonscan.Top(raw, payloadKeys, p.Spans[:]); err != nil {
		return nil, err
	}
	return p, nil
}

// TooLargeError is Intake's refusal of a payload over MaxPayloadBytes;
// errors.Is(err, ErrTooLarge) holds for it. The payload is not parsed, but
// its first 4 KiB were read before the cap was reached, and SessionID and CWD
// are read from them so the refusal is recorded in the run that sent it, as
// the supervisor scans that head (Technical spec 4.2.10). A value that does
// not end inside the head, or a head that is not one object's valid start,
// leaves both empty; a key repeated past the head is not seen.
type TooLargeError struct {
	SessionID, CWD string
}

func (e *TooLargeError) Error() string { return ErrTooLarge.Error() }

// Is makes errors.Is(err, ErrTooLarge) hold.
func (e *TooLargeError) Is(target error) bool { return target == ErrTooLarge }

// refused reads session_id and cwd out of an over-cap payload's head.
func refused(head []byte) *TooLargeError {
	var e TooLargeError
	paths := []jsonscan.Path{
		{Elems: []string{"session_id"}, Max: len(head), Mode: jsonscan.CaptureHead, String: true, Fold: true},
		{Elems: []string{"cwd"}, Max: len(head), Mode: jsonscan.CaptureHead, String: true, Fold: true},
	}
	err := IntakeStream(bytes.NewReader(head), paths, func(i int, v []byte, _ bool) bool {
		if i == 0 {
			e.SessionID = string(v)
		} else {
			e.CWD = string(v)
		}
		return true
	})
	// The head is cut inside the payload, so a sound head ends in the middle
	// of a value: anything else, or a cut sooner, names nothing.
	var se *jsonscan.SyntaxError
	if !errors.As(err, &se) || se.Offset != int64(len(head)) {
		return &TooLargeError{}
	}
	return &e
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

// StrField pairs a key with the string Strs decodes it into (Into).
type StrField struct {
	key int
	dst *string
}

// Into names the string key k decodes into.
func Into(k int, dst *string) StrField { return StrField{key: k, dst: dst} }

// Strs decodes each field into the string paired with its key, in order,
// and returns on the first one of the wrong type. Fields before it may
// already be written, which is safe because every caller discards them on
// error (handle.go, post.go, probe.go, paused.go): one bad field fails the
// whole intake, as it failed the struct decode.
func (p *Payload) Strs(fields ...StrField) error {
	for _, f := range fields {
		v, err := p.Str(f.key)
		if err != nil {
			return err
		}
		*f.dst = v
	}
	return nil
}
