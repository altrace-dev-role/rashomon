package recap

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestBareIncomplete_EachTermDecides: BareIncomplete is true only for the
// incomplete record's sentence and the pointer with nothing between them, on
// an incomplete record whose row is unknown. Each other case differs from the
// bare one in one term, so dropping that term from the predicate turns the
// case true. Break: drop the recording term (a complete record fails), or the
// row term (another row fails).
func TestBareIncomplete_EachTermDecides(t *testing.T) {
	const sid = "session-1"
	const sentence = "recording incomplete for this turn; verification state unknown"
	view := func(r verify.Recording, row verify.Row) verify.StopView {
		var v verify.StopView
		v.Snap.Recording = r
		v.Summary.Row = row
		v.Summary.Sentence = sentence
		return v
	}
	bare := prefix + sentence + ".\n" + pointer(sid, false)
	for _, c := range []struct {
		name string
		v    verify.StopView
		line string
		want bool
	}{
		{"the bare line", view(verify.Incomplete, verify.RowUnknown), bare, true},
		{"a complete record", view(verify.Complete, verify.RowUnknown), bare, false},
		{"another row", view(verify.Incomplete, verify.RowRed), bare, false},
		{"a rider before the pointer", view(verify.Incomplete, verify.RowUnknown),
			prefix + sentence + ".\n" + "  a rider\n" + pointer(sid, false), false},
	} {
		if got := BareIncomplete(c.v, c.line, sid, false); got != c.want {
			t.Errorf("%s: BareIncomplete = %v, want %v", c.name, got, c.want)
		}
	}
}
