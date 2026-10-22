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

// finalMessage is the slice of a Stop or StopFailure payload recap and
// digest --stdin read.
type finalMessage struct {
	SessionID string
	Message   string // the head: the whole message up to messageHeadBytes
	Tail      string // the last messageTailBytes when the head was cut
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
func readFinalMessage(in io.Reader, withSession bool) (finalMessage, bool) {
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
		}
		return true
	})
	if err != nil || cr.n == 0 || cr.n > hookio.MaxPayloadBytes {
		return finalMessage{}, false
	}
	return fm, true
}
