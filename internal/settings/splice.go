package settings

import (
	"bytes"
	"encoding/json"
	"io"
)

// Textual splicing: the edits SetHookEntries and RemoveHookEvent make to a
// hooks object and to an event's array are made on the bytes the user wrote,
// not by re-rendering the level. Re-rendering put a one-line
// {"matcher": ..., "hooks": [...]} on three lines and the other way round, so
// a file that watch had touched could never be restored by detach to the bytes
// it started as.
//
// The two edits are exact inverses. An insertion goes immediately after the
// last element, as the separator the file already uses between its last two
// elements followed by the new element; the removal of that element deletes
// exactly the bytes from the end of the one before it to its own end.

// span is one element of an array, or one member of an object: key start (the
// element start for an array), value start, and value end.
type span struct{ start, vstart, end int }

// spansOf splits raw, an array or an object, into its elements' extents. ok is
// false when raw is not one.
func spansOf(raw []byte, object bool) (spans []span, ok bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, false
	}
	want := json.Delim('[')
	if object {
		want = '{'
	}
	if d, isDelim := tok.(json.Delim); !isDelim || d != want {
		return nil, false
	}
	prev := int(dec.InputOffset())
	for dec.More() {
		start := prev
		for start < len(raw) && (raw[start] == ' ' || raw[start] == '\t' || raw[start] == '\n' || raw[start] == '\r' || raw[start] == ',') {
			start++
		}
		if object {
			if _, err := dec.Token(); err != nil {
				return nil, false
			}
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return nil, false
		}
		end := int(dec.InputOffset())
		if object {
			spans = append(spans, span{start, end - len(v), end})
		} else {
			spans = append(spans, span{end - len(v), end - len(v), end})
		}
		prev = end
	}
	if _, err := dec.Token(); err != nil {
		return nil, false
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, false
	}
	return spans, true
}

// separator is the bytes this container puts between elements: the gap
// between its last two when it has two, else the comma and the line break and
// indentation (or the space) that the first element implies.
func separator(raw []byte, spans []span) []byte {
	if n := len(spans); n >= 2 {
		return raw[spans[n-2].end:spans[n-1].start]
	}
	gap := raw[1:spans[0].start]
	if i := bytes.LastIndexByte(gap, '\n'); i >= 0 {
		return append([]byte(","), gap[i:]...)
	}
	return []byte(", ")
}

// spliceAppend inserts a new last element of raw: value for an array, key and
// value for an object. ok is false when raw has no element to take the layout
// from, and the caller renders it whole.
func spliceAppend(raw []byte, object bool, key, value []byte) ([]byte, bool) {
	spans, ok := spansOf(raw, object)
	if !ok || len(spans) == 0 {
		return nil, false
	}
	sep := separator(raw, spans)
	if !bytes.ContainsRune(sep, '\n') {
		// A container written on one line keeps to it.
		var b bytes.Buffer
		if json.Compact(&b, value) == nil {
			value = b.Bytes()
		}
	}
	elem := value
	if object {
		elem = append(append(append([]byte{}, key...), ": "...), value...)
	}
	at := spans[len(spans)-1].end
	out := make([]byte, 0, len(raw)+len(sep)+len(elem))
	out = append(out, raw[:at]...)
	out = append(out, sep...)
	out = append(out, elem...)
	return append(out, raw[at:]...), true
}

// spliceRemove deletes element i of raw along with its separator. ok is false
// when it is the only element, which the caller removes with its container.
func spliceRemove(raw []byte, object bool, i int) ([]byte, bool) {
	spans, ok := spansOf(raw, object)
	if !ok || len(spans) < 2 || i < 0 || i >= len(spans) {
		return nil, false
	}
	from, to := spans[0].start, spans[1].start
	if i > 0 {
		from, to = spans[i-1].end, spans[i].end
	}
	return append(append([]byte{}, raw[:from]...), raw[to:]...), true
}

// spliceReplace replaces the value of member i of an object.
func spliceReplace(raw []byte, i int, value []byte) ([]byte, bool) {
	spans, ok := spansOf(raw, true)
	if !ok || i < 0 || i >= len(spans) {
		return nil, false
	}
	out := append([]byte{}, raw[:spans[i].vstart]...)
	out = append(out, value...)
	return append(out, raw[spans[i].end:]...), true
}
