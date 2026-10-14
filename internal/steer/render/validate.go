package render

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// MessagePrefix is the fixed prefix every rendered model-facing string
// starts with (tech 4.2.7: "starts with `rashomon record: `").
const MessagePrefix = "rashomon record: "

// MirrorPrefix and MirrorSuffix wrap a rendered model_text into the
// user-facing mirror, byte for byte (tech 4.2.7: "Mirror: `※ rashomon told
// the agent: "<model text>"`, byte for byte"). The mirror form itself is
// not ASCII -- it opens with U+203B, a reference mark -- so Mirror does not
// run the wrapped string back through Validate, whose ASCII rule governs
// only the model-facing text the mirror quotes, not the wrapper around it.
const (
	MirrorPrefix = "※ rashomon told the agent: \""
	MirrorSuffix = "\""
)

// MaxMessageBytes is the byte cap on a rendered note (tech 4.2.7: "at most
// 1,200 bytes").
const MaxMessageBytes = 1200

// MaxBriefingMessageBytes is continuity-briefing's wider cap (tech 4.2.7:
// "briefing 1,600").
const MaxBriefingMessageBytes = 1600

// maxBriefingLines is a briefing's header and at most five lines (archive
// 40.4.3: "then at most 5 lines").
const maxBriefingLines = 6

// Mirror wraps modelText for user-facing display. It performs no
// validation of its own: callers are expected to have already run
// modelText through Validate, and Mirror's only job is the fixed wrap,
// byte for byte.
func Mirror(modelText string) string {
	return MirrorPrefix + modelText + MirrorSuffix
}

// forbiddenSubstring is refused in any case (tech 4.2.7; archive 40.3.8:
// "the substring `system-reminder` (case-insensitive)").
const forbiddenSubstring = "system-reminder"

// bannedWords may not appear outside a quote (tech 4.2.7: "`must`,
// `should`, `please`, `need`, `verify`, `verified`, `compliant`, `catch`,
// `caught`, `lie`, `cheat`, `deleted`; ASCII case-insensitive whole words").
var bannedWords = []string{
	"must", "should", "please", "need", "verify", "verified",
	"compliant", "catch", "caught", "lie", "cheat", "deleted",
}

// Validate checks a rendered note against tech 4.2.7's whole-string rule:
// it starts with MessagePrefix, is at most MaxMessageBytes, and is one line
// that passes checkLine: no banned word, no second person and no imperative
// outside a quote (templates.go), the same check STR-14 runs over the
// template table.
func Validate(s string) error {
	if !strings.HasPrefix(s, MessagePrefix) {
		return fmt.Errorf("render: message does not start with %q", MessagePrefix)
	}
	if len(s) > MaxMessageBytes {
		return fmt.Errorf("render: message is %d bytes, want at most %d", len(s), MaxMessageBytes)
	}
	return checkLine(s)
}

// ValidateBriefing checks a continuity briefing, the only multi-line note
// (G4: "A briefing, the only multi-line note, is split at its template's
// line feeds and each line checked separately; a line feed anywhere else is
// refused"): it starts with MessagePrefix, is at most
// MaxBriefingMessageBytes, holds at most maxBriefingLines lines, uses a line
// feed only between two non-empty lines, and every line passes checkLine.
func ValidateBriefing(s string) error {
	if !strings.HasPrefix(s, MessagePrefix) {
		return fmt.Errorf("render: briefing does not start with %q", MessagePrefix)
	}
	if len(s) > MaxBriefingMessageBytes {
		return fmt.Errorf("render: briefing is %d bytes, want at most %d", len(s), MaxBriefingMessageBytes)
	}
	lines := strings.Split(s, "\n")
	if len(lines) > maxBriefingLines {
		return fmt.Errorf("render: briefing has %d lines, want at most %d", len(lines), maxBriefingLines)
	}
	for _, line := range lines {
		if line == "" {
			return errors.New("render: briefing has an empty line")
		}
		if err := checkLine(line); err != nil {
			return err
		}
	}
	return nil
}

// checkLine applies the rules every line obeys: ASCII only; none of '<',
// '>', a backtick or a control rune (a line feed included); no
// `system-reminder` in any case; and no banned word outside a quote.
func checkLine(line string) error {
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case c > unicode.MaxASCII:
			return errors.New("render: message holds a non-ASCII byte")
		case c == '<' || c == '>' || c == '`':
			return fmt.Errorf("render: message holds the forbidden character %q", c)
		case c < 0x20 || c == 0x7f:
			return errors.New("render: message holds a control character")
		}
	}
	if strings.Contains(strings.ToLower(line), forbiddenSubstring) {
		return fmt.Errorf("render: message holds %q", forbiddenSubstring)
	}
	return checkWords(line)
}

// checkWords refuses a banned word, a second-person word or an imperative
// outside a quote (statementOfFact). A
// quote opens at a single quote that does not follow a letter or digit and
// closes at the next one not followed by a letter or digit, so a single
// quote inside a word ("project's", "run's") is an apostrophe. A quote
// still open at the end of the line is refused, so an unclosed quote cannot
// hide the rest of the line from this check.
func checkWords(line string) error {
	unquoted, err := outsideQuotes(line)
	if err != nil {
		return err
	}
	start := -1
	for i := 0; i <= len(unquoted); i++ {
		if i < len(unquoted) && isAlnum(unquoted[i]) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			if w, ok := bannedWord(unquoted[start:i]); ok {
				return fmt.Errorf("render: message holds the banned word %q outside a quote", w)
			}
			start = -1
		}
	}
	return statementOfFact(unquoted)
}

// outsideQuotes returns line with every quoted span replaced by one space,
// or an error when a quote is never closed.
func outsideQuotes(line string) (string, error) {
	var out strings.Builder
	inQuote := false
	for i := 0; i < len(line); i++ {
		if line[i] == '\'' {
			prev := i > 0 && isAlnum(line[i-1])
			next := i+1 < len(line) && isAlnum(line[i+1])
			if !inQuote && !prev {
				inQuote = true
				out.WriteByte(' ')
				continue
			} else if inQuote && !next {
				inQuote = false
				continue
			}
		}
		if !inQuote {
			out.WriteByte(line[i])
		}
	}
	if inQuote {
		return "", errors.New("render: message holds a quote that is never closed")
	}
	return out.String(), nil
}

func bannedWord(w string) (string, bool) {
	for _, b := range bannedWords {
		if strings.EqualFold(w, b) {
			return b, true
		}
	}
	return "", false
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
