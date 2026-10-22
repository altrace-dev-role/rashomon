// Package jsonscan reads fields out of a JSON payload without decoding it.
//
// A hook payload is attacker-influenced and can be 8 MiB, and decoding it into
// Go values copied it about eight times. This package locates values instead:
// Top validates the buffer once with encoding/json's json.Valid and walks the
// top-level object once, recording each wanted key's byte range. Large string
// fields are then read through fixed buffers (Head, Tail, Lines) and never
// become a Go string. Stream does the same for payloads that are never held.
//
// The walker never decides validity itself: Top and Field rely on json.Valid
// for that verdict, and Stream, which validates as it goes, is fuzzed against
// encoding/json as its oracle. The package has no dependencies beyond the
// standard library and builds nothing at init.
package jsonscan

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"unicode"
	"unicode/utf8"
)

// Kind is the JSON type of a span. Absent means the key was not present.
type Kind uint8

// The kinds a span can carry.
const (
	Absent Kind = iota
	String
	Number
	Object
	Array
	True
	False
	Null
)

// Span is a value's byte range inside a buffer the caller owns. It holds the
// raw encoded bytes; string spans include their quotes and are NOT unescaped.
type Span struct {
	Off, Len int32
	Kind     Kind
}

// Present reports a key that was in the object, whatever its value.
func (s Span) Present() bool { return s.Kind != Absent }

// Set reports a key whose value is neither absent, null nor false: the
// presence bit harness detection and background detection read.
func (s Span) Set() bool { return s.Kind != Absent && s.Kind != Null && s.Kind != False }

// Raw returns the span's encoded bytes, or nil when it is absent or does not
// lie inside buf.
func (s Span) Raw(buf []byte) []byte {
	if s.Kind == Absent || !s.inside(buf) {
		return nil
	}
	return buf[s.Off : s.Off+s.Len]
}

func (s Span) inside(buf []byte) bool {
	return s.Off >= 0 && s.Len >= 0 && int64(s.Off)+int64(s.Len) <= int64(len(buf))
}

// Errors returned by Top and Field.
var (
	ErrInvalid   = errors.New("jsonscan: invalid JSON")
	ErrNotObject = errors.New("jsonscan: value is not an object")
	ErrNotString = errors.New("jsonscan: value is not a string")
	ErrTooLarge  = errors.New("jsonscan: buffer exceeds the span range")
	ErrSpan      = errors.New("jsonscan: span outside the buffer")
	ErrShortOut  = errors.New("jsonscan: output has fewer slots than keys")
)

// KeySet is a fixed list of keys a caller wants. Each key's slot in Top's
// output is its position in the list given to NewKeySet or NewFoldKeySet.
type KeySet struct {
	sorted  []string
	slot    []int
	longest int
	fold    bool
}

// NewKeySet builds a KeySet that matches keys exactly after unescaping, as
// decoding into a map does. Keys must be distinct; a repeated key's later
// copies are never filled.
func NewKeySet(keys ...string) *KeySet {
	return newKeySet(keys, false)
}

// NewFoldKeySet builds a KeySet that matches keys as decoding into a struct
// matches a field's name: exactly, or else case-insensitively under Unicode
// simple folding (strings.EqualFold), so "SESSION_ID" fills session_id's slot.
// Keys must be distinct when folded.
func NewFoldKeySet(keys ...string) *KeySet {
	return newKeySet(keys, true)
}

func newKeySet(keys []string, fold bool) *KeySet {
	ks := &KeySet{sorted: make([]string, len(keys)), slot: make([]int, len(keys)), fold: fold}
	order := make([]int, len(keys))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return keys[order[a]] < keys[order[b]] })
	for i, o := range order {
		ks.sorted[i], ks.slot[i] = keys[o], o
		if len(keys[o]) > ks.longest {
			ks.longest = len(keys[o])
		}
	}
	return ks
}

// Len is the number of keys, and the length out must have.
func (k *KeySet) Len() int { return len(k.sorted) }

// find returns the caller's slot for an unescaped key, or -1.
func (k *KeySet) find(key []byte) int {
	i := sort.Search(len(k.sorted), func(i int) bool { return k.sorted[i] >= string(key) })
	if i < len(k.sorted) && k.sorted[i] == string(key) {
		return k.slot[i]
	}
	if k.fold {
		for i, want := range k.sorted {
			if equalFold(key, want) {
				return k.slot[i]
			}
		}
	}
	return -1
}

// limit is the longest unescaped key that can match: the longest wanted key,
// or for a fold set that many characters of up to utf8.UTFMax bytes each,
// since a folded character can take more bytes than the one it matches.
func (k *KeySet) limit() int {
	n := k.longest
	if k.fold {
		n *= utf8.UTFMax
	}
	return min(n, maxKeyBytes)
}

// maxKeyBytes bounds the stack buffer an escaped key is unescaped into. A key
// longer than every wanted key cannot match one and is not unescaped further.
const maxKeyBytes = 128

// lookup finds the slot of a raw key body (the bytes between its quotes).
// A key with escapes is unescaped first, so "session_id" matches
// session_id as encoding/json would decode it.
func (k *KeySet) lookup(body []byte) int {
	if indexByte(body, '\\') < 0 {
		return k.find(body)
	}
	var tmp [maxKeyBytes]byte
	out, truncated := unescapeHead(body, tmp[:0], k.limit())
	if truncated {
		return -1
	}
	return k.find(out)
}

// Top validates buf with json.Valid, then walks the top-level object once and
// fills out[i] with the span of keys' i-th key. It allocates nothing per
// value. Duplicate keys: the last one wins, as encoding/json does. A top-level
// null leaves every key absent, as decoding it into a map does.
func Top(buf []byte, keys *KeySet, out []Span) error {
	if len(out) < keys.Len() {
		return ErrShortOut
	}
	clear(out[:keys.Len()])
	if len(buf) > math.MaxInt32 {
		return ErrTooLarge
	}
	if !json.Valid(buf) {
		return ErrInvalid
	}
	i := skipWS(buf, 0)
	if i < len(buf) && buf[i] == 'n' {
		return nil
	}
	return walkObject(buf, i, len(buf), keys, out)
}

// Field is Top applied inside an object span of the same, already validated,
// buffer (tool_input, tool_response). An absent or null obj leaves every key
// absent; any other non-object is ErrNotObject.
//
// Precondition: buf has passed json.Valid, as Top's buffer has. Field does
// not validate: it finds the object's end by bracket matching, so on bytes
// json.Valid would refuse it may still report keys, and anything after the
// object's closing brace is never looked at. Lookup and ValueSpan rely on the
// same precondition.
func Field(buf []byte, obj Span, keys *KeySet, out []Span) error {
	if len(out) < keys.Len() {
		return ErrShortOut
	}
	clear(out[:keys.Len()])
	switch obj.Kind {
	case Absent, Null:
		return nil
	case Object:
	default:
		return ErrNotObject
	}
	if !obj.inside(buf) {
		return ErrSpan
	}
	end := int(obj.Off) + int(obj.Len)
	return walkObject(buf[:end], int(obj.Off), end, keys, out)
}

// walkObject records the spans of wanted keys in the object at buf[i]. The
// buffer has been validated; every access is still bounds-checked, so a span
// from some other buffer yields ErrInvalid rather than a panic.
func walkObject(buf []byte, i, end int, keys *KeySet, out []Span) error {
	if i >= end || buf[i] != '{' {
		return ErrNotObject
	}
	i = skipWS(buf, i+1)
	if i < end && buf[i] == '}' {
		return nil
	}
	for i < end {
		if buf[i] != '"' {
			return ErrInvalid
		}
		ke, ok := skipString(buf, i)
		if !ok {
			return ErrInvalid
		}
		slot := keys.lookup(buf[i+1 : ke-1])
		i = skipWS(buf, ke)
		if i >= end || buf[i] != ':' {
			return ErrInvalid
		}
		i = skipWS(buf, i+1)
		if i >= end {
			return ErrInvalid
		}
		kind := kindOf(buf[i])
		ve, ok := skipValue(buf, i)
		if !ok || kind == Absent {
			return ErrInvalid
		}
		if slot >= 0 {
			out[slot] = Span{Off: int32(i), Len: int32(ve - i), Kind: kind}
		}
		i = skipWS(buf, ve)
		if i >= end {
			return ErrInvalid
		}
		switch buf[i] {
		case ',':
			i = skipWS(buf, i+1)
		case '}':
			return nil
		default:
			return ErrInvalid
		}
	}
	return ErrInvalid
}

func kindOf(c byte) Kind {
	switch {
	case c == '"':
		return String
	case c == '{':
		return Object
	case c == '[':
		return Array
	case c == 't':
		return True
	case c == 'f':
		return False
	case c == 'n':
		return Null
	case c == '-' || (c >= '0' && c <= '9'):
		return Number
	}
	return Absent
}

func isWS(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

func skipWS(buf []byte, i int) int {
	for i < len(buf) && isWS(buf[i]) {
		i++
	}
	return i
}

// skipString returns the index just past the string starting at buf[i]. In
// a valid string a backslash only begins an escape, so a quote preceded by an
// even run of backslashes is the closing one.
func skipString(buf []byte, i int) (int, bool) {
	j := i + 1
	for {
		q := indexByte(buf[j:], '"')
		if q < 0 {
			return len(buf), false
		}
		j += q
		n := 0
		for k := j - 1; k > i && buf[k] == '\\'; k-- {
			n++
		}
		if n%2 == 0 {
			return j + 1, true
		}
		j++
	}
}

// skipValue returns the index just past the value starting at buf[i].
func skipValue(buf []byte, i int) (int, bool) {
	switch buf[i] {
	case '"':
		return skipString(buf, i)
	case '{', '[':
		depth := 0
		for i < len(buf) {
			switch buf[i] {
			case '"':
				e, ok := skipString(buf, i)
				if !ok {
					return len(buf), false
				}
				i = e
				continue
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					return i + 1, true
				}
			}
			i++
		}
		return len(buf), false
	}
	j := i
	for j < len(buf) && !isWS(buf[j]) && buf[j] != ',' && buf[j] != '}' && buf[j] != ']' {
		j++
	}
	return j, j > i
}

// ValueSpan is the span of the one value buf holds, whitespace trimmed: a
// tool_input subslice handed to a package that only has its bytes.
func ValueSpan(buf []byte) Span {
	i, j := skipWS(buf, 0), len(buf)
	for j > i && isWS(buf[j-1]) {
		j--
	}
	if i >= j || len(buf) > math.MaxInt32 {
		return Span{}
	}
	return Span{Off: int32(i), Len: int32(j - i), Kind: kindOf(buf[i])}
}

// Lookup is Field for one key, matched exactly: the span of key in the
// object at obj, the last if it repeats, Absent when it is missing. buf must
// have passed json.Valid (Field).
func Lookup(buf []byte, obj Span, key string) (Span, error) {
	ks := KeySet{sorted: []string{key}, slot: []int{0}, longest: len(key)}
	var out [1]Span
	err := Field(buf, obj, &ks, out[:])
	return out[0], err
}

// equalFold is strings.EqualFold for a byte key against a wanted key, without
// converting the key to a string.
func equalFold(b []byte, s string) bool {
	for len(b) > 0 && len(s) > 0 {
		r, n := rune(b[0]), 1
		if r >= utf8.RuneSelf {
			r, n = utf8.DecodeRune(b)
		}
		b = b[n:]
		t, m := rune(s[0]), 1
		if t >= utf8.RuneSelf {
			t, m = utf8.DecodeRuneInString(s)
		}
		s = s[m:]
		if r == t {
			continue
		}
		if t < r {
			r, t = t, r
		}
		if t < utf8.RuneSelf {
			if 'A' <= r && r <= 'Z' && t == r+'a'-'A' {
				continue
			}
			return false
		}
		f := unicode.SimpleFold(r)
		for f != r && f < t {
			f = unicode.SimpleFold(f)
		}
		if f != t {
			return false
		}
	}
	return len(b) == 0 && len(s) == 0
}
