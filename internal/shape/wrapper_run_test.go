package shape

import (
	"encoding/json"
	"testing"
)

// TestWR1_AWrapperRunStartsWithAShellOrAScript: a Bash line's output is read
// as a wrapper's only where its program is a shell or a script named by its
// extension, past a leading cd and assignments. A status check that starts
// with date, echo or sleep and reads an old log is not a run (the L1 replay
// of 11-01: "date; ...; grep -E 'FAIL|ok ' job.log" bound as go's red).
// Break: read every class-execute line, or drop the extension rule.
func TestWR1_AWrapperRunStartsWithAShellOrAScript(t *testing.T) {
	for _, c := range []struct {
		command string
		want    bool
	}{
		{"bash scripts/test.sh", true},
		{"zsh run.zsh", true},
		{"./scripts/test.sh", true},
		{"cd /repo && bash scripts/test.sh", true},
		{"GOSLOT_WAIT=3600 /x/slot/goslot.sh rv-qa3 /x/wt bash /tmp/s.sh", true},
		{"/x/bin/check.bash", true},
		{"date; cat rate-limits.json; echo; grep -E 'FAIL|ok ' job.log", false},
		{"echo checking; tail -5 job.log", false},
		{"sleep 5; cat job.log", false},
		{"python3 tools/report.py", false},
		{"./run-tests", false},
		{"timeout 600 bash scripts/test.sh", false},
		{"cat job.log", false},
		{"go test ./...", false},
		{"make test", false},
	} {
		in, err := json.Marshal(map[string]string{"command": c.command})
		if err != nil {
			t.Fatal(err)
		}
		s := Derive("Bash", in, []byte("k"))
		if got := WrapperRun("Bash", s.Launch, s.RunnerDigest, s.StatusMasked, s.VerbClass); got != c.want {
			t.Errorf("%q (program %v, class %s): wrapper %v, want %v", c.command, deref(s.Program), s.VerbClass, got, c.want)
		}
		// A record before call schema 10 has no launch bit, and is read by
		// its first program, which on each of these lines says the same.
		if got := WrapperRun("Bash", RecordedLaunch(s.Program, false, false), s.RunnerDigest, s.StatusMasked, s.VerbClass); got != c.want {
			t.Errorf("%q (program %v) read as a record before the bit: wrapper %v, want %v", c.command, deref(s.Program), got, c.want)
		}
	}
	if WrapperRun("Bash", RecordedLaunch(nil, false, false), nil, nil, VerbExecute) {
		t.Error("a line whose program is unknown is read as a wrapper's")
	}
	if WrapperRun("Write", true, nil, nil, VerbExecute) {
		t.Error("a Write call is read as a wrapper's")
	}
}
