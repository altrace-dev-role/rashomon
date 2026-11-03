package recap

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestSUF1b_TheNoGateLineNamesUntrustedGatesWithinTheLine (STOP-UNBOUND-FIRST):
// the no-gate row names the gates an untrusted spec declares, up to three
// and then "and N more", and reads "the spec's gates" when the names would
// carry it past lineColumns; the line under it says why and the step that
// counts them, and every line fits, a 48-byte id (the spec's longest)
// included. Break: drop the short form, or name every gate however many.
func TestSUF1b_TheNoGateLineNamesUntrustedGatesWithinTheLine(t *testing.T) {
	const spec = ".claude/rashomon/spec.json"
	gates := func(ids ...string) []verify.UntrustedGate {
		var out []verify.UntrustedGate
		for _, id := range ids {
			out = append(out, verify.UntrustedGate{ID: "spec:" + id, Spec: spec})
		}
		return out
	}
	for _, c := range []struct {
		name  string
		gates []verify.UntrustedGate
		want  string
	}{
		{"one", gates("unit"), "2 files edited; 'unit' does not count"},
		{"two", gates("unit", "vet"), "2 files edited; 'unit' and 'vet' do not count"},
		{"four", gates("a", "b", "c", "d"), "2 files edited; 'a', 'b', 'c' and 1 more do not count"},
		{"a 48-byte id", gates(strings.Repeat("u", 48)), "2 files edited; the spec's gates do not count"},
		{"a 24-rune id beside another", gates("unit-tests-of-the-module", "vet"), "2 files edited; 'unit-tests-of-the-module' and 'vet' do not count"},
	} {
		t.Run(c.name, func(t *testing.T) {
			v := viewOf(verify.RowNoGate, wrote)
			v.Untrusted = c.gates
			got := ValueLine(v, Impact{}, "sess-1", false)
			if want := prefix + c.want + ".\n" + indent + "The spec is not trusted; review it, then run rashomon spec trust.\n"; !strings.HasPrefix(got, want) {
				t.Errorf("ValueLine =\n%s\nwant it to open\n%s", got, want)
			}
			for _, l := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
				if columns(l) > lineColumns {
					t.Errorf("a line is %d columns, want at most %d: %q", columns(l), lineColumns, l)
				}
			}
		})
	}
}
