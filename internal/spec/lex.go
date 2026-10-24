package spec

import (
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// The document tree and the strict JSON reader that builds it (RFC 8259 with
// the I-JSON restrictions RFC 8785 assumes: no duplicate keys, no unpaired
// surrogates, integers only). It exists instead of encoding/json because the
// standard decoder matches keys case-insensitively, keeps the last of a
// duplicate, turns null into a zero value and replaces bad UTF-8, and each of
// those would make two different files one spec or one file two digests.

// nodeKind is the JSON type of a node.
type nodeKind uint8

const (
	kObject nodeKind = iota + 1
	kArray
	kString
	kInt
	kBool
	kNull
)

// node is one JSON value. Invariant: an object's keys are distinct and keep
// file order, with vals parallel to keys; an array's elements are vals; an
// integer lies within +-(2^53-1) so its canonical form is its decimal digits.
type node struct {
	kind nodeKind
	str  string
	num  int64
	flag bool
	keys []string
	vals []node
}

// maxSafeInt is 2^53-1, the largest integer RFC 8785's number form (the
// ECMAScript double) holds exactly.
const maxSafeInt = 1<<53 - 1

// lexer reads one document. It stops at the first error; err is then set and
// every later call returns the zero node. Work and memory are linear in the
// input, which Parse has already capped at MaxBytes, and the tree is capped
// by MaxNodes and MaxDepth.
type lexer struct {
	data  []byte
	pos   int
	nodes int
	path  []string
	err   *Error
}

// lex reads data, which must be valid UTF-8, into a tree. On failure the
// Error carries the reason and the pointer of the value being read.
func lex(data []byte) (node, *Error) {
	l := &lexer{data: data}
	n := l.value(1)
	if l.err == nil {
		l.skipSpace()
		if l.pos != len(l.data) {
			l.fail(ReasonSyntax, 0)
		}
	}
	if l.err != nil {
		return node{}, l.err
	}
	return n, nil
}

func (l *lexer) fail(r Reason, limit int) {
	if l.err == nil {
		l.err = &Error{Pointer: pointer(l.path), Reason: r, Limit: limit}
	}
}

func (l *lexer) skipSpace() {
	for l.pos < len(l.data) {
		switch l.data[l.pos] {
		case ' ', '\t', '\n', '\r':
			l.pos++
		default:
			return
		}
	}
}

// value reads one value; depth is the nesting level it would open (the
// file's own object is 1).
func (l *lexer) value(depth int) node {
	l.skipSpace()
	if l.pos >= len(l.data) {
		l.fail(ReasonSyntax, 0)
		return node{}
	}
	if l.nodes++; l.nodes > MaxNodes {
		l.fail(ReasonTooManyValues, MaxNodes)
		return node{}
	}
	switch c := l.data[l.pos]; {
	case c == '{':
		return l.object(depth)
	case c == '[':
		return l.array(depth)
	case c == '"':
		s, ok := l.str()
		if !ok {
			return node{}
		}
		return node{kind: kString, str: s}
	case c == '-' || '0' <= c && c <= '9':
		return l.number()
	case l.literal("true"):
		return node{kind: kBool, flag: true}
	case l.literal("false"):
		return node{kind: kBool}
	case l.literal("null"):
		return node{kind: kNull}
	}
	l.fail(ReasonSyntax, 0)
	return node{}
}

func (l *lexer) literal(word string) bool {
	if len(l.data)-l.pos >= len(word) && string(l.data[l.pos:l.pos+len(word)]) == word {
		l.pos += len(word)
		return true
	}
	return false
}

func (l *lexer) object(depth int) node {
	if depth > MaxDepth {
		l.fail(ReasonTooDeep, MaxDepth)
		return node{}
	}
	l.pos++ // {
	n := node{kind: kObject}
	seen := map[string]struct{}{}
	l.skipSpace()
	if l.pos < len(l.data) && l.data[l.pos] == '}' {
		l.pos++
		return n
	}
	for {
		l.skipSpace()
		if l.pos >= len(l.data) || l.data[l.pos] != '"' {
			l.fail(ReasonSyntax, 0)
			return node{}
		}
		key, ok := l.str()
		if !ok {
			return node{}
		}
		l.path = append(l.path, key)
		if _, dup := seen[key]; dup {
			l.fail(ReasonDuplicateKey, 0)
			return node{}
		}
		seen[key] = struct{}{}
		l.skipSpace()
		if l.pos >= len(l.data) || l.data[l.pos] != ':' {
			l.fail(ReasonSyntax, 0)
			return node{}
		}
		l.pos++
		v := l.value(depth + 1)
		if l.err != nil {
			return node{}
		}
		l.path = l.path[:len(l.path)-1]
		n.keys = append(n.keys, key)
		n.vals = append(n.vals, v)
		l.skipSpace()
		if l.pos >= len(l.data) {
			l.fail(ReasonSyntax, 0)
			return node{}
		}
		switch l.data[l.pos] {
		case ',':
			l.pos++
		case '}':
			l.pos++
			return n
		default:
			l.fail(ReasonSyntax, 0)
			return node{}
		}
	}
}

func (l *lexer) array(depth int) node {
	if depth > MaxDepth {
		l.fail(ReasonTooDeep, MaxDepth)
		return node{}
	}
	l.pos++ // [
	n := node{kind: kArray}
	l.skipSpace()
	if l.pos < len(l.data) && l.data[l.pos] == ']' {
		l.pos++
		return n
	}
	for i := 0; ; i++ {
		l.path = append(l.path, strconv.Itoa(i))
		v := l.value(depth + 1)
		if l.err != nil {
			return node{}
		}
		l.path = l.path[:len(l.path)-1]
		n.vals = append(n.vals, v)
		l.skipSpace()
		if l.pos >= len(l.data) {
			l.fail(ReasonSyntax, 0)
			return node{}
		}
		switch l.data[l.pos] {
		case ',':
			l.pos++
		case ']':
			l.pos++
			return n
		default:
			l.fail(ReasonSyntax, 0)
			return node{}
		}
	}
}

// number reads an integer: -?(0|[1-9][0-9]*) with no fraction or exponent,
// within +-(2^53-1). A fraction or exponent is refused even when its value
// is whole ("1.0"): the archive says integers only.
func (l *lexer) number() node {
	start := l.pos
	if l.data[l.pos] == '-' {
		l.pos++
	}
	switch {
	case l.pos >= len(l.data):
		l.fail(ReasonSyntax, 0)
		return node{}
	case l.data[l.pos] == '0':
		l.pos++
	case '1' <= l.data[l.pos] && l.data[l.pos] <= '9':
		for l.pos < len(l.data) && '0' <= l.data[l.pos] && l.data[l.pos] <= '9' {
			l.pos++
		}
	default:
		l.fail(ReasonSyntax, 0)
		return node{}
	}
	if l.pos < len(l.data) && (l.data[l.pos] == '.' || l.data[l.pos] == 'e' || l.data[l.pos] == 'E') {
		l.fail(ReasonNotInteger, 0)
		return node{}
	}
	v, err := strconv.ParseInt(string(l.data[start:l.pos]), 10, 64)
	if err != nil || v > maxSafeInt || v < -maxSafeInt {
		l.fail(ReasonIntegerRange, 0)
		return node{}
	}
	return node{kind: kInt, num: v}
}

// str reads a string starting at its opening quote and returns it decoded.
// An unpaired surrogate escape is refused: it has no UTF-8 form, and the
// standard decoder would silently write U+FFFD in its place.
func (l *lexer) str() (string, bool) {
	l.pos++ // opening quote
	var buf []byte
	for l.pos < len(l.data) {
		c := l.data[l.pos]
		switch {
		case c == '"':
			l.pos++
			return string(buf), true
		case c < 0x20:
			l.fail(ReasonSyntax, 0)
			return "", false
		case c != '\\':
			buf = append(buf, c)
			l.pos++
		default:
			if !l.escape(&buf) {
				return "", false
			}
		}
	}
	l.fail(ReasonSyntax, 0)
	return "", false
}

// escape reads one backslash escape at l.pos and appends what it means.
func (l *lexer) escape(buf *[]byte) bool {
	if l.pos+1 >= len(l.data) {
		l.fail(ReasonSyntax, 0)
		return false
	}
	switch c := l.data[l.pos+1]; c {
	case '"', '\\', '/':
		*buf = append(*buf, c)
	case 'b':
		*buf = append(*buf, '\b')
	case 'f':
		*buf = append(*buf, '\f')
	case 'n':
		*buf = append(*buf, '\n')
	case 'r':
		*buf = append(*buf, '\r')
	case 't':
		*buf = append(*buf, '\t')
	case 'u':
		r, ok := l.hex4(l.pos + 2)
		if !ok {
			l.fail(ReasonSyntax, 0)
			return false
		}
		l.pos += 6
		if utf16.IsSurrogate(r) {
			low, ok := rune(0), false
			if r < 0xDC00 && l.pos+1 < len(l.data) && l.data[l.pos] == '\\' && l.data[l.pos+1] == 'u' {
				low, ok = l.hex4(l.pos + 2)
			}
			if !ok || low < 0xDC00 || low > 0xDFFF {
				l.fail(ReasonBadEscape, 0)
				return false
			}
			r = utf16.DecodeRune(r, low)
			l.pos += 6
		}
		*buf = utf8.AppendRune(*buf, r)
		return true
	default:
		l.fail(ReasonSyntax, 0)
		return false
	}
	l.pos += 2
	return true
}

// hex4 reads four hex digits at i.
func (l *lexer) hex4(i int) (rune, bool) {
	if i+4 > len(l.data) {
		return 0, false
	}
	var r rune
	for _, c := range l.data[i : i+4] {
		switch {
		case '0' <= c && c <= '9':
			r = r<<4 | rune(c-'0')
		case 'a' <= c && c <= 'f':
			r = r<<4 | rune(c-'a'+10)
		case 'A' <= c && c <= 'F':
			r = r<<4 | rune(c-'A'+10)
		default:
			return 0, false
		}
	}
	return r, true
}

// pointer renders segments as an RFC 6901 pointer. A segment that is not
// short and made of [A-Za-z0-9._-] is "?", so the pointer never carries
// text a hostile file chose, and never needs escaping.
func pointer(segs []string) string {
	var b []byte
	for _, s := range segs {
		b = append(b, '/')
		if safeSegment(s) {
			b = append(b, s...)
		} else {
			b = append(b, '?')
		}
	}
	return string(b)
}

func safeSegment(s string) bool {
	if len(s) == 0 || len(s) > MaxIDBytes {
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
