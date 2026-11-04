package slot

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// MaxSaidBytes is the most of the person's words one quote holds: the clause
// that states a directive, not a pasted page (37a15912, 2026-11-02: of the 58
// typed prompts at or under it, 57 keep the line law; of the 20 over it, none
// would).
const MaxSaidBytes = 240

// Said is one of the person's standing directives, quoted verbatim from the
// session's transcript by the continuity briefing (COMPACT-SLOT1; tech 4.2.4
// :1255, STR-46), with the earlier words it restates when it is a
// correction. The words are the person's own, so they render inside an
// attributed quote, never as rashomon's text, and keep the line law every
// message keeps (G4: ASCII, none of < > ` ", no control character, no
// system-reminder) with Approved's apostrophe, URL-scheme and secret checks.
// Words that break one are withheld whole, never cut or changed. The banned
// words are the quote's to hold, as Approved's are.
//
// The record form holds each quote's keyed digest (intent.SaidDigest, under
// the install key) and byte length, never the words (D160). It renders as the
// byte counts, and it is the only form a steer line can hold: steer refuses
// to record the other.
type Said struct {
	words, restates string
	digest, rDigest string
	n, rn           int
}

// NewSaid quotes words, with restates, the earlier words a correction says
// again, or "".
func NewSaid(words, restates string) Said { return Said{words: words, restates: restates} }

// NewSaidDigest is the record form: the keyed digest and byte length of a
// quote and of the words it restates ("" and 0 when it restates none).
func NewSaidDigest(digest string, n int, restatesDigest string, restatesN int) Said {
	return Said{digest: digest, n: n, rDigest: restatesDigest, rn: restatesN}
}

// Keyed reports the record form.
func (s Said) Keyed() bool { return s.digest != "" }

// Digest, Bytes, RestatesDigest and RestatesBytes are the record form's
// values; each is "" or 0 in the other form.
func (s Said) Digest() string         { return s.digest }
func (s Said) Bytes() int             { return s.n }
func (s Said) RestatesDigest() string { return s.rDigest }
func (s Said) RestatesBytes() int     { return s.rn }

func (s Said) Validate() error {
	if s.Keyed() {
		return s.validateKeyed()
	}
	if err := sayable(s.words); err != nil {
		return err
	}
	if s.restates != "" {
		return sayable(s.restates)
	}
	return nil
}

// validateKeyed checks the record form: digests only, each 64 lowercase hex
// over 1 to MaxSaidBytes bytes, and the restated pair both set or both not.
func (s Said) validateKeyed() error {
	switch {
	case s.words != "" || s.restates != "":
		return errors.New("slot: said holds both words and digests")
	case !digestShaped(s.digest) || s.n < 1 || s.n > MaxSaidBytes:
		return fmt.Errorf("slot: said digest is not 64 lowercase hex over 1 to %d bytes", MaxSaidBytes)
	case s.rDigest == "" && s.rn == 0:
		return nil
	case !digestShaped(s.rDigest) || s.rn < 1 || s.rn > MaxSaidBytes:
		return fmt.Errorf("slot: said restates digest is not 64 lowercase hex over 1 to %d bytes", MaxSaidBytes)
	}
	return nil
}

func (s Said) Text() string {
	if s.Keyed() {
		t := "a quote of " + strconv.Itoa(s.n) + " bytes"
		if s.rDigest != "" {
			t += ", restating a quote of " + strconv.Itoa(s.rn) + " bytes"
		}
		return t
	}
	if s.restates == "" {
		return quoted(s.words)
	}
	return quoted(s.words) + ", restating " + quoted(s.restates)
}

func (s Said) bind() (Value, error) { return checked(s, s.Validate()) }

// sayable is the line law over one quote of the person's words. Its error
// names the rule, never the words.
func sayable(w string) error {
	if len(w) == 0 || len(w) > MaxSaidBytes {
		return fmt.Errorf("slot: said words are not 1 to %d bytes", MaxSaidBytes)
	}
	for i := 0; i < len(w); i++ {
		switch c := w[i]; {
		case c < 0x20 || c > 0x7e:
			return errors.New("slot: said words hold a byte outside printable ASCII")
		case c == '<' || c == '>' || c == '`' || c == '"':
			return errors.New("slot: said words hold a character the line law refuses")
		case c == '\'' && (i == 0 || i == len(w)-1 || !isAlnum(w[i-1]) || !isAlnum(w[i+1])):
			return errors.New("slot: said words hold a quote that is not an apostrophe")
		}
	}
	if strings.Contains(strings.ToLower(w), "system-reminder") {
		return errors.New("slot: said words hold system-reminder")
	}
	if urlSchemeRE.get().MatchString(w) {
		return errors.New("slot: said words hold a URL scheme")
	}
	if err := denied("said words", w); err != nil {
		return err
	}
	for _, word := range strings.FieldsFunc(w, func(r rune) bool { return strings.ContainsRune(" ,;()", r) }) {
		if highEntropy(word) {
			return fmt.Errorf("%w: said words hold a word that fails the entropy test", ErrSecretShaped)
		}
	}
	return nil
}
