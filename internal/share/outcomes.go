// Package share builds the bundles a person may choose to hand over, and
// nothing else: it holds no network package (TestTRU40b_ShareLinksNoNetworkPackage),
// writes no file, and reads the store only through the Reader its caller
// passes in. The caller prints a bundle in full, then its hash, and only then
// writes it (Task 4.3), so what a person reviews is the bytes that are kept.
//
// The outcomes bundle is counts, closed pattern ids and dates. No string in it
// is text from a session: every string is one of a closed set or a fixed form
// (Validate states each), and a Reader that offers anything else is refused,
// not trimmed.
package share

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// SchemaOutcomes is the schema field of the outcomes bundle. The bundle's
// keys are append-only: a later schema adds keys and a new number, never
// changes what one means.
const SchemaOutcomes = "outcomes/1"

// MinCount is the smallest count printed as a number. A count below it --
// zero included -- prints as SmallCount, so a bundle never says that one
// session, or none, did something.
const MinCount = 5

// SmallCount is what a count under MinCount prints as.
const SmallCount = "<5"

// MaxCount bounds a count a Reader may report. It is far above any real tally
// and below what a float64 holds exactly, so a corrupt store cannot produce a
// number a reader of the bundle would round.
const MaxCount = 1 << 40

// dayForm is the form of a period's ends: a calendar day, as the file names
// a store's days.
const dayForm = "2006-01-02"

// digestLen is the length of install_digest: a hex SHA-256.
const digestLen = 64

// Count is a tally. It marshals as its number, or as SmallCount below MinCount.
// Its value is never negative: Build refuses a Reader that reports one.
type Count int

// MarshalJSON implements json.Marshaler: the suppression is in the type, so no
// path to JSON can print a small count as a number.
func (c Count) MarshalJSON() ([]byte, error) {
	if c < MinCount {
		return []byte(`"` + SmallCount + `"`), nil
	}
	return []byte(strconv.Itoa(int(c))), nil
}

// Period is the days a bundle covers, both ends included.
type Period struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Counts are the totals over every pattern.
type Counts struct {
	Fired      Count `json:"fired"`
	Fixed      Count `json:"fixed"`
	Ignored    Count `json:"ignored"`
	Overridden Count `json:"overridden"`
}

// Precision is what W1 measures for a detector: how many of its firings a
// person labelled, and how many of those labels said it was right.
type Precision struct {
	Labelled Count `json:"labelled"`
	True     Count `json:"true"`
}

// PatternRow is one pattern's line of the bundle.
type PatternRow struct {
	Pattern   pattern.ID `json:"pattern"`
	Fired     Count      `json:"fired"`
	Precision Precision  `json:"precision"`
}

// Bundle is the outcomes bundle. Its fields are the whole of it: Render adds
// nothing, so the preview and the file are one encoding.
type Bundle struct {
	Schema        string       `json:"schema"`
	InstallDigest string       `json:"install_digest"`
	Period        Period       `json:"period"`
	Counts        Counts       `json:"counts"`
	ByPattern     []PatternRow `json:"by_pattern"`
}

// Tally is one pattern's raw counts, as a Reader reports them.
type Tally struct {
	Pattern    pattern.ID
	Fired      int
	Fixed      int
	Ignored    int
	Overridden int
	Labelled   int
	True       int
}

// Outcomes is everything a Reader reports: the install's digest, the days
// covered and one Tally per pattern that fired.
type Outcomes struct {
	InstallDigest string
	Period        Period
	Tallies       []Tally
}

// Reader is where the counts come from. Phase 2 writes the outcome labels
// (fired, fixed, ignored, overridden) and will supply the implementation that
// reads them; until then the package is tested against a fake, and the
// command has no reader to wire (cmd/rashomon/share.go).
type Reader interface {
	Outcomes() (Outcomes, error)
}

// Build reads o from r and returns the bundle, or an error naming the first
// thing a closed set does not allow. Tallies come out in pattern.All order,
// whatever order the Reader gave them. The totals are summed from the raw
// counts before any suppression. Time is linear in the tallies, which are at
// most one per pattern.
func Build(r Reader) (Bundle, error) {
	o, err := r.Outcomes()
	if err != nil {
		return Bundle{}, err
	}
	if err := checkDigest(o.InstallDigest); err != nil {
		return Bundle{}, err
	}
	if err := checkPeriod(o.Period); err != nil {
		return Bundle{}, err
	}
	if len(o.Tallies) > len(pattern.All()) {
		return Bundle{}, fmt.Errorf("share: %d tallies for %d patterns", len(o.Tallies), len(pattern.All()))
	}
	byID := make(map[pattern.ID]Tally, len(o.Tallies))
	for _, t := range o.Tallies {
		if !pattern.Valid(t.Pattern) {
			return Bundle{}, errors.New("share: a tally names a pattern outside the closed set")
		}
		if _, dup := byID[t.Pattern]; dup {
			return Bundle{}, fmt.Errorf("share: two tallies for %s", t.Pattern)
		}
		if err := checkTally(t); err != nil {
			return Bundle{}, err
		}
		byID[t.Pattern] = t
	}

	b := Bundle{Schema: SchemaOutcomes, InstallDigest: o.InstallDigest, Period: o.Period, ByPattern: []PatternRow{}}
	var fired, fixed, ignored, overridden int
	for _, id := range pattern.All() {
		t, ok := byID[id]
		if !ok {
			continue
		}
		fired, fixed, ignored, overridden = fired+t.Fired, fixed+t.Fixed, ignored+t.Ignored, overridden+t.Overridden
		b.ByPattern = append(b.ByPattern, PatternRow{
			Pattern:   id,
			Fired:     Count(t.Fired),
			Precision: Precision{Labelled: Count(t.Labelled), True: Count(t.True)},
		})
	}
	b.Counts = Counts{Fired: Count(fired), Fixed: Count(fixed), Ignored: Count(ignored), Overridden: Count(overridden)}
	for _, n := range []int{fired, fixed, ignored, overridden} {
		if n > MaxCount {
			return Bundle{}, fmt.Errorf("share: a total of %d passes %d", n, MaxCount)
		}
	}
	return b, nil
}

// checkTally refuses a count outside 0..MaxCount, and a precision that claims
// more true labels than labels, or more labels than firings.
func checkTally(t Tally) error {
	for _, n := range []int{t.Fired, t.Fixed, t.Ignored, t.Overridden, t.Labelled, t.True} {
		if n < 0 || n > MaxCount {
			return fmt.Errorf("share: %s reports a count of %d, outside 0..%d", t.Pattern, n, MaxCount)
		}
	}
	if t.True > t.Labelled || t.Labelled > t.Fired {
		return fmt.Errorf("share: %s reports %d true of %d labelled of %d fired", t.Pattern, t.True, t.Labelled, t.Fired)
	}
	return nil
}

// checkDigest accepts exactly a lower-case hex string of digestLen characters.
func checkDigest(s string) error {
	if len(s) != digestLen {
		return fmt.Errorf("share: install_digest is %d characters, want %d", len(s), digestLen)
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < '0' || c > '9' && c < 'a' || c > 'f' {
			return errors.New("share: install_digest is not lower-case hex")
		}
	}
	return nil
}

// checkPeriod accepts two calendar days, start not after end. The layout's
// two-digit fields and time.Parse's refusal of trailing text make the parse
// exact: 2026-10-24 is a day and 2026-10-4 and 2026-10-24T00:00:00Z are not.
func checkPeriod(p Period) error {
	var days [2]time.Time
	for i, s := range []string{p.Start, p.End} {
		d, err := time.Parse(dayForm, s)
		if err != nil {
			return errors.New("share: a period end is not a YYYY-MM-DD day")
		}
		days[i] = d
	}
	if days[0].After(days[1]) {
		return errors.New("share: the period starts after it ends")
	}
	return nil
}

// Render is the bundle's bytes: indented JSON and a final line feed. It is
// the one encoding, used for the preview and for the file. HTML escaping is
// off, because it would print SmallCount as "\u003c5", which is not what the
// schema says a small count is.
func Render(b Bundle) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(b); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
