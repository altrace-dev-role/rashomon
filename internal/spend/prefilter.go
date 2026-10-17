package spend

import "bytes"

// usageMarker is a byte test run before any decoding of a main transcript's
// lines. Only a line that carries an API response's usage can contribute, and
// on a real transcript most lines do not (attachments, tool results,
// bookkeeping); skipping them without a parse is what keeps a multi-gigabyte
// history affordable. A subagent transcript's every line is decoded, since
// any of them may end its tie (readFile).
var usageMarker = []byte(`"usage"`)

// mayCarryUsage is the prefilter's question -- may the decode read a usage
// object from this line? -- answered without decoding, and never "no" for a
// line the decode would read one from (FuzzUsagePrefilter). usageMarker is the
// common case. encoding/json also accepts the key in any case under Unicode
// folding ("Usage", and "uſage", whose U+017F folds to s) and with escapes
// ("us\u0061ge"), so a line passes as well when it holds a backslash-u
// escape anywhere, or a quoted five- or six-byte word that folds to "usage"
// (the folded s is the one two-byte letter that can stand in it). Not every
// backslash: nearly every real line has an escaped newline or quote, and
// keeping those would undo the prefilter.
//
// Unparsed stays tied to usageMarker alone (readFile): a line this widening
// keeps and the decode then rejects was never shown to carry usage.
func mayCarryUsage(raw []byte) bool {
	if bytes.Contains(raw, usageMarker) || bytes.Contains(raw, escapeMarker) {
		return true
	}
	for i := 0; ; {
		q := bytes.IndexByte(raw[i:], '"')
		if q < 0 {
			return false
		}
		i += q + 1
		if i < len(raw) && raw[i]|0x20 == 'u' && foldsToUsageKey(raw[i:]) {
			return true
		}
	}
}

// escapeMarker opens a \uXXXX escape, the only escape that can spell a letter.
var escapeMarker = []byte(`\u`)

// foldsToUsageKey reports whether b opens with a word that folds to "usage"
// and then a closing quote.
func foldsToUsageKey(b []byte) bool {
	for _, n := range [2]int{len("usage"), len("usage") + 1} {
		if len(b) > n && b[n] == '"' && bytes.EqualFold(b[:n], []byte("usage")) {
			return true
		}
	}
	return false
}
