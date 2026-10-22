package spend

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"
)

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
// folding ("Usage", and "uſage", whose U+017F folds to s) and with a \uXXXX
// escape of any of its letters ("usage"), so a quoted word right after
// a `"` also passes when it folds to "usage" that way. The widening is
// anchored at each quoted word's start, not at every `\u` in the line: a
// tool_result's own \u001b ANSI escapes sit inside a string's content, never
// right after its opening quote, so they no longer send an otherwise
// unremarkable line to a full decode (TestRubricPERF3_AToolResultHeavyLineIsSkipped).
//
// Unparsed stays tied to usageMarker alone (readFile): a line this widening
// keeps and the decode then rejects was never shown to carry usage.
func mayCarryUsage(raw []byte) bool {
	if bytes.Contains(raw, usageMarker) {
		return true
	}
	for i := 0; ; {
		q := bytes.IndexByte(raw[i:], '"')
		if q < 0 {
			return false
		}
		i += q + 1
		if foldsToUsageKey(raw[i:]) {
			return true
		}
	}
}

// usageKeyLetters is "usage", the key foldsToUsageKey reads b against, one
// rune at a time.
var usageKeyLetters = []rune("usage")

// foldsToUsageKey reports whether b opens with a word -- each letter plain,
// case-folded, or spelt as a \uXXXX escape of either -- that reads "usage"
// under Unicode simple folding and then closes with a quote.
func foldsToUsageKey(b []byte) bool {
	for _, want := range usageKeyLetters {
		r, n, ok := decodeKeyRune(b)
		if !ok || !runeFoldsTo(r, want) {
			return false
		}
		b = b[n:]
	}
	return len(b) > 0 && b[0] == '"'
}

// decodeKeyRune decodes one rune from the start of b -- a \uXXXX escape (no
// surrogate pairs: none of "usage"'s folds needs one) or a literal UTF-8
// rune -- and reports how many bytes of b it consumed. It reports ok=false
// at the end of b, or an escape whose four digits are not hex.
func decodeKeyRune(b []byte) (r rune, n int, ok bool) {
	if len(b) >= 6 && b[0] == '\\' && b[1] == 'u' {
		v, err := strconv.ParseUint(string(b[2:6]), 16, 32)
		if err != nil {
			return 0, 0, false
		}
		return rune(v), 6, true
	}
	if len(b) == 0 {
		return 0, 0, false
	}
	r, n = utf8.DecodeRune(b)
	if r == utf8.RuneError && n <= 1 {
		return 0, 0, false
	}
	return r, n, true
}

// runeFoldsTo reports whether r and want are the same rune under Unicode
// simple case folding -- the rule strings.EqualFold applies, which is where
// U+017F (long s) folds to 's'.
func runeFoldsTo(r, want rune) bool {
	return strings.EqualFold(string(r), string(want))
}
