package hygiene

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// isDocument reports s as one JSON object or array: a payload, a record, or a
// string holding one escaped. A bare number or word parses as JSON too, and
// is text here.
func isDocument(s string) bool {
	t := strings.TrimSpace(s)
	return t != "" && (t[0] == '{' || t[0] == '[') && json.Valid([]byte(t))
}

// units cuts a file into the pieces judged on their own: the whole file when
// it is one JSON document, and otherwise each line, which is how an .ndjson
// capture, a transcript and a text file all arrive. f gets each piece and its
// line number and returns its replacement.
func units(text string, f func(unit string, line int) string) string {
	if isDocument(text) {
		return f(text, 1)
	}
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		lines[i] = f(l, i+1)
	}
	return strings.Join(lines, "\n")
}

// rewrite walks the JSON document doc and returns it with each string
// literal replaced by str's answer and each inventory list by its length when
// list says so. Every other byte is copied, and a literal str leaves alone
// keeps its own escapes, so a scrubbed fixture differs from its capture only
// where something was removed: a decode-and-encode round trip would reorder
// members and respell numbers, and the diff would hide the change that
// matters. member is the name the literal is the value of, "" for a member
// name or an array element; at is the literal's offset in doc.
func rewrite(doc string, str func(member, s string, at int) string, list func(member string, n, at int) bool) string {
	var b strings.Builder
	b.Grow(len(doc))
	member := ""
	for i := 0; i < len(doc); {
		c := doc[i]
		if c != '"' {
			if strings.IndexByte(",{[}]", c) >= 0 {
				member = ""
			}
			b.WriteByte(c)
			i++
			continue
		}
		end := literalEnd(doc, i)
		var s string
		if end < 0 || json.Unmarshal([]byte(doc[i:end]), &s) != nil {
			b.WriteByte(c) // not reached for a document json.Valid accepted
			i++
			continue
		}
		if k := skipSpace(doc, end); k < len(doc) && doc[k] == ':' {
			if v := skipSpace(doc, k+1); inventory[s] && v < len(doc) && doc[v] == '[' {
				if close, n := arrayEnd(doc, v); close > 0 && list(s, n, i) {
					b.WriteString(doc[i:v] + strconv.Itoa(n))
					i = close
					continue
				}
			}
			b.WriteString(requote(doc[i:end], s, str("", s, i)))
			member = s
		} else {
			b.WriteString(requote(doc[i:end], s, str(member, s, i)))
			member = ""
		}
		i = end
	}
	return b.String()
}

// literalEnd returns the offset just past the string literal opening at
// doc[i], or -1 when it does not close.
func literalEnd(doc string, i int) int {
	for j := i + 1; j < len(doc); j++ {
		switch doc[j] {
		case '\\':
			j++
		case '"':
			return j + 1
		}
	}
	return -1
}

// arrayEnd returns the offset just past the array opening at doc[v] and how
// many elements it holds, or -1 when it does not close.
func arrayEnd(doc string, v int) (int, int) {
	depth, commas, any := 0, 0, false
	for j := v; j < len(doc); j++ {
		switch c := doc[j]; {
		case c == '"':
			if j = literalEnd(doc, j) - 1; j < 0 {
				return -1, 0
			}
			any = true
		case c == '[' || c == '{':
			any = any || depth == 1
			depth++
		case c == ']' || c == '}':
			if depth--; depth == 0 {
				if any {
					return j + 1, commas + 1
				}
				return j + 1, 0
			}
		case c == ',' && depth == 1:
			commas++
		case depth == 1 && c != ' ' && c != '\t' && c != '\n' && c != '\r':
			any = true
		}
	}
	return -1, 0
}

func skipSpace(doc string, i int) int {
	for i < len(doc) && (doc[i] == ' ' || doc[i] == '\t' || doc[i] == '\n' || doc[i] == '\r') {
		i++
	}
	return i
}

// requote returns raw, the literal as written, when s is unchanged, and the
// new value encoded otherwise, without the HTML escaping encoding/json adds
// by default, which would turn every & and < in a command into &.
func requote(raw, old, s string) string {
	if s == old {
		return raw
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s) // a Go string always encodes
	return strings.TrimSuffix(b.String(), "\n")
}
