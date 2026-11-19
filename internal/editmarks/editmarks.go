// Package editmarks computes the edit marks of an Edit or a MultiEdit
// (store.EditMarks, basis edit_delta): for each id of a fixed table, how many
// more lines of the new text than of the old text are one of its markers.
// The integrity checks of internal/verify read them (test_removed,
// skip_added, assertions_removed).
//
// A mark holds ids from the table and counts, and never a byte of the edit:
// the only strings in it are the table's ids and the basis, and a count is a
// number of lines. The producer reads old_string and new_string in place in
// the tool input, decoding no more of a line than its first bytes, and it
// allocates only for an edit that moves an id.
package editmarks

import (
	"strings"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// headBytes is how much of a line, after its leading blanks, is decoded for
// matching: more than the longest prefix and its boundary byte.
const headBytes = 48

// The keys read of an Edit's input and of one of a MultiEdit's edits; the file
// path and the edits array are read of the first alone.
func inputKeys() *jsonscan.KeySet {
	keysOnce.Do(func() {
		keysOnce.input = jsonscan.NewKeySet("file_path", "edits", "old_string", "new_string")
		keysOnce.edit = jsonscan.NewKeySet("old_string", "new_string")
	})
	return keysOnce.input
}

// editKeys is the key set of one edit of a MultiEdit.
func editKeys() *jsonscan.KeySet {
	inputKeys()
	return keysOnce.edit
}

// keysOnce holds the two key sets, built on the first call and never at init
// (RES-2).
var keysOnce struct {
	sync.Once
	input, edit *jsonscan.KeySet
}

// The slots of inputKeys' spans.
const (
	keyPath = iota
	keyEdits
	keyOld
	keyNew
)

// Of is the marks of one Edit or MultiEdit call whose tool_input is input,
// nil for any other tool, for a file that is not a test-like path in Go or
// Python, for an input it cannot read, and for an edit that moves no id. An
// id's mark is the net of its lines over all the edits: Added where the new
// text has more of them, Removed where it has fewer, so a line that is the
// same on both sides moves nothing.
//
// The path filter is cheap and generous (a path naming test, spec or e2e):
// the fold's own path class decides what counts, so a source file this keeps
// is read and then ignored there, and no test path is dropped here.
//
// input must be valid JSON, as a hook payload's tool_input is once its
// payload has been read. A replace_all edit counts one occurrence: how many
// the file held is not in the input.
func Of(tool string, input []byte) *store.EditMarks {
	if tool != "Edit" && tool != "MultiEdit" {
		return nil
	}
	root := jsonscan.ValueSpan(input)
	var top [4]jsonscan.Span
	if jsonscan.Field(input, root, inputKeys(), top[:]) != nil {
		return nil
	}
	l, ok := langOf(top[keyPath], input)
	if !ok {
		return nil
	}
	var net [len(table)]int
	readable := true
	if tool == "MultiEdit" {
		if top[keyEdits].Kind == jsonscan.Array {
			_ = jsonscan.Elements(input, top[keyEdits], func(el jsonscan.Span) bool {
				var e [2]jsonscan.Span
				readable = jsonscan.Field(input, el, editKeys(), e[:]) == nil && pair(input, e[0], e[1], l, &net)
				return readable
			})
		}
	} else {
		readable = pair(input, top[keyOld], top[keyNew], l, &net)
	}
	if !readable {
		return nil
	}
	return marksOf(&net)
}

// marksOf is net as a mark, nil where every id nets zero.
func marksOf(net *[len(table)]int) *store.EditMarks {
	var m *store.EditMarks
	for i, n := range net {
		if n == 0 {
			continue
		}
		if m == nil {
			m = &store.EditMarks{Added: map[string]int{}, Removed: map[string]int{}, Basis: Basis, Version: Version}
		}
		if n > 0 {
			m.Added[table[i].id] = n
		} else {
			m.Removed[table[i].id] = -n
		}
	}
	return m
}

// pair subtracts the lines of the old text from net and adds those of the new
// one; an absent or null value reads as empty, and one that is neither that
// nor a string is an edit no tool makes, which pair leaves unread (false).
func pair(in []byte, from, to jsonscan.Span, l lang, net *[len(table)]int) bool {
	for _, s := range [2]jsonscan.Span{from, to} {
		if k := s.Kind; k != jsonscan.Absent && k != jsonscan.Null && k != jsonscan.String {
			return false
		}
	}
	scan(body(in, from), l, -1, net)
	scan(body(in, to), l, 1, net)
	return true
}

// body is the raw bytes between the quotes of a string span, still escaped.
func body(in []byte, s jsonscan.Span) []byte {
	if s.Kind != jsonscan.String {
		return nil
	}
	raw := s.Raw(in)
	if len(raw) < 2 {
		return nil
	}
	return raw[1 : len(raw)-1]
}

// langOf is the language of the path span's file by its extension, and false
// for a path that is not a test-like Go or Python path.
func langOf(path jsonscan.Span, in []byte) (lang, bool) {
	p := body(in, path)
	var l lang
	switch {
	case hasSuffixFold(p, ".go"):
		l = langGo
	case hasSuffixFold(p, ".py"):
		l = langPy
	default:
		return 0, false
	}
	if !containsFold(p, "test") && !containsFold(p, "spec") && !containsFold(p, "e2e") {
		return 0, false
	}
	return l, true
}

func hasSuffixFold(b []byte, suffix string) bool {
	return len(b) >= len(suffix) && strings.EqualFold(string(b[len(b)-len(suffix):]), suffix)
}

// containsFold reports whether b holds the lower case word w, in any case.
func containsFold(b []byte, w string) bool {
	for i := 0; i+len(w) <= len(b); i++ {
		if strings.EqualFold(string(b[i:i+len(w)]), w) {
			return true
		}
	}
	return false
}

// lineScan reads the lines of one raw JSON string body, keeping each line's
// head: the first headBytes decoded bytes after its blanks.
type lineScan struct {
	l    lang
	sign int
	net  *[len(table)]int
	head [headBytes]byte
	n    int
	lead bool // still in the line's leading blanks
}

// scan adds sign to the net of the id of each line of raw that is a marker of
// language l. raw is the JSON-escaped body of a string: "\n" ends a line and
// the other escapes are decoded into the head.
func scan(raw []byte, l lang, sign int, net *[len(table)]int) {
	s := lineScan{l: l, sign: sign, net: net, lead: true}
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c != '\\' || i+1 == len(raw) {
			s.put(c)
			continue
		}
		i++
		switch e := raw[i]; e {
		case 'n':
			s.end()
		case 't':
			s.put('\t')
		case 'r':
			s.put('\r')
		case 'b':
			s.put('\b')
		case 'f':
			s.put('\f')
		case 'u':
			r, ok := hex4(raw, i+1)
			if !ok {
				s.put(e)
				break
			}
			i += 4
			if r < utf8Self {
				s.put(byte(r))
			} else {
				s.put(nonASCII)
			}
		default: // \" \\ \/
			s.put(e)
		}
	}
	s.end()
}

const (
	utf8Self = 0x80 // below it a rune is one byte
	// nonASCII stands for a decoded rune above ASCII: no prefix holds one, so
	// only that it is neither a blank nor an identifier byte matters.
	nonASCII = 0xff
)

// hex4 is the four hex digits at raw[i:] as a rune value.
func hex4(raw []byte, i int) (rune, bool) {
	if i+4 > len(raw) {
		return 0, false
	}
	var r rune
	for _, c := range raw[i : i+4] {
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
		r = r<<4 | rune(c)
	}
	return r, true
}

// put adds one decoded byte to the current line's head.
func (s *lineScan) put(c byte) {
	if s.lead {
		if c == ' ' || c == '\t' {
			return
		}
		s.lead = false
	}
	if s.n < len(s.head) {
		s.head[s.n] = c
		s.n++
	}
}

// end closes the current line, counting it where its head is a marker.
func (s *lineScan) end() {
	if s.n > 0 {
		s.match(s.head[:s.n])
	}
	s.n, s.lead = 0, true
}

// match counts line, a head, once, for the first id it is a marker of.
func (s *lineScan) match(line []byte) {
	for i := range table {
		r := &table[i]
		if r.lang != s.l {
			continue
		}
		for _, p := range r.prefixes {
			if len(line) >= len(p) && string(line[:len(p)]) == p && bounded(r.bound, p, line[len(p):]) {
				s.net[i] += s.sign
				return
			}
		}
	}
}

// bounded reports whether rest, what follows the prefix p of a line, satisfies
// b. A word boundary is asked only of a prefix that ends in an identifier
// byte: after "pytest.skip(" anything may follow.
func bounded(b boundary, p string, rest []byte) bool {
	switch b {
	case boundWord:
		return !identByte(p[len(p)-1]) || len(rest) == 0 || !identByte(rest[0])
	case boundGoTest:
		if len(rest) > 0 && rest[0] >= 'a' && rest[0] <= 'z' {
			return false
		}
		return !(len(rest) >= len("Main(") && string(rest[:len("Main(")]) == "Main(")
	}
	return true
}

func identByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
