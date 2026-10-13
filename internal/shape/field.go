package shape

import (
	"encoding/json"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
)

// inputField finds one named field of a tool input. The input is a subslice
// of a payload hookio.Intake validated, so nothing here decodes it into a
// map: one walk finds the key, and only that field's bytes are read.
func inputField(raw json.RawMessage, name string) (jsonscan.Span, bool) {
	if len(raw) == 0 {
		return jsonscan.Span{}, false
	}
	s, err := jsonscan.Lookup(raw, jsonscan.ValueSpan(raw), name)
	if err != nil || !s.Present() {
		return jsonscan.Span{}, false
	}
	return s, true
}

// stringValue decodes a field that must be a string. null reads as empty, as
// decoding null into a string did; any other type is no string at all.
func stringValue(raw json.RawMessage, s jsonscan.Span) (string, bool) {
	switch s.Kind {
	case jsonscan.Null:
		return "", true
	case jsonscan.String:
		// An invalid byte decodes to three, so 3*Len bounds the decoded size.
		out, _, err := jsonscan.Head(raw, s, 3*int(s.Len), nil)
		if err != nil {
			return "", false
		}
		return string(out), true
	}
	return "", false
}
