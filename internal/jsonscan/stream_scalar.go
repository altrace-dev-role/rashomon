package jsonscan

import "strconv"

// Numbers and literals, validated per the JSON grammar as they stream.

// literal consumes a literal and the delimiter check after it.
func (s *streamer) literal(lit string, t uint64) error {
	s.ensure(len(lit))
	if s.end-s.pos < len(lit) || string(s.win[s.pos:s.pos+len(lit)]) != lit {
		return s.syntax("invalid literal, expected " + lit)
	}
	if t != 0 {
		s.startValue(t, false)
	}
	s.pos += len(lit)
	if err := s.delimited(); err != nil {
		return err
	}
	s.endScalar(t)
	return nil
}

// endScalar finishes a raw tap opened for a scalar.
func (s *streamer) endScalar(t uint64) {
	if t&s.captured() == 0 {
		return
	}
	s.flushTaps(s.pos)
	tp := s.taps[len(s.taps)-1]
	s.taps = s.taps[:len(s.taps)-1]
	s.finishCaps(tp.mask)
}

// delimited checks that a number or literal is followed by a byte that may
// end it.
func (s *streamer) delimited() error {
	c, ok := s.peek()
	if !ok || isWS(c) {
		return nil
	}
	if s.depth > 0 && (c == ',' || c == '}' || c == ']') {
		return nil
	}
	return s.syntax("invalid character " + strconv.QuoteRune(rune(c)) + " after value")
}

// number consumes a number per the JSON grammar.
func (s *streamer) number(t uint64) error {
	if t != 0 {
		s.startValue(t, false)
	}
	if c, _ := s.peek(); c == '-' {
		s.pos++
	}
	c, ok := s.peek()
	switch {
	case !ok:
		return s.endErr()
	case c == '0':
		s.pos++
	case c >= '1' && c <= '9':
		s.digits()
	default:
		return s.syntax("invalid character in numeric literal")
	}
	if c, ok := s.peek(); ok && c == '.' {
		s.pos++
		if n := s.digits(); n == 0 {
			return s.digitErr()
		}
	}
	if c, ok := s.peek(); ok && (c == 'e' || c == 'E') {
		s.pos++
		if c, ok := s.peek(); ok && (c == '+' || c == '-') {
			s.pos++
		}
		if n := s.digits(); n == 0 {
			return s.digitErr()
		}
	}
	if err := s.delimited(); err != nil {
		return err
	}
	s.endScalar(t)
	return nil
}

func (s *streamer) digitErr() error {
	if _, ok := s.peek(); !ok {
		return s.endErr()
	}
	return s.syntax("invalid character in numeric literal")
}

func (s *streamer) digits() int {
	n := 0
	for {
		c, ok := s.peek()
		if !ok || c < '0' || c > '9' {
			return n
		}
		s.pos++
		n++
	}
}
