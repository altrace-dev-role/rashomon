package jsonscan

import "io"

// The input window: refills, lookahead and errors at an input offset.

func (s *streamer) syntax(msg string) error {
	return &SyntaxError{Offset: s.base + int64(s.pos), Msg: msg}
}

// fill flushes consumed bytes to open taps, shifts the unread bytes to the
// front of the window and reads at least one more byte. It reports whether
// anything is left to read.
func (s *streamer) fill() bool {
	if s.eof {
		return s.pos < s.end
	}
	s.flushTaps(s.pos)
	var n int
	if len(s.win) < windowBytes && s.read > 0 {
		// The window starts at 4 KiB, so a small payload costs little,
		// and grows to 64 KiB once a payload has outrun it.
		w := make([]byte, min(4*len(s.win), windowBytes))
		n = copy(w, s.win[s.pos:s.end])
		s.win = w
	} else {
		n = copy(s.win, s.win[s.pos:s.end])
	}
	s.base += int64(s.pos)
	s.pos, s.end, s.tapFrom = 0, n, 0
	for zero := 0; s.end < len(s.win); {
		m, err := s.r.Read(s.win[s.end:])
		s.end += m
		s.read += int64(m)
		if err != nil {
			s.eof = true
			if err != io.EOF {
				s.rerr = err
			}
			break
		}
		if m > 0 {
			break
		}
		if zero++; zero >= maxZeroReads {
			s.eof, s.rerr = true, io.ErrNoProgress
			break
		}
	}
	return s.pos < s.end
}

// ensure makes k bytes readable at pos unless the input ends first.
func (s *streamer) ensure(k int) {
	for s.end-s.pos < k && !s.eof {
		s.fill()
	}
}

func (s *streamer) peek() (byte, bool) {
	if s.pos >= s.end && !s.fill() {
		return 0, false
	}
	return s.win[s.pos], true
}

func (s *streamer) endErr() error {
	if s.rerr != nil {
		return s.rerr
	}
	return s.syntax("unexpected end of input")
}

// skipWS consumes whitespace and reports whether there was any.
func (s *streamer) skipWS() bool {
	any := false
	for {
		c, ok := s.peek()
		if !ok || !isWS(c) {
			return any
		}
		s.pos++
		any = true
	}
}
