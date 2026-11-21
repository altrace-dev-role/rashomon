package slot

import (
	"errors"
	"slices"
	"strconv"
	"strings"
)

// IdentList is the names a note lists, each an Ident inside its own quote,
// then "and {k} more" when the record holds names the list leaves out. It
// has no cap of its own: the pattern that builds one decides how many names
// fit its message and counts the rest into the count, so a cut list never
// reads as complete. Build one with NewIdentList.
type IdentList struct {
	idents []Ident
	more   int
}

// NewIdentList lists idents, with unlisted the names the record holds
// beyond them.
func NewIdentList(idents []Ident, unlisted int) IdentList {
	return IdentList{idents: slices.Clone(idents), more: unlisted}
}

// Idents returns a copy of the listed names.
func (l IdentList) Idents() []Ident { return slices.Clone(l.idents) }

// More returns the count "and {k} more" renders.
func (l IdentList) More() int { return l.more }

// Validate checks the list as it is: a non-negative count, at least one
// name, every name passing every Ident check. A name that fails refuses the
// list, so the pattern leaves such a name out, and counts it, before it
// builds one.
func (l IdentList) Validate() error {
	if l.more < 0 {
		return errors.New("slot: identifier list has a negative count")
	}
	if len(l.idents) == 0 {
		return errors.New("slot: identifier list names nothing")
	}
	for _, i := range l.idents {
		if err := i.Validate(); err != nil {
			return err
		}
	}
	return nil
}

func (l IdentList) Text() string {
	parts := make([]string, len(l.idents))
	for i, n := range l.idents {
		parts[i] = n.Text()
	}
	s := strings.Join(parts, ", ")
	if l.more > 0 {
		s += " and " + strconv.Itoa(l.more) + " more"
	}
	return s
}

func (l IdentList) bind() (Value, error) { return checked(l, l.Validate()) }
