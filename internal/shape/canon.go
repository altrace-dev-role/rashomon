package shape

import (
	"hash"
	"math"
	"slices"
	"strconv"
	"unicode/utf8"
)

// canonEncoder writes a value decoded into any exactly as json.Marshal
// encodes it -- sorted object keys, HTML-safe string escapes, ES6 number
// formatting -- through a fixed buffer, so the canonical form of an 8 MiB
// tool input is never held whole. FuzzCanonicalMatchesMarshal holds it to
// json.Marshal byte for byte.
type canonEncoder struct {
	w   hash.Hash
	buf [4 << 10]byte
	n   int
}

func (e *canonEncoder) flush() {
	e.w.Write(e.buf[:e.n])
	e.n = 0
}

func (e *canonEncoder) write(b []byte) {
	for len(b) > 0 {
		if e.n == len(e.buf) {
			e.flush()
		}
		k := copy(e.buf[e.n:], b)
		e.n += k
		b = b[k:]
	}
}

func (e *canonEncoder) writeString(s string) {
	for len(s) > 0 {
		if e.n == len(e.buf) {
			e.flush()
		}
		k := copy(e.buf[e.n:], s)
		e.n += k
		s = s[k:]
	}
}

func (e *canonEncoder) byte1(c byte) {
	if e.n == len(e.buf) {
		e.flush()
	}
	e.buf[e.n] = c
	e.n++
}

func (e *canonEncoder) value(v any) {
	switch x := v.(type) {
	case nil:
		e.writeString("null")
	case bool:
		if x {
			e.writeString("true")
		} else {
			e.writeString("false")
		}
	case float64:
		e.float(x)
	case string:
		e.str(x)
	case []any:
		e.byte1('[')
		for i, el := range x {
			if i > 0 {
				e.byte1(',')
			}
			e.value(el)
		}
		e.byte1(']')
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		e.byte1('{')
		for i, k := range keys {
			if i > 0 {
				e.byte1(',')
			}
			e.str(k)
			e.byte1(':')
			e.value(x[k])
		}
		e.byte1('}')
	}
}

// float formats as encoding/json does: as if by ES6 number-to-string, %g
// with JSON's exponent cutoffs and an unpadded exponent.
func (e *canonEncoder) float(f float64) {
	var tmp [32]byte
	format := byte('f')
	if abs := math.Abs(f); abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		format = 'e'
	}
	b := strconv.AppendFloat(tmp[:0], f, format, -1, 64)
	if format == 'e' {
		if n := len(b); n >= 4 && b[n-4] == 'e' && b[n-3] == '-' && b[n-2] == '0' {
			b[n-2] = b[n-1]
			b = b[:n-1]
		}
	}
	e.write(b)
}

const hexDigits = "0123456789abcdef"

// str quotes as encoding/json does with HTML escaping on: <, > and & as
// \u003c, \u003e and \u0026, U+2028 and U+2029 escaped, invalid UTF-8 as
// \ufffd, and control bytes as \b \f \n \r \t or \u00XX.
func (e *canonEncoder) str(s string) {
	e.byte1('"')
	start := 0
	for i := 0; i < len(s); {
		if c := s[i]; c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
				i++
				continue
			}
			e.writeString(s[start:i])
			switch c {
			case '"', '\\':
				e.write([]byte{'\\', c})
			case '\b':
				e.writeString(`\b`)
			case '\f':
				e.writeString(`\f`)
			case '\n':
				e.writeString(`\n`)
			case '\r':
				e.writeString(`\r`)
			case '\t':
				e.writeString(`\t`)
			default:
				e.write([]byte{'\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xF]})
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			e.writeString(s[start:i])
			e.writeString(`\ufffd`)
		case r == '\u2028' || r == '\u2029':
			e.writeString(s[start:i])
			e.write([]byte{'\\', 'u', '2', '0', '2', hexDigits[r&0xF]})
		default:
			i += size
			continue
		}
		i += size
		start = i
	}
	e.writeString(s[start:])
	e.byte1('"')
}
