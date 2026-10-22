package jsonscan

import (
	"fmt"
	"unicode/utf16"
	"unicode/utf8"
)

// str consumes the string at pos. Its decoded bytes go to the captures in t
// and, for a key, to s.key up to the longest path element.
func (s *streamer) str(t uint64, asKey bool) error {
	s.pos++
	t &= s.captured()
	var tmp [utf8.UTFMax]byte
	for {
		if s.pos >= s.end && !s.fill() {
			return s.endErr()
		}
		c := s.win[s.pos]
		switch {
		case c == '"':
			s.pos++
			return nil
		case c == '\\':
			s.ensure(12)
			seg, n, err := s.escapeAt(&tmp)
			if err != nil {
				return err
			}
			s.pos += n
			s.emit(t, asKey, seg)
		case c < 0x20:
			return s.syntax("invalid character in string literal")
		default:
			if j := s.textRun(); j > s.pos {
				s.emit(t, asKey, s.win[s.pos:j])
				s.pos = j
				continue
			}
			s.ensure(utf8.UTFMax)
			if rr, size := utf8.DecodeRune(s.win[s.pos:s.end]); rr != utf8.RuneError || size > 1 {
				s.emit(t, asKey, s.win[s.pos:s.pos+size])
				s.pos += size
				continue
			}
			n := utf8.EncodeRune(tmp[:], utf8.RuneError)
			s.emit(t, asKey, tmp[:n])
			s.pos++
		}
	}
}

// textRun returns the end of the run at pos of bytes that decode to
// themselves: printable ASCII other than quote and backslash, and whole
// valid UTF-8 characters. A character cut by the window end stops it.
func (s *streamer) textRun() int {
	j := s.pos
	for j < s.end {
		c := s.win[j]
		if c < utf8.RuneSelf {
			if c < 0x20 || c == '"' || c == '\\' {
				break
			}
			j++
			continue
		}
		if s.end-j < utf8.UTFMax && !s.eof {
			break
		}
		rr, size := utf8.DecodeRune(s.win[j:s.end])
		if rr == utf8.RuneError && size == 1 {
			break
		}
		j += size
	}
	return j
}

// escapeAt validates and decodes the escape at pos.
func (s *streamer) escapeAt(tmp *[utf8.UTFMax]byte) ([]byte, int, error) {
	b := s.win[s.pos:s.end]
	if len(b) < 2 {
		return nil, 0, s.endErr()
	}
	switch c := b[1]; c {
	case '"', '\\', '/':
		tmp[0] = c
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
		v1, ok := hex4(b, 2)
		if !ok {
			if len(b) < 6 && s.eof {
				return nil, 0, s.endErr()
			}
			return nil, 0, s.syntax("invalid escape sequence in string")
		}
		rr, n := rune(v1), 6
		if utf16.IsSurrogate(rr) {
			rr = utf8.RuneError
			if len(b) >= 12 && b[6] == '\\' && b[7] == 'u' {
				if v2, ok := hex4(b, 8); ok {
					if d := utf16.DecodeRune(rune(v1), rune(v2)); d != utf8.RuneError {
						rr, n = d, 12
					}
				}
			}
		}
		k := utf8.EncodeRune(tmp[:], rr)
		return tmp[:k], n, nil
	default:
		return nil, 0, s.syntax("invalid escape sequence in string")
	}
	return tmp[:1], 2, nil
}

func (s *streamer) emit(t uint64, asKey bool, seg []byte) {
	if asKey && !s.keyLong {
		if len(s.key)+len(seg) > s.longest {
			s.keyLong = true
		} else {
			s.key = append(s.key, seg...)
		}
	}
	if t != 0 {
		s.feed(t, seg, true)
	}
}

// capture holds one path's buffers. They grow by append to at most Max (the
// Lines work buffer is Max from its first use) and are reused for every
// later value, so a short value costs its own bytes and no value allocates
// once the buffers have grown.
type capture struct {
	s     *streamer
	idx   int
	mode  CaptureMode
	max   int
	head  []byte
	full  bool
	ring  tailRing
	lines lineWriter
	total int64
	text  bool
}

func (c *capture) init(s *streamer, idx int, p Path) error {
	c.s, c.idx, c.mode, c.max = s, idx, p.Mode, p.Max
	c.ring.size = p.Max
	switch p.Mode {
	case CaptureHead, CaptureTail, CaptureHeadTail, CapturePresence:
	case CaptureLines:
		if p.Max < utf8.UTFMax {
			return fmt.Errorf("%w: a CaptureLines path's Max (%d) must be at least %d, to hold one rune", ErrPaths, p.Max, utf8.UTFMax)
		}
		c.lines.fn = func(line []byte, more bool) bool { return c.deliver(line, more) }
	default:
		return fmt.Errorf("%w: capture mode %d is not set or not a mode Stream knows", ErrPaths, p.Mode)
	}
	return nil
}

func (c *capture) start(text bool) {
	c.head, c.full, c.total, c.text = c.head[:0], false, 0, text
	c.ring.reset()
	c.lines.n, c.lines.split = 0, false
}

func (c *capture) cut(seg []byte, room int) int {
	if c.text {
		return runeCut(seg, room)
	}
	return room
}

func (c *capture) write(seg []byte, text bool) {
	c.total += int64(len(seg))
	switch c.mode {
	case CaptureHead:
		c.fillHead(seg)
	case CaptureTail:
		c.ring.write(seg)
	case CaptureHeadTail:
		if rest := c.fillHead(seg); len(rest) > 0 {
			c.ring.write(rest)
		}
	case CaptureLines:
		if c.lines.work == nil {
			c.lines.work = make([]byte, c.max)
		}
		c.lines.feed(seg, c.text)
	}
}

// fillHead copies what fits into the head and returns the rest.
func (c *capture) fillHead(seg []byte) []byte {
	if c.full {
		return seg
	}
	room := c.max - len(c.head)
	k := len(seg)
	if k > room {
		k = c.cut(seg, room)
		c.full = true
	}
	c.head = append(grow(c.head, len(c.head)+k, c.max), seg[:k]...)
	return seg[k:]
}

func (c *capture) deliver(v []byte, truncated bool) bool {
	if c.s.stopped {
		return false
	}
	if !c.s.fn(c.idx, v, truncated) {
		c.s.stopped = true
	}
	return !c.s.stopped
}

func (c *capture) finish() {
	switch c.mode {
	case CaptureHead:
		c.deliver(c.head, c.total > int64(len(c.head)))
	case CaptureTail:
		out, truncated := c.ring.bytes()
		c.deliver(out, truncated)
	case CaptureHeadTail:
		c.finishHeadTail()
	case CaptureLines:
		c.lines.finish()
	}
}

// finishHeadTail passes a value that fits in 2*Max as one piece, else the
// head and then the tail.
func (c *capture) finishHeadTail() {
	if c.ring.total <= int64(c.max) {
		c.head = append(grow(c.head, len(c.head)+len(c.ring.buf), 2*c.max), c.ring.buf...)
		c.deliver(c.head, false)
		return
	}
	if !c.deliver(c.head, true) {
		return
	}
	// The bool says whether the ring wrapped, and it has: total is over max.
	out, _ := c.ring.bytes()
	c.deliver(out, true)
}
