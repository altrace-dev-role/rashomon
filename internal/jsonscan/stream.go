package jsonscan

import (
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// CaptureMode says how much of a wanted value Stream copies out. The zero
// mode is none: a Path whose Mode was never set is refused with ErrPaths,
// rather than read as CaptureHead and copied out unasked.
type CaptureMode uint8

// The capture modes.
const (
	_ CaptureMode = iota // the zero value: no mode, refused
	// CaptureHead keeps the first Max bytes.
	CaptureHead
	// CaptureTail keeps the last Max bytes.
	CaptureTail
	// CaptureHeadTail keeps the first Max and the last Max bytes.
	CaptureHeadTail
	// CapturePresence keeps nothing; fn is told the value is there.
	CapturePresence
	// CaptureLines passes every line of the value in pieces of at most Max
	// bytes, as Lines does, so a whole value is scanned without being held.
	CaptureLines
)

// Path names values Stream copies out. Elems are object keys from the top
// level down; "[]" stands for every element of an array.
type Path struct {
	Elems []string // e.g. {"tool_calls", "[]", "tool_use_id"}
	Max   int
	Mode  CaptureMode
	// String makes any value at the path other than a string or null stop
	// the stream with ErrNotString, as decoding it into a string failed.
	String bool
	// Fold matches each object key as NewFoldKeySet does, as decoding into
	// a struct field matches its name; without it a key matches exactly.
	Fold bool
}

// elemIs reports whether an unescaped key or "[]" is the path's d-th element.
func (p *Path) elemIs(d int, elem string) bool {
	return p.Elems[d] == elem || (p.Fold && strings.EqualFold(p.Elems[d], elem))
}

// Stream bounds.
const (
	windowBytes  = 64 << 10
	firstWindow  = 4 << 10
	maxDepth     = 10000 // encoding/json's nesting limit
	maxPaths     = 64
	maxPathElems = 8
	maxZeroReads = 100
)

// ErrPaths reports a path list Stream cannot serve.
var ErrPaths = errors.New("jsonscan: too many paths, a path too long, or a capture of no bytes")

// SyntaxError is the first syntax error Stream met, at an input offset.
type SyntaxError struct {
	Offset int64
	Msg    string
}

func (e *SyntaxError) Error() string {
	return "jsonscan: " + e.Msg + " at byte " + strconv.FormatInt(e.Offset, 10)
}

// Stream walks one or more top-level JSON values read from r through a 64 KiB
// window, consecutive values separated by whitespace. It copies out only the
// values at the wanted paths, each capped at Path.Max bytes; everything else
// is skipped without being buffered. A string value is passed unescaped, any
// other value as its JSON text, and a null value is not passed at all.
//
// fn is called when a wanted value ends: once with the captured bytes, or for
// CaptureHeadTail of a value longer than 2*Max twice, the head and then the
// tail, both truncated. CapturePresence calls fn once with a nil value when
// the value starts; CaptureLines once per line or piece, truncated meaning
// more of that line follows. A key that repeats is passed each time, so the
// last call wins. value aliases Stream's buffers and is valid only during the
// call. fn returning false stops the stream with no error. On a syntax
// error fn may already have been called, CaptureLines pieces included, for
// what came before it; a caller discards them.
//
// It validates as it goes and reports the first syntax error with its byte
// offset. The window and each capture grow to at most their caps and are
// reused for every later value: nothing is allocated per value.
func Stream(r io.Reader, paths []Path, fn func(path int, value []byte, truncated bool) bool) (read int64, err error) {
	s, err := newStreamer(r, paths, fn)
	if err != nil {
		return 0, err
	}
	err = s.run()
	return s.read, err
}

// StreamOne is Stream over exactly one top-level value, with whitespace only
// after it: the inputs json.Valid accepts.
func StreamOne(r io.Reader, paths []Path, fn func(path int, value []byte, truncated bool) bool) (read int64, err error) {
	s, err := newStreamer(r, paths, fn)
	if err != nil {
		return 0, err
	}
	s.one = true
	err = s.run()
	return s.read, err
}

type tap struct {
	mask  uint64
	depth int
}

type streamer struct {
	r        io.Reader
	win      []byte
	pos, end int
	base     int64
	read     int64
	eof      bool
	rerr     error

	paths   []Path
	caps    []capture
	fn      func(int, []byte, bool) bool
	stopped bool

	depth   int
	objects [maxDepth/64 + 1]uint64
	masks   [maxPathElems + 2]uint64
	taps    []tap
	tapFrom int

	one     bool
	strict  uint64
	key     []byte
	keyLong bool
	longest int
	t, cont uint64
}

func newStreamer(r io.Reader, paths []Path, fn func(int, []byte, bool) bool) (*streamer, error) {
	if len(paths) > maxPaths {
		return nil, ErrPaths
	}
	s := &streamer{r: r, win: make([]byte, firstWindow), paths: paths, fn: fn,
		caps: make([]capture, len(paths)), taps: make([]tap, 0, len(paths))}
	for i, p := range paths {
		if len(p.Elems) > maxPathElems || (p.Max <= 0 && p.Mode != CapturePresence) {
			return nil, ErrPaths
		}
		for _, e := range p.Elems {
			n := len(e)
			if p.Fold {
				// A folded character can take more bytes than the one it
				// matches, as in KeySet.limit.
				n *= utf8.UTFMax
			}
			s.longest = max(s.longest, n)
		}
		if err := s.caps[i].init(s, i, p); err != nil {
			return nil, err
		}
		if p.String {
			s.strict |= 1 << i
		}
	}
	s.key = make([]byte, 0, s.longest)
	return s, nil
}

func (s *streamer) run() error {
	all := uint64(0)
	for i, p := range s.paths {
		if len(p.Elems) == 0 {
			all |= 1 << i
		}
	}
	for first := true; ; first = false {
		ws := s.skipWS()
		if _, ok := s.peek(); !ok {
			if s.rerr != nil || first {
				return s.endErr()
			}
			return nil
		}
		if !first && (!ws || s.one) {
			return s.syntax("invalid character after top-level value")
		}
		s.t, s.cont = all, s.rootMask()
		if err := s.value(); err != nil || s.stopped {
			return err
		}
	}
}

func (s *streamer) rootMask() uint64 {
	m := uint64(0)
	for i, p := range s.paths {
		if len(p.Elems) > 0 {
			m |= 1 << i
		}
	}
	return m
}

// value parses one complete value at pos, iteratively.
func (s *streamer) value() error {
	for {
		opened, err := s.begin()
		if err != nil || s.stopped {
			return err
		}
		if opened {
			closed, err := s.firstOrClose()
			if err != nil || s.stopped {
				return err
			}
			if !closed {
				continue
			}
		}
		for {
			if s.depth == 0 || s.stopped {
				return nil
			}
			more, err := s.next()
			if err != nil {
				return err
			}
			if more {
				break
			}
		}
	}
}

// begin parses a scalar whole, or opens a container and reports so.
func (s *streamer) begin() (bool, error) {
	c, ok := s.peek()
	if !ok {
		return false, s.endErr()
	}
	if c != '"' && c != 'n' && s.t&s.strict != 0 {
		return false, ErrNotString
	}
	switch {
	case c == '{' || c == '[':
		if s.depth >= maxDepth {
			return false, s.syntax("exceeded max depth")
		}
		s.startValue(s.t, true)
		s.pos++
		s.push(c == '{')
		return true, nil
	case c == '"':
		s.present(s.t)
		s.startCaps(s.t, false)
		if err := s.str(s.t, false); err != nil {
			return false, err
		}
		s.finishCaps(s.t)
		return false, nil
	case c == 'n':
		return false, s.literal("null", 0)
	case c == 't':
		return false, s.literal("true", s.t)
	case c == 'f':
		return false, s.literal("false", s.t)
	case c == '-' || (c >= '0' && c <= '9'):
		return false, s.number(s.t)
	}
	return false, s.syntax("invalid character " + strconv.QuoteRune(rune(c)) + " looking for beginning of value")
}

// startValue fires presence and opens a raw tap for the wanted paths.
func (s *streamer) startValue(t uint64, container bool) {
	s.present(t)
	raw := t & s.captured()
	if raw == 0 {
		return
	}
	s.flushTaps(s.pos)
	s.startCaps(raw, true)
	d := s.depth
	if container {
		d++
	}
	s.taps = append(s.taps, tap{mask: raw, depth: d})
}

func (s *streamer) captured() uint64 {
	m := uint64(0)
	for i, p := range s.paths {
		if p.Mode != CapturePresence {
			m |= 1 << i
		}
	}
	return m
}

func (s *streamer) present(t uint64) {
	for i := range s.paths {
		if t&(1<<i) != 0 && s.paths[i].Mode == CapturePresence && !s.stopped {
			s.stopped = !s.fn(i, nil, false)
		}
	}
}
