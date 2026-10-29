package main

import (
	"io"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
)

// Final-message capture sizes. The account reads the first 64 Ki runes of
// the message (report's messageCap), which UTF-8 holds in at most 256 KiB, so
// the head keeps exactly what the account read when the whole message was
// decoded. The tail is captured beside it for the comparator (RES-17, P-40);
// nothing on this base reads it.
const (
	sessionIDBytes   = 4 << 10
	messageHeadBytes = 256 << 10
	messageTailBytes = 64 << 10
)

// Harness-signal capture sizes, read only when readFinalMessage is asked for
// them (withHarness). hookEventNameBytes and transcriptPathBytes are
// generous metadata caps, not content caps: 2.1.280's event names are a
// handful of bytes and sessionIDBytes already sets the precedent for a path,
// not a message, field. cursorVersionBytes gives internal/harness.Detect's
// own checkedVersion (a 32-byte ceiling before it drops the value) room to
// see a too-long value and reject it, rather than this capture silently
// truncating one into a false accept.
const (
	hookEventNameBytes  = 128
	transcriptPathBytes = 4 << 10
	cursorVersionBytes  = 64
)

// Stop-view capture sizes, read with the harness paths (card 2.4 R11). cwd is
// a path, capped as transcript_path is; a cwd over it names no project, as an
// over-long session_id names no session. stop_hook_active is a JSON boolean
// read as its literal: five bytes hold "false", and anything longer is not
// "true".
const (
	cwdBytes            = 4 << 10
	stopHookActiveBytes = 5
)

// finalMessage is the slice of a Stop or StopFailure payload recap and
// digest --stdin read.
type finalMessage struct {
	SessionID string
	Message   string // the head: the whole message up to messageHeadBytes
	Tail      string // the last messageTailBytes when the head was cut
	Harness   harnessSignal
	// CWD is the payload's cwd, the directory the project index is keyed on
	// (baseline.Key; card 2.4 R10); "" when absent or over cwdBytes.
	CWD string
	// StopHookActive is the payload's stop_hook_active. It is carried, not
	// read, in v0 (card 2.4 R11, D77).
	StopHookActive bool
}

// harnessSignal is the slice of a Stop payload internal/harness.Detect
// reads, captured by the same single stream pass as the rest of
// finalMessage so recap's one read of stdin never happens twice -- see
// readFinalMessage's withHarness. It carries exactly what Detect's own doc
// lists: HookEventName, TranscriptPath and CursorVersion as values (Detect
// calls Str on all three), and the other four as presence only (Detect
// calls Has, never Str, on conversation_id, generation_id, turn_id and
// model -- see Detect's doc on why user_email is read the same way and
// never appears here at all: nothing downstream of this capture needs it).
//
// HasCursorVersion is captured separately from CursorVersion's value
// (rather than inferred from an empty value) because the two differ for
// internal/hookio.Payload.Has: a payload carrying `"cursor_version":""`
// must still read as present, a bit Str's own empty-string return cannot
// distinguish from absent.
//
// One divergence from Has's own definition (internal/jsonscan.Span.Set,
// "present with a value other than null or false") is accepted here: the
// four presence-only fields are captured with jsonscan.CapturePresence,
// which (unlike Set) does not exclude a literal JSON false -- see
// cmd/rashomon's own doc on syntheticHarnessPayload for why that is a safe
// direction to be wrong in.
type harnessSignal struct {
	HookEventName     string
	TranscriptPath    string
	CursorVersion     string
	HasCursorVersion  bool
	HasConversationID bool
	HasGenerationID   bool
	HasTurnID         bool
	HasModel          bool
}

// countingReader counts what passes through it.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}

// readFinalMessage streams a Stop payload (hookio.IntakeStream): the message
// is never held whole, only its head and tail, and at most
// hookio.MaxPayloadBytes+1 bytes are read, so an endless stdin is refused
// rather than read forever. A payload that is not one object, is not valid,
// names a field with the wrong type, or is over the cap is no payload at all,
// as it was when the whole of it was decoded into a struct; keys are matched
// as that struct matched them, in any case. A repeated key is read from its
// last copy. A session_id over sessionIDBytes is no session named: the
// message is still read, and recap falls back to the newest run, as for a
// payload with no session_id.
//
// withHarness adds the paths only recapTurn's call reads: the harnessSignal
// paths, so H1's recap entry point gets what internal/harness.Detect reads
// from the same single stream pass that reads the message -- recap has no
// second read of stdin to spend on it (see cmd/rashomon's recapHostGuard) --
// and the Stop view's cwd and stop_hook_active (card 2.4 R11). digest
// --stdin's call passes false: it has no installed-for to stand down
// against and no Stop view. cwd is read as the three string fields are, so a
// cwd that is not a string is no payload. stop_hook_active is a boolean, so
// its path takes any value (a String path would refuse the whole payload on
// it, stream.go's ErrNotString) and only the literal true reads as set; a
// string "true" reads as set too, which the field's v0 use, none, cannot
// tell apart.
func readFinalMessage(in io.Reader, withSession, withHarness bool) (finalMessage, bool) {
	var fm finalMessage
	paths := []jsonscan.Path{
		{Elems: []string{"session_id"}, Max: sessionIDBytes, Mode: jsonscan.CaptureHead, String: true, Fold: true},
		{Elems: []string{"last_assistant_message"}, Max: messageHeadBytes, Mode: jsonscan.CaptureHead, String: true, Fold: true},
		{Elems: []string{"last_assistant_message"}, Max: messageTailBytes, Mode: jsonscan.CaptureTail, String: true, Fold: true},
	}
	if !withSession {
		// digest --stdin never read session_id, so its type cannot fail it.
		paths[0] = jsonscan.Path{Elems: []string{"session_id"}, Mode: jsonscan.CapturePresence}
	}
	if withHarness {
		paths = append(paths,
			jsonscan.Path{Elems: []string{"hook_event_name"}, Max: hookEventNameBytes, Mode: jsonscan.CaptureHead, String: true, Fold: true},
			jsonscan.Path{Elems: []string{"transcript_path"}, Max: transcriptPathBytes, Mode: jsonscan.CaptureHead, String: true, Fold: true},
			jsonscan.Path{Elems: []string{"cursor_version"}, Max: cursorVersionBytes, Mode: jsonscan.CaptureHead, String: true, Fold: true},
			jsonscan.Path{Elems: []string{"cursor_version"}, Mode: jsonscan.CapturePresence, Fold: true},
			jsonscan.Path{Elems: []string{"conversation_id"}, Mode: jsonscan.CapturePresence, Fold: true},
			jsonscan.Path{Elems: []string{"generation_id"}, Mode: jsonscan.CapturePresence, Fold: true},
			jsonscan.Path{Elems: []string{"turn_id"}, Mode: jsonscan.CapturePresence, Fold: true},
			jsonscan.Path{Elems: []string{"model"}, Mode: jsonscan.CapturePresence, Fold: true},
			jsonscan.Path{Elems: []string{"cwd"}, Max: cwdBytes, Mode: jsonscan.CaptureHead, String: true, Fold: true},
			jsonscan.Path{Elems: []string{"stop_hook_active"}, Max: stopHookActiveBytes, Mode: jsonscan.CaptureHead, Fold: true},
		)
	}
	cut := false
	cr := &countingReader{r: io.LimitReader(in, hookio.MaxPayloadBytes+1)}
	// The head is passed before the tail of the same value (path order).
	err := hookio.IntakeStream(cr, paths, func(i int, v []byte, truncated bool) bool {
		switch i {
		case 0:
			fm.SessionID = string(v)
			if truncated {
				fm.SessionID = ""
			}
		case 1:
			fm.Message, cut = string(v), truncated
		case 2:
			fm.Tail = ""
			if cut {
				fm.Tail = string(v)
			}
		case 3:
			fm.Harness.HookEventName = string(v)
		case 4:
			fm.Harness.TranscriptPath = string(v)
		case 5:
			fm.Harness.CursorVersion = string(v)
		case 6:
			fm.Harness.HasCursorVersion = true
		case 7:
			fm.Harness.HasConversationID = true
		case 8:
			fm.Harness.HasGenerationID = true
		case 9:
			fm.Harness.HasTurnID = true
		case 10:
			fm.Harness.HasModel = true
		case 11:
			fm.CWD = string(v)
			if truncated {
				fm.CWD = ""
			}
		case 12:
			fm.StopHookActive = string(v) == "true" && !truncated
		}
		return true
	})
	if err != nil || cr.n == 0 || cr.n > hookio.MaxPayloadBytes {
		return finalMessage{}, false
	}
	return fm, true
}
