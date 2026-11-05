package verify

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
)

// TestGI2v_AMaskedNoteNamesItsGateByTheRunnersRow (card GATE-INFER2, DE R87):
// a masked run's note names its gate by the runner's row as the binary's
// lists write it, where the gate slot's one or two words refused a row of
// three words or with a flag and the note fell back to the program word
// ("gate 'python3'", "gate 'npx'"). A test row, and a check row, which the
// slot admits as well since membership is the whole list: these are the
// words the agent sees at the act, pinned. Control: a two-word row reads as
// before. Break: refuse a runner's row at the gate slot.
func TestGI2v_AMaskedNoteNamesItsGateByTheRunnersRow(t *testing.T) {
	tsc := headedVet(2, "head")
	tsc.prog, tsc.runner, tsc.kind, tsc.runnerName = "npx", "npx-tsc", shape.RunnerTypecheck, "npx tsc --noEmit"
	for _, tc := range []struct {
		name, edit string
		c          call
		gate       string
	}{
		{"a test row", "internal/a.py", tailedPytest(2, "head", ""), "python3 -m pytest"},
		{"a typecheck row", "web/a.ts", tsc, "npx tsc --noEmit"},
		{"control: a two-word row", "internal/a.go", headedVet(2, "head"), "go vet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := runOf(t, edit(1, tc.edit), tc.c)
			minuteClock(t, r)
			got := maskedFindings(postAt(t, r, 2, "t2"))
			want := "rashomon record: gate '" + tc.gate + "' ran in this call and the pipe to head hid its exit status; " +
				"no outcome of this run is recorded."
			if len(got) != 1 || got[0].Message.Text() != want {
				t.Errorf("candidates %d, note %q; want one saying %q", len(got), noteOf(got), want)
			}
		})
	}
}
