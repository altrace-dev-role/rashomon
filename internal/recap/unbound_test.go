package recap

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestSTD1c_TheLineNamesASpecAnEditUnbound (card SPEC-TRUST-DROP1, d4): each
// spec the session edited after the user trusted it adds two lines, under
// the value line's sentence before the AutoSteer clause, and under the done
// flag's before the pointer: the spec in the path slot's quote, then the
// verb that trusts it again, each within lineColumns. A path the slot
// refuses prints as "the repository spec"; no unbound spec adds nothing.
// Break: drop unboundLines from ValueLine or from DoneFlag, or print the
// path unchecked.
func TestSTD1c_TheLineNamesASpecAnEditUnbound(t *testing.T) {
	red := gateAt("go:test", "test", verify.Red, verify.FailedExit, 7, exitCode(1))
	const sentence = "2 files edited; go exited 1 after the last edit (call 7)"
	const report = "→ rashomon report --session sess-1"
	const steered = "AutoSteer: 1 regression location given at the failure.\n"
	im := Impact{Delivered: []NoteRef{{Pattern: "regression-location", Event: "PostToolUseFailure"}}}
	pair := func(name string) string {
		return indent + name + " was edited after you trusted it,\n" +
			indent + "so it flags to you only until you run rashomon spec trust.\n"
	}
	for _, c := range []struct {
		name    string
		unbound []string
		lines   string
	}{
		{"none", nil, ""},
		{"the repository spec", []string{".claude/rashomon/spec.json"}, pair("'.claude/rashomon/spec.json'")},
		{"a path the slot refuses", []string{"../spec.json"}, pair("the repository spec")},
	} {
		v := viewOf(verify.RowRed, wrote, red)
		v.Unbound = c.unbound
		if got, want := ValueLine(v, im, "sess-1", false), prefix+sentence+".\n"+c.lines+indent+steered+indent+report; got != want {
			t.Errorf("%s: ValueLine\n got %q\nwant %q", c.name, got, want)
		}
		v.Scan = scanOf("All 42 tests pass.", "", red)
		if got, ok := DoneFlag(v, "sess-1", false); !ok || got != prefix+doneLead+sentence+".\n"+c.lines+indent+report {
			t.Errorf("%s: DoneFlag = %q, %v, want the two lines before the pointer", c.name, got, ok)
		}
		for _, l := range strings.Split(strings.TrimSuffix(c.lines, "\n"), "\n") {
			if n := columns(l); n > lineColumns {
				t.Errorf("%s: %q is %d columns, want at most %d", c.name, l, n, lineColumns)
			}
		}
	}
}
