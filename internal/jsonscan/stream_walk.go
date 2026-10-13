package jsonscan

// Containers: the nesting stack, path matching and the raw taps.

func (s *streamer) push(object bool) {
	s.depth++
	w, b := s.depth/64, uint(s.depth%64)
	if object {
		s.objects[w] |= 1 << b
	} else {
		s.objects[w] &^= 1 << b
	}
	if s.depth < len(s.masks) {
		s.masks[s.depth] = s.cont
	}
}

func (s *streamer) isObject() bool {
	return s.objects[s.depth/64]&(1<<uint(s.depth%64)) != 0
}

func (s *streamer) mask() uint64 {
	if s.depth < len(s.masks) {
		return s.masks[s.depth]
	}
	return 0
}

// firstOrClose handles the byte after an opening bracket.
func (s *streamer) firstOrClose() (bool, error) {
	s.skipWS()
	c, ok := s.peek()
	if !ok {
		return false, s.endErr()
	}
	if (s.isObject() && c == '}') || (!s.isObject() && c == ']') {
		s.pos++
		s.pop()
		return true, nil
	}
	return false, s.member()
}

// next handles the byte after a value inside a container: a comma positions
// the next member and reports true; a closing bracket pops.
func (s *streamer) next() (bool, error) {
	s.skipWS()
	c, ok := s.peek()
	if !ok {
		return false, s.endErr()
	}
	obj := s.isObject()
	switch {
	case c == ',':
		s.pos++
		s.skipWS()
		return true, s.member()
	case (obj && c == '}') || (!obj && c == ']'):
		s.pos++
		s.pop()
		return false, nil
	}
	if obj {
		return false, s.syntax("expected ',' or '}' after object value")
	}
	return false, s.syntax("expected ',' or ']' after array element")
}

// member positions the next value of the open container and sets the masks
// of the paths it is wanted by (t) and continues (cont).
func (s *streamer) member() error {
	m := s.mask()
	d := s.depth - 1
	if !s.isObject() {
		s.match(m, d, "[]", false)
		return nil
	}
	c, ok := s.peek()
	if !ok {
		return s.endErr()
	}
	if c != '"' {
		return s.syntax("expected string for object key")
	}
	s.key, s.keyLong = s.key[:0], false
	if err := s.str(0, m != 0); err != nil {
		return err
	}
	s.skipWS()
	c, ok = s.peek()
	if !ok {
		return s.endErr()
	}
	if c != ':' {
		return s.syntax("expected ':' after object key")
	}
	s.pos++
	s.skipWS()
	s.match(m, d, string(s.key), s.keyLong)
	return nil
}

func (s *streamer) match(m uint64, d int, elem string, long bool) {
	s.t, s.cont = 0, 0
	if m == 0 || long {
		return
	}
	for i, p := range s.paths {
		if m&(1<<i) == 0 || d >= len(p.Elems) || p.Elems[d] != elem {
			continue
		}
		if d == len(p.Elems)-1 {
			s.t |= 1 << i
		} else {
			s.cont |= 1 << i
		}
	}
}

// pop closes the container at the current depth and finishes its taps.
func (s *streamer) pop() {
	for len(s.taps) > 0 && s.taps[len(s.taps)-1].depth == s.depth {
		t := s.taps[len(s.taps)-1]
		s.flushTaps(s.pos)
		s.taps = s.taps[:len(s.taps)-1]
		s.finishCaps(t.mask)
	}
	s.depth--
}

// flushTaps passes the bytes consumed since the last flush to open taps.
func (s *streamer) flushTaps(upto int) {
	if upto > s.tapFrom {
		seg := s.win[s.tapFrom:upto]
		for _, t := range s.taps {
			s.feed(t.mask, seg, false)
		}
	}
	s.tapFrom = upto
}

func (s *streamer) feed(m uint64, seg []byte, text bool) {
	for i := range s.caps {
		if m&(1<<i) != 0 {
			s.caps[i].write(seg, text)
		}
	}
}

func (s *streamer) startCaps(m uint64, raw bool) {
	for i := range s.caps {
		if m&(1<<i) != 0 {
			s.caps[i].start(!raw)
		}
	}
}

func (s *streamer) finishCaps(m uint64) {
	for i := range s.caps {
		if m&(1<<i) != 0 && !s.stopped {
			s.caps[i].finish()
		}
	}
}
