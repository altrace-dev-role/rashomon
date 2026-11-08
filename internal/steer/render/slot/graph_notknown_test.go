package slot

import (
	"slices"
	"strings"
	"testing"
)

// TestGU6_TheNotKnownLineNamesTheUnitThenTheKindsWithheld (W-M1.38, the
// unit rule): the not-known line says the unit is not known, then the
// kinds of role line withheld with it in the unit line's order whatever
// order it was given them; it is its own kind, no unit line, and its
// record form is the line itself, keyed and valid. A line that does not
// name the unit first, names nothing, names a kind twice or one outside
// the four, or holds a second fact, is refused in both forms, each beside
// a twin that binds.
func TestGU6_TheNotKnownLineNamesTheUnitThenTheKindsWithheld(t *testing.T) {
	for _, c := range []struct {
		name    string
		unknown []string
		want    string
	}{
		{"the unit alone", []string{UnknownUnit}, "not known: the unit to change"},
		{"every kind, given out of order", []string{UnknownUnit, UnknownKnowledge, UnknownTarget, UnknownConsumer, UnknownReuse},
			"not known: the unit to change, the file to change, the code to reuse, the consumers of the change, the records that apply to the change"},
	} {
		v, err := Bind(NewNotKnownLine(NotKnownFact{Unknown: c.unknown}))
		if err != nil {
			t.Fatalf("%s: Bind: %v", c.name, err)
		}
		l := v.(GraphLine)
		if got := l.Text(); got != c.want {
			t.Errorf("%s: text\n got %q\nwant %q", c.name, got, c.want)
		}
		if f, ok := l.NotKnown(); !ok || !slices.Equal(f.Unknown, c.unknown) || l.Kind() != KindNotKnown {
			t.Errorf("%s: kind %q fact %+v", c.name, l.Kind(), f)
		}
		if _, ok := l.Unit(); ok {
			t.Errorf("%s: a unit line", c.name)
		}
		rec := l.Record(func(string) string { return strings.Repeat("ab", 32) })
		if f, ok := rec.NotKnown(); !ok || !rec.Keyed() || !slices.Equal(f.Unknown, c.unknown) || rec.Digest() != "" || rec.Text() != c.want {
			t.Errorf("%s: record form %+v keyed %v digest %q text %q", c.name, f, rec.Keyed(), rec.Digest(), rec.Text())
		}
		if err := rec.Validate(); err != nil {
			t.Errorf("%s: record form: %v", c.name, err)
		}
	}
	for _, c := range []struct {
		name      string
		bad, good []string
	}{
		{"the unit not first", []string{UnknownTarget, UnknownUnit}, []string{UnknownUnit, UnknownTarget}},
		{"the unit not named", []string{UnknownTarget}, []string{UnknownUnit, UnknownTarget}},
		{"nothing named", nil, []string{UnknownUnit}},
		{"the unit twice", []string{UnknownUnit, UnknownUnit}, []string{UnknownUnit}},
		{"a kind twice", []string{UnknownUnit, UnknownReuse, UnknownReuse}, []string{UnknownUnit, UnknownReuse}},
		{"a kind outside the four", []string{UnknownUnit, "rows"}, []string{UnknownUnit, UnknownConsumer}},
	} {
		bad, good := NewNotKnownLine(NotKnownFact{Unknown: c.bad}), NewNotKnownLine(NotKnownFact{Unknown: c.good})
		if _, err := Bind(bad); err == nil {
			t.Errorf("%s: bound", c.name)
		}
		if bad.Validate() == nil {
			t.Errorf("%s: valid", c.name)
		}
		if (GraphLine{keyed: true, unknown: &NotKnownFact{Unknown: c.bad}}).Validate() == nil {
			t.Errorf("%s: the record form is valid", c.name)
		}
		if _, err := Bind(good); err != nil {
			t.Errorf("%s: the twin is refused: %v", c.name, err)
		}
		if err := (GraphLine{keyed: true, unknown: &NotKnownFact{Unknown: c.good}}).Validate(); err != nil {
			t.Errorf("%s: the twin's record form is refused: %v", c.name, err)
		}
	}
	two := GraphLine{unit: &UnitFact{Path: "internal/store"}, unknown: &NotKnownFact{Unknown: []string{UnknownUnit}}}
	if _, err := Bind(two); err == nil || two.Validate() == nil {
		t.Errorf("a line holding a unit and a not-known fact: bind %v", err)
	}
	// The bound line and its record form hold their own lists: the caller's
	// slice changed after either is made changes neither.
	kinds := []string{UnknownUnit, UnknownTarget}
	v, err := Bind(NewNotKnownLine(NotKnownFact{Unknown: kinds}))
	if err != nil {
		t.Fatal(err)
	}
	rec := v.(GraphLine).Record(func(string) string { return strings.Repeat("ab", 32) })
	kinds[1] = UnknownReuse
	v.(GraphLine).unknown.Unknown[1] = UnknownConsumer
	if f, _ := rec.NotKnown(); f.Unknown[1] != UnknownTarget {
		t.Errorf("the record form shares its list: %v", f.Unknown)
	}
	if f, _ := v.(GraphLine).NotKnown(); f.Unknown[1] != UnknownConsumer || kinds[1] != UnknownReuse {
		t.Errorf("the bound line shares the caller's list: %v %v", f.Unknown, kinds)
	}
}
