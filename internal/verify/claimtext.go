package verify

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The message as the claim comparator reads it (spec 20.2.5 steps 1 to 3 and
// the word rule): each window capped, sanitised and stripped, split into
// sentences, each sentence split into words. Claims, in claims.go, compares
// what these give with the record.

// maxQuotedRunes bounds a double-quoted span step 2 removes.
const maxQuotedRunes = 200

// claimClauseBreaks end a clause, and with it a negation (claimsPass's list).
const claimClauseBreaks = ".,;:!?()\n"

// capWindow is b cut to MaxClaimMessageBytes on a rune boundary, and whether
// it was cut.
func capWindow(b []byte) ([]byte, bool) {
	if len(b) <= MaxClaimMessageBytes {
		return b, false
	}
	i := MaxClaimMessageBytes
	for i > 0 && !utf8.RuneStart(b[i]) {
		i--
	}
	return b[:i], true
}

// sanitize is b with the runes sanitizeMessage drops dropped (account.go):
// every rune neither graphic nor space. An invalid byte reads as U+FFFD,
// which is graphic, as it does there. A new slice: b is never written.
func sanitize(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); {
		if c := b[i]; c < utf8.RuneSelf { // the ASCII the two tests keep: printable, or one of the six spaces
			if ' ' <= c && c < 0x7f || '\t' <= c && c <= '\r' {
				out = append(out, c)
			}
			i++
			continue
		}
		r, n := utf8.DecodeRune(b[i:])
		if unicode.IsGraphic(r) || unicode.IsSpace(r) {
			out = utf8.AppendRune(out, r)
		}
		i += n
	}
	return out
}

// stripWindow is step 2 over a sanitised window, in place: the line rules on
// the raw line, then the span rules inside each line kept. A removed line
// leaves its newline, so the text around it never joins into one sentence.
func stripWindow(b []byte, delivered [][]byte) []byte {
	out := b[:0]
	fenced := false
	for len(b) > 0 {
		line := b
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			line, b = b[:i], b[i+1:]
		} else {
			b = nil
		}
		lead := bytes.TrimLeft(line, " \t")
		switch {
		case bytes.HasPrefix(lead, []byte("```")):
			fenced = !fenced
		case fenced, bytes.HasPrefix(lead, []byte(">")), isDelivered(lead, delivered):
		default:
			n := len(out)
			out = append(out, line...) // never past line's own bytes: out trails b
			out = out[:n+len(cutURLs(cutSpans(cutSpans(out[n:], '`', 0), '"', maxQuotedRunes)))]
		}
		out = append(out, '\n')
	}
	return out
}

// isDelivered reports whether a line, past its leading spaces, is "※" and,
// past the spaces after it, one of delivered byte for byte. A bare "※" or a
// prefix of a delivered line is not one.
func isDelivered(lead []byte, delivered [][]byte) bool {
	rest, ok := bytes.CutPrefix(lead, []byte("※"))
	if !ok {
		return false
	}
	text := bytes.TrimRight(bytes.TrimLeft(rest, " \t"), " \t\r")
	for _, d := range delivered {
		if bytes.Equal(text, d) {
			return true
		}
	}
	return false
}

// cutSpans replaces, in place, each span from a q to the next q with one
// space. With max above 0, a span of more than max runes between its quotes
// is none, and its opening q stays.
func cutSpans(b []byte, q byte, max int) []byte {
	if bytes.IndexByte(b, q) < 0 {
		return b
	}
	w := 0
	for r := 0; r < len(b); r++ {
		if b[r] == q {
			end := bytes.IndexByte(b[r+1:], q)
			if end >= 0 && (max == 0 || utf8.RuneCount(b[r+1:r+1+end]) <= max) {
				b[w] = ' '
				w++
				r += end + 1
				continue
			}
		}
		b[w] = b[r]
		w++
	}
	return b[:w]
}

// cutURLs replaces, in place, each URL with one space: a scheme of letters
// and "://", from a letter that follows none, to the next space or tab. The
// scheme is read only where a run of letters starts, so a long run is read
// once, not once per letter.
func cutURLs(b []byte) []byte {
	if !bytes.Contains(b, []byte("://")) {
		return b
	}
	w := 0
	for r := 0; r < len(b); {
		n := 0
		if r == 0 || !isLetter(b[r-1]) {
			n = schemeLen(b[r:])
		}
		if n > 0 {
			end := r + n
			for end < len(b) && b[end] != ' ' && b[end] != '\t' {
				end++
			}
			b[w] = ' '
			w++
			r = end
			continue
		}
		b[w] = b[r]
		w++
		r++
	}
	return b[:w]
}

// schemeLen is the length of the letters and "://" b opens with, or 0.
func schemeLen(b []byte) int {
	i := 0
	for i < len(b) && isLetter(b[i]) {
		i++
	}
	if i > 0 && bytes.HasPrefix(b[i:], []byte("://")) {
		return i + 3
	}
	return 0
}

// appendSentences appends s's sentences to dst (step 3): each ends after
// '.', '!' or '?' followed by a space or the end, or at a newline; trimmed;
// an empty one is dropped.
func appendSentences(dst []string, s string) []string {
	start := 0
	for i := 0; i < len(s); i++ {
		end := -1
		switch s[i] {
		case '\n':
			end = i
		case '.', '!', '?':
			if r, _ := utf8.DecodeRuneInString(s[i+1:]); i+1 == len(s) || unicode.IsSpace(r) {
				end = i + 1
			}
		}
		if end < 0 {
			continue
		}
		if t := strings.TrimSpace(s[start:end]); t != "" {
			dst = append(dst, t)
		}
		start = i + 1
	}
	if t := strings.TrimSpace(s[start:]); t != "" {
		dst = append(dst, t)
	}
	return dst
}

// words is s's words: runs of letters, digits, '/' and apostrophes,
// lower-cased, the typographic apostrophe read as ', apostrophes trimmed.
// claimsPass's rule, with digits and '/' kept so "42/42" is one word (Gate B
// item 1).
func words(s string) []string {
	ws, _ := appendWords(nil, nil, s)
	return ws
}

// appendWords appends s's words to ws and, for each, to cs the clause start
// counted from the first word s appends (claimClauseBreaks).
func appendWords(ws []string, cs []int, s string) ([]string, []int) {
	base := len(ws)
	clause := 0
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if !wordRune(r) {
			if strings.ContainsRune(claimClauseBreaks, r) {
				clause = len(ws) - base
			}
			i += n
			continue
		}
		j, ascii := i+n, r < utf8.RuneSelf
		for j < len(s) {
			r, n := utf8.DecodeRuneInString(s[j:])
			if !wordRune(r) {
				break
			}
			j, ascii = j+n, ascii && r < utf8.RuneSelf
		}
		w := strings.ToLower(s[i:j])
		if !ascii {
			w = strings.ReplaceAll(w, "’", "'")
		}
		if w = strings.Trim(w, "'"); w != "" {
			ws, cs = append(ws, w), append(cs, clause)
		}
		i = j
	}
	return ws, cs
}

func wordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '/' || r == '\'' || r == '’'
}
