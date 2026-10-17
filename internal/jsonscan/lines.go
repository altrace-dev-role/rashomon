package jsonscan

import "unicode/utf8"

// Lines unescapes a string span into lines through one work buffer of
// len(work) bytes (64 KiB in every caller), calling fn once per line without
// its newline. A line longer than the buffer is passed in pieces with
// more=true, cut on character boundaries; the last piece has more=false. A
// trailing newline ends the last line and adds no empty one. fn returning
// false stops the scan. It never builds the whole string.
func Lines(buf []byte, s Span, work []byte, fn func(line []byte, more bool) bool) error {
	b, err := body(buf, s)
	if err != nil {
		return err
	}
	if len(work) < utf8.UTFMax {
		return ErrShortOut
	}
	l := lineWriter{work: work, fn: fn}
	var tmp [utf8.UTFMax]byte
	for r := 0; r < len(b) && !l.stopped; {
		seg, next := segment(b, r, &tmp)
		r = next
		l.feed(seg, true)
	}
	l.finish()
	return nil
}

// lineWriter accumulates one line in work and flushes it in pieces.
type lineWriter struct {
	work    []byte
	n       int
	split   bool // a piece of the current line has been passed already
	stopped bool
	fn      func(line []byte, more bool) bool
}

// feed splits seg at newlines. text says seg is decoded text, cut on
// character boundaries; raw JSON is cut on byte boundaries.
func (l *lineWriter) feed(seg []byte, text bool) {
	for len(seg) > 0 && !l.stopped {
		nl := indexByte(seg, '\n')
		if nl < 0 {
			l.write(seg, text)
			return
		}
		l.write(seg[:nl], text)
		if !l.stopped {
			l.endLine()
		}
		seg = seg[nl+1:]
	}
}

// finish ends a line left open at the end of the value.
func (l *lineWriter) finish() {
	if !l.stopped && (l.n > 0 || l.split) {
		l.endLine()
	}
}

func (l *lineWriter) write(seg []byte, text bool) {
	for len(seg) > 0 && !l.stopped {
		room := len(l.work) - l.n
		k := len(seg)
		if k > room {
			k = room
			if text {
				k = runeCut(seg, room)
			}
		}
		l.n += copy(l.work[l.n:], seg[:k])
		seg = seg[k:]
		if len(seg) > 0 {
			l.piece()
		}
	}
}

func (l *lineWriter) piece() {
	l.split = true
	l.stopped = !l.fn(l.work[:l.n], true)
	l.n = 0
}

func (l *lineWriter) endLine() {
	l.stopped = !l.fn(l.work[:l.n], false)
	l.n, l.split = 0, false
}
