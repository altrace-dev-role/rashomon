package report

import (
	"strconv"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Issue #52: a failed Bash call that only found a program missing is set
// aside as a lookup is: a call whose declaration carries the probe bit (v10),
// a line of nothing but which, type and command -v or -V probes read from the
// words at the hook, or a call the shell ended with 127, command not found,
// which the execution holds at any version. A call with no bit counts, since
// its line held another command, whose failure may be the line's, or its
// record predates the bit: `which pytest && pytest -q` exits 1 when a test
// failed. The call stays in Failed either way. Break: set aside every failed
// call, drop the 127 reading, read the bit off a record with no declaration,
// or count the set-aside.
func TestSilentFailures_AMissingProgramIsSetAside(t *testing.T) {
	one, two, nf := 1, 2, 127
	for _, tc := range []struct {
		name  string
		decl  bool
		probe bool
		exit  *int
		aside bool
	}{
		{"a probe, exit 1", true, true, &one, true},
		{"a probe, no exit read", true, true, nil, true},
		{"no probe, exit 127", true, false, &nf, true},
		{"no declaration, exit 127", false, false, &nf, true},
		{"no probe, exit 1", true, false, &one, false},
		{"no probe, exit 2", true, false, &two, false},
		{"no probe, no exit read", true, false, nil, false},
		{"no declaration, exit 1", false, false, &one, false},
	} {
		path := transcript(t, "All done.")
		id := "x1"
		if !tc.decl {
			id = "x2"
		}
		run := &store.Run{
			Declarations: []store.Declaration{{ToolUseID: id, ToolName: "Bash", TranscriptPath: path, Probe: tc.probe}},
			Executions:   []store.Execution{{ToolUseID: "x1", ToolName: "Bash", Outcome: store.ExecFailed, ExitCode: tc.exit}},
		}
		sf := BuildSilentFailures(run, accountFromDisk(run))
		code := "null"
		if tc.exit != nil {
			code = strconv.Itoa(*tc.exit)
		}
		if aside := sf.Failed-sf.Counted() == 1; sf.Failed != 1 || aside != tc.aside || sf.Fires == tc.aside {
			t.Errorf("%s (exit %s): failed/counted/fires = %d/%d/%v, want set aside %v (fires %v)",
				tc.name, code, sf.Failed, sf.Counted(), sf.Fires, tc.aside, !tc.aside)
		}
	}
}
