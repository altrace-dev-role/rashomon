package spec

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// The document tree: what a spec.json says, before the schema reads it, and
// its RFC 8785 form (archive 20.2.8, Digest). encoding/json cannot be the
// reader alone: it matches keys without regard to case, keeps the last of a
// duplicate, turns null into a zero value and turns an unpaired surrogate
// escape into U+FFFD, and each of those would make two different files one
// spec or one spec two digests.

type nodeKind uint8

const (
	kObject nodeKind = iota + 1
	kArray
	kString
	kInt
	kBool
	kNull
)

// node is one JSON value. Invariant: an object's keys are distinct and in file
// order with vals parallel to them; an array's elements are vals; an integer is
// within +-maxSafeInt, so its canonical form is its decimal digits.
type node struct {
	kind nodeKind
	str  string
	num  int64
	flag bool
	keys []string
	vals []node
}

// maxSafeInt is 2^53-1, the largest integer RFC 8785's number form (ECMAScript
// Number) writes exactly.
const maxSafeInt = 1<<53 - 1

// readTree reads data as one JSON value under the lexical rules of archive
// 20.2.8: at most MaxBytes, valid UTF-8, no unpaired surrogate escape, no
// duplicate key, integers only, nesting at most MaxDepth. Cost: O(n) time and
// space in the n bytes, recursion at most MaxDepth deep. It keeps no
// reference to data.
func readTree(data []byte) (node, *Error) {
	if len(data) > MaxBytes {
		return node{}, &Error{Reason: ReasonTooLarge, Limit: MaxBytes}
	}
	b := &builder{dec: json.NewDecoder(bytes.NewReader(data))}
	b.dec.UseNumber()
	b.bad, b.badReason = lexicalFault(data)
	root := b.value(0)
	if b.err == nil {
		if _, err := b.dec.Token(); err != io.EOF {
			b.fail(ReasonSyntax, 0)
		}
	}
	if b.err != nil {
		return node{}, b.err
	}
	return root, nil
}

// builder reads tokens into a node. Invariant: err is set by the first
// failure only, and path is the JSON pointer segments of the value being read.
type builder struct {
	dec       *json.Decoder
	path      []string
	err       *Error
	bad       int // offset of the first invalid UTF-8 or unpaired surrogate, or -1
	badReason Reason
}

func (b *builder) fail(r Reason, limit int) {
	if b.err == nil {
		b.err = &Error{Pointer: pointer(b.path), Reason: r, Limit: limit}
	}
}

// next reads one token. A token whose bytes end after the first lexical fault
// is where that fault is reported, so its pointer is the value it is in.
func (b *builder) next() json.Token {
	if b.err != nil {
		return nil
	}
	t, err := b.dec.Token()
	if err != nil {
		b.fail(ReasonSyntax, 0)
		return nil
	}
	if b.bad >= 0 && int(b.dec.InputOffset()) > b.bad {
		b.fail(b.badReason, 0)
	}
	return t
}

func (b *builder) value(depth int) node {
	t := b.next()
	if b.err != nil {
		return node{}
	}
	switch t := t.(type) {
	case json.Delim:
		if depth+1 > MaxDepth {
			b.fail(ReasonTooDeep, MaxDepth)
			return node{}
		}
		if t == '{' {
			return b.object(depth + 1)
		}
		return b.array(depth + 1)
	case string:
		return node{kind: kString, str: t}
	case json.Number:
		return b.number(t)
	case bool:
		return node{kind: kBool, flag: t}
	case nil:
		return node{kind: kNull}
	}
	b.fail(ReasonSyntax, 0)
	return node{}
}

func (b *builder) object(depth int) node {
	n := node{kind: kObject}
	seen := map[string]struct{}{}
	for b.err == nil && b.dec.More() {
		key, _ := b.next().(string)
		if b.err != nil {
			break
		}
		b.path = append(b.path, key)
		if _, dup := seen[key]; dup {
			b.fail(ReasonDuplicateKey, 0)
		}
		seen[key] = struct{}{}
		v := b.value(depth)
		b.path = b.path[:len(b.path)-1]
		n.keys, n.vals = append(n.keys, key), append(n.vals, v)
	}
	b.next() // the closing brace
	return n
}

func (b *builder) array(depth int) node {
	n := node{kind: kArray}
	for i := 0; b.err == nil && b.dec.More(); i++ {
		b.path = append(b.path, strconv.Itoa(i))
		v := b.value(depth)
		b.path = b.path[:len(b.path)-1]
		n.vals = append(n.vals, v)
	}
	b.next() // the closing bracket
	return n
}

func (b *builder) number(t json.Number) node {
	v, err := strconv.ParseInt(string(t), 10, 64)
	switch {
	case errors.Is(err, strconv.ErrRange), v > maxSafeInt, v < -maxSafeInt:
		b.fail(ReasonIntegerRange, 0)
	case err != nil:
		b.fail(ReasonNotInteger, 0)
	}
	return node{kind: kInt, num: v}
}

// lexicalFault returns the offset and reason of the first byte sequence in
// data that is not UTF-8, or the first \uD800-\uDFFF escape inside a string
// that does not form a pair, and -1 when there is none. The decoder would
// replace either with U+FFFD, so it is found here, on the raw bytes. A
// literal U+FFFD is text and is not a fault.
func lexicalFault(data []byte) (int, Reason) {
	inString := false
	for i := 0; i < len(data); {
		c := data[i]
		switch {
		case c >= utf8.RuneSelf:
			r, size := utf8.DecodeRune(data[i:])
			if r == utf8.RuneError && size == 1 {
				return i, ReasonNotUTF8
			}
			i += size
		case !inString:
			inString = c == '"'
			i++
		case c == '"':
			inString = false
			i++
		case c == '\\' && i+1 < len(data) && data[i+1] == 'u':
			unit, ok := hex4(data, i+2)
			switch {
			case !ok:
				i += 2
			case unit >= 0xDC00 && unit <= 0xDFFF:
				return i, ReasonSurrogate
			case unit >= 0xD800 && unit <= 0xDBFF:
				low, ok := rune(0), false
				if i+7 < len(data) && data[i+6] == '\\' && data[i+7] == 'u' {
					low, ok = hex4(data, i+8)
				}
				if !ok || low < 0xDC00 || low > 0xDFFF {
					return i, ReasonSurrogate
				}
				i += 12
			default:
				i += 6
			}
		case c == '\\':
			i += 2
		default:
			i++
		}
	}
	return -1, ""
}

// hex4 reads the four hex digits at data[at:], or reports they are not there.
func hex4(data []byte, at int) (rune, bool) {
	if at+4 > len(data) {
		return 0, false
	}
	v, err := strconv.ParseUint(string(data[at:at+4]), 16, 32)
	return rune(v), err == nil
}

// pointer renders segments as an RFC 6901 pointer. A segment outside
// [A-Za-z0-9._-] or over MaxIDBytes is written "?", so no byte of a hostile
// key reaches a flag; none of the kept characters needs RFC 6901 escaping.
func pointer(segs []string) string {
	out := make([]byte, 0, 64)
	for _, s := range segs {
		out = append(out, '/')
		if safeSegment(s) {
			out = append(out, s...)
		} else {
			out = append(out, '?')
		}
	}
	return string(out)
}

func safeSegment(s string) bool {
	if len(s) > MaxIDBytes {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '.' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// appendJCS appends the RFC 8785 form of n to dst: no whitespace, object keys
// in UTF-16 code unit order, strings escaped as section 3.2.2.2 fixes, integers
// as decimal digits (the only numbers a node holds, within 2^53-1, so the
// ECMAScript form is the digits). Cost: O(n log k) for n bytes and k keys in
// the widest object.
func appendJCS(dst []byte, n node) []byte {
	switch n.kind {
	case kObject:
		dst = append(dst, '{')
		for i, j := range sortedKeys(n.keys) {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = appendJCSString(dst, n.keys[j])
			dst = append(dst, ':')
			dst = appendJCS(dst, n.vals[j])
		}
		return append(dst, '}')
	case kArray:
		dst = append(dst, '[')
		for i, v := range n.vals {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = appendJCS(dst, v)
		}
		return append(dst, ']')
	case kString:
		return appendJCSString(dst, n.str)
	case kInt:
		return strconv.AppendInt(dst, n.num, 10)
	case kBool:
		return strconv.AppendBool(dst, n.flag)
	}
	return append(dst, "null"...)
}

// sortedKeys returns the indexes of keys ordered by UTF-16 code units. Code
// point order differs for runes above U+FFFF, whose surrogates sort below
// U+E000 to U+FFFF.
func sortedKeys(keys []string) []int {
	units := make([][]uint16, len(keys))
	idx := make([]int, len(keys))
	for i, k := range keys {
		units[i], idx[i] = utf16.Encode([]rune(k)), i
	}
	slices.SortFunc(idx, func(a, b int) int { return slices.Compare(units[a], units[b]) })
	return idx
}

// appendJCSString appends s quoted: the quote, the backslash and C0 controls
// are escaped (short forms where JSON has one, \u00xx in lower case otherwise)
// and every other rune is written as itself, DEL, U+2028 and "<>&" included.
func appendJCSString(dst []byte, s string) []byte {
	const hexDigits = "0123456789abcdef"
	dst = append(dst, '"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"' || c == '\\':
			dst = append(dst, '\\', c)
		case c == '\b':
			dst = append(dst, '\\', 'b')
		case c == '\f':
			dst = append(dst, '\\', 'f')
		case c == '\n':
			dst = append(dst, '\\', 'n')
		case c == '\r':
			dst = append(dst, '\\', 'r')
		case c == '\t':
			dst = append(dst, '\\', 't')
		case c < 0x20:
			dst = append(dst, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
		default:
			dst = append(dst, c)
		}
	}
	return append(dst, '"')
}
