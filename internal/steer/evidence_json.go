package steer

import (
	"strconv"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
)

// The evidence check reads a steer line and a transcript record with
// jsonscan, never a decode (RES-5: a hook path locates fields), so the one
// transcript read of a Stop allocates the strings it compares and little else.

// noteKeyNames are the keys of a steer line, or of an EvidenceLine, a Stop
// reads, in the order of noteKey's constants.
var noteKeyNames = []string{"type", "id", "steer_id", "at_unix_ms", "session_id", "prompt_id", "agent_id", "event", "state",
	"model_text", "client_version", "verified_floor", "excluded"}

const (
	nkType = iota
	nkID
	nkSteerID
	nkAt
	nkSession
	nkPrompt
	nkAgent
	nkEvent
	nkState
	nkModel
	nkVersion
	nkFloor
	nkExcluded
	nkCount
)

var noteKeysOnce struct {
	sync.Once
	v *jsonscan.KeySet
}

// noteKeys is the one KeySet a steer line is read with, built on first use and
// never at init (RES-2).
func noteKeys() *jsonscan.KeySet {
	noteKeysOnce.Do(func() { noteKeysOnce.v = jsonscan.NewKeySet(noteKeyNames...) })
	return noteKeysOnce.v
}

var recordKeysOnce struct {
	sync.Once
	rec, att *jsonscan.KeySet
}

// recordKeys are the KeySets a transcript record is read with: the record's
// time and attachment, and the attachment's type and content.
func recordKeys() (rec, att *jsonscan.KeySet) {
	recordKeysOnce.Do(func() {
		recordKeysOnce.rec = jsonscan.NewKeySet("timestamp", "attachment")
		recordKeysOnce.att = jsonscan.NewKeySet("type", "content")
	})
	return recordKeysOnce.rec, recordKeysOnce.att
}

// textOf is a string span's text, "" for any other kind or a bad escape.
func textOf(b []byte, sp jsonscan.Span) string {
	if sp.Kind != jsonscan.String {
		return ""
	}
	s, err := jsonscan.DecodeString(b, sp)
	if err != nil {
		return ""
	}
	return s
}

// int64Of is a number span's value, 0 for any other kind or a value that is
// not an integer.
func int64Of(b []byte, sp jsonscan.Span) int64 {
	if sp.Kind != jsonscan.Number {
		return 0
	}
	n, err := strconv.ParseInt(string(sp.Raw(b)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// stringsOf is a span's strings: the elements of an array that are strings, in
// order, up to the first element that is not one, or the one string of a string
// span; nothing for any other kind.
func stringsOf(b []byte, sp jsonscan.Span) []string {
	switch sp.Kind {
	case jsonscan.String:
		if s := textOf(b, sp); s != "" {
			return []string{s}
		}
		return nil
	case jsonscan.Array:
	default:
		return nil
	}
	var out []string
	end := int(sp.Off + sp.Len - 1)
	for i := int(sp.Off) + 1; i < end; {
		switch b[i] {
		case ' ', '\t', '\n', '\r', ',':
			i++
			continue
		case '"':
		default:
			return out
		}
		j := i + 1
		for j < end && b[j] != '"' {
			if b[j] == '\\' {
				j++
			}
			j++
		}
		if j >= end {
			return out
		}
		if s := textOf(b, jsonscan.Span{Off: int32(i), Len: int32(j + 1 - i), Kind: jsonscan.String}); s != "" {
			out = append(out, s)
		}
		i = j + 1
	}
	return out
}
