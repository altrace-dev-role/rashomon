package slot

import "errors"

// The not-known line (W-M1.38, the unit rule): when the engine withholds
// the unit line, or settles on no unit where that cell is withheld, the
// brief says so in the first line's place, so no related line moves up
// into the unit's slot and no line that stays is read as the unit's. It
// names the unit and the kinds of role line withheld with it,
// in unknownWords' order, and no path, name or record, so its record form
// is the line itself. Split from graph.go for Law 10.

// UnknownUnit is the kind a not-known line names first: the unit itself,
// which a unit line never names as not known. KindNotKnown is the line's
// Kind.
const (
	UnknownUnit  = "unit"
	KindNotKnown = "not-known"
)

// NotKnownFact is what the brief does not know when its unit is withheld:
// the unit, then the kinds of role line withheld with it, in the Unknown
// words.
type NotKnownFact struct {
	Unknown []string
}

// NewNotKnownLine is a line for f.
func NewNotKnownLine(f NotKnownFact) GraphLine { return GraphLine{unknown: &f} }

// NotKnown is the line's not-known fact and whether it is a not-known
// line.
func (l GraphLine) NotKnown() (NotKnownFact, bool) {
	if l.unknown == nil {
		return NotKnownFact{}, false
	}
	return *l.unknown, true
}

// validate is whether the line names the unit first and then each of the
// four role kinds at most once.
func (f NotKnownFact) validate() error {
	if len(f.Unknown) == 0 || f.Unknown[0] != UnknownUnit {
		return errors.New("slot: a not-known line names the unit first")
	}
	return validUnknown(f.Unknown[1:])
}

// notKnownText is "not known: the unit to change, the file to change".
func (l GraphLine) notKnownText() string { return notKnown(l.unknown.Unknown, false) }
