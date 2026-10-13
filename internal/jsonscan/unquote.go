package jsonscan

import (
	"bytes"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

func indexByte(b []byte, c byte) int { return bytes.IndexByte(b, c) }

// segment returns the decoded bytes of a string body starting at r, and the
// index after them: a run of bytes that decode to themselves (a subslice of
// body, valid UTF-8, never holding a newline), or one escape or one invalid
// byte decoded into tmp. Decoding follows encoding/json: invalid UTF-8 and
// unpaired surrogates become U+FFFD. body is the bytes between the quotes.
func segment(body []byte, r int, tmp *[utf8.UTFMax]byte) ([]byte, int) {
	if body[r] == '\\' {
		return escape(body, r, tmp)
	}
	j := r
	for j < len(body) {
		c := body[j]
		if c == '\\' {
			break
		}
		if c < utf8.RuneSelf {
			j++
			continue
		}
		rr, size := utf8.DecodeRune(body[j:])
		if rr == utf8.RuneError && size == 1 {
			break
		}
		j += size
	}
	if j > r {
		return body[r:j], j
	}
	n := utf8.EncodeRune(tmp[:], utf8.RuneError)
	return tmp[:n], r + 1
}

// escape decodes the escape sequence at body[r].
func escape(body []byte, r int, tmp *[utf8.UTFMax]byte) ([]byte, int) {
	if r+1 >= len(body) {
		n := utf8.EncodeRune(tmp[:], utf8.RuneError)
		return tmp[:n], len(body)
	}
	switch c := body[r+1]; c {
	case 'b':
		tmp[0] = '\b'
	case 'f':
		tmp[0] = '\f'
	case 'n':
		tmp[0] = '\n'
	case 'r':
		tmp[0] = '\r'
	case 't':
		tmp[0] = '\t'
	case 'u':
		return escapeU(body, r, tmp)
	default:
		tmp[0] = c
	}
	return tmp[:1], r + 2
}

func escapeU(body []byte, r int, tmp *[utf8.UTFMax]byte) ([]byte, int) {
	v1, ok := hex4(body, r+2)
	if !ok {
		n := utf8.EncodeRune(tmp[:], utf8.RuneError)
		return tmp[:n], len(body)
	}
	r += 6
	rr := rune(v1)
	if utf16.IsSurrogate(rr) {
		rr = utf8.RuneError
		if r+6 <= len(body) && body[r] == '\\' && body[r+1] == 'u' {
			if v2, ok := hex4(body, r+2); ok {
				if d := utf16.DecodeRune(rune(v1), rune(v2)); d != utf8.RuneError {
					rr = d
					r += 6
				}
			}
		}
	}
	n := utf8.EncodeRune(tmp[:], rr)
	return tmp[:n], r
}

func hex4(b []byte, i int) (uint16, bool) {
	if i+4 > len(b) {
		return 0, false
	}
	var v uint16
	for _, c := range b[i : i+4] {
		switch {
		case c >= '0' && c <= '9':
			c -= '0'
		case c >= 'a' && c <= 'f':
			c = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			c = c - 'A' + 10
		default:
			return 0, false
		}
		v = v<<4 | uint16(c)
	}
	return v, true
}

// runeCut returns the largest k <= n at which a valid UTF-8 run may be cut
// without splitting a character.
func runeCut(run []byte, n int) int {
	if n >= len(run) {
		return len(run)
	}
	for i := 1; i < utf8.UTFMax && n > 0 && !utf8.RuneStart(run[n]); i++ {
		n--
	}
	return n
}

// unescapeHead appends at most max decoded bytes of body to dst, never
// splitting a character, and reports whether the string was longer.
func unescapeHead(body, dst []byte, max int) ([]byte, bool) {
	var tmp [utf8.UTFMax]byte
	for r := 0; r < len(body); {
		seg, next := segment(body, r, &tmp)
		room := max - len(dst)
		if len(seg) > room {
			return append(dst, seg[:runeCut(seg, room)]...), true
		}
		dst = append(dst, seg...)
		r = next
	}
	return dst, false
}

// body returns the bytes between a string span's quotes.
func body(buf []byte, s Span) ([]byte, error) {
	if s.Kind != String {
		return nil, ErrNotString
	}
	if !s.inside(buf) || s.Len < 2 {
		return nil, ErrSpan
	}
	return buf[s.Off+1 : s.Off+s.Len-1], nil
}

// Head unescapes at most max bytes from the start of a string span into dst,
// never splitting a character. It allocates nothing when cap(dst) >= max.
// truncated reports whether the string was longer than max.
func Head(buf []byte, s Span, max int, dst []byte) (out []byte, truncated bool, err error) {
	b, err := body(buf, s)
	if err != nil {
		return dst[:0], false, err
	}
	out, truncated = unescapeHead(b, dst[:0], max)
	return out, truncated, nil
}

// DecodeString decodes a whole string span. It allocates once, a buffer of the
// span's encoded length, which no valid string's decoding exceeds, and hands
// that buffer back as the string: Head into a buffer grown from nothing and
// then string() copied a command twice after growing the first copy a
// quarter at a time, and a hook decodes its command once per reader of it.
// Only invalid UTF-8, each byte of which decodes to three, grows the buffer.
func DecodeString(buf []byte, s Span) (string, error) {
	b, err := body(buf, s)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	out.Grow(len(b))
	var tmp [utf8.UTFMax]byte
	for r := 0; r < len(b); {
		seg, next := segment(b, r, &tmp)
		out.Write(seg)
		r = next
	}
	return out.String(), nil
}

// Tail keeps the last max decoded bytes of a string span through a ring
// buffer of size max while scanning forward, because a JSON string cannot be
// decoded backwards. ring must hold max bytes; out aliases it and starts on a
// character boundary. truncated reports whether the string was longer.
func Tail(buf []byte, s Span, max int, ring []byte) (out []byte, truncated bool, err error) {
	b, err := body(buf, s)
	if err != nil {
		return nil, false, err
	}
	if max <= 0 {
		return ring[:0], len(b) > 0, nil
	}
	if len(ring) < max {
		return nil, false, ErrShortOut
	}
	t := tailRing{buf: ring[:0], size: max}
	var tmp [utf8.UTFMax]byte
	for r := 0; r < len(b); {
		seg, next := segment(b, r, &tmp)
		t.write(seg)
		r = next
	}
	out, truncated = t.bytes()
	return out, truncated, nil
}

// tailRing keeps the last size bytes written to it. buf fills by append up
// to size, so a short value costs only its own bytes, and then wraps.
type tailRing struct {
	buf   []byte
	size  int
	pos   int
	total int64
}

func (t *tailRing) reset() { t.buf, t.pos, t.total = t.buf[:0], 0, 0 }

func (t *tailRing) write(p []byte) {
	t.total += int64(len(p))
	size := t.size
	if room := size - len(t.buf); room > 0 {
		k := min(len(p), room)
		t.buf = append(grow(t.buf, len(t.buf)+k, size), p[:k]...)
		if p = p[k:]; len(p) == 0 {
			return
		}
	}
	if len(p) >= size {
		copy(t.buf, p[len(p)-size:])
		t.pos = 0
		return
	}
	n := copy(t.buf[t.pos:], p)
	if n < len(p) {
		t.pos = copy(t.buf, p[n:])
		return
	}
	t.pos += n
	if t.pos == size {
		t.pos = 0
	}
}

// bytes returns the kept bytes in order, rotating the ring in place, and
// drops a leading partial character.
func (t *tailRing) bytes() ([]byte, bool) {
	size := int64(t.size)
	if t.total <= size {
		return t.buf[:t.total], false
	}
	reverse(t.buf[:t.pos])
	reverse(t.buf[t.pos:])
	reverse(t.buf)
	out := t.buf
	for i := 1; i < utf8.UTFMax && len(out) > 0 && !utf8.RuneStart(out[0]); i++ {
		out = out[1:]
	}
	t.pos = 0
	return out, true
}

// grow returns b with room for need bytes, doubling its capacity up to
// limit, so a buffer grown to n bytes has cost at most 2n.
func grow(b []byte, need, limit int) []byte {
	if need <= cap(b) {
		return b
	}
	c := max(2*cap(b), need, 512)
	if c > limit {
		c = max(limit, need)
	}
	nb := make([]byte, len(b), c)
	copy(nb, b)
	return nb
}

func reverse(b []byte) {
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
}
