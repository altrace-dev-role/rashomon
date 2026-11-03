package shape

import (
	"encoding/json"
	"testing"
)

// TestLA1_ALaunchIsReadInEveryCommand: a Bash line launches where any of its
// commands is a script named by its extension and not asked to report, or a
// shell that runs a script, its standard input, or a -c line that launches
// or names a runner (card LAUNCH-ANY1). The lines are R67's shapes and the
// manager's controls. Break: read the first command only, stop at a stage of
// assignments alone, launch on -n, on -o noexec, on a report word or on any
// -c line, drop the -c recursion, let the byte scan drop a launch, or let
// Commands read past assignments for every caller.
func TestLA1_ALaunchIsReadInEveryCommand(t *testing.T) {
	for _, c := range []struct {
		command string
		want    bool
	}{
		{"./run-tests.sh", true},
		{"mkdir -p out && ./run-tests.sh", true},
		{"chmod +x x.sh && GOSLOT_WAIT=1 ~/slot/goslot.sh rv-se2 ~/wt bash ~/job.sh > job.log 2>&1", true},
		{"d=~/e; chmod +x $d/x.sh; df -h / | tail -1; sysctl vm.swapusage; GOSLOT_WAIT=3600 ~/slot/goslot.sh rv-se2 ~/wt $d/x.sh > $d/x.txt 2>&1", true},
		{"cd ~/wt && mkdir -p ~/e && echo \"start $(date +%H:%M:%S)\" && GOSLOT_WAIT=3600 ~/slot/goslot.sh rv-se2 ~/wt go test ./x -count=1 2>&1 | tee ~/e/r.txt | tail -40", true},
		{"until grep -q '^goslot exit=' /tmp/g.log 2>/dev/null; do sleep 30; done; GOSLOT_WAIT=3600 ~/slot/goslot.sh rv-se3 ~/wt bash /tmp/s.sh", true},
		{"bash -x ./t.sh", true},
		{"bash -c './run-tests.sh'", true},
		{"bash -lc 'cd sub && ./run-tests.sh'", true},
		{"sh -c 'go test ./...'", true},
		{"bash -o pipefail -c 'make test | tail'", true},
		{"sh -c 'bash -c \"./t.sh\"'", true},
		{"bash", true},
		{"./t.sh build", true},
		{"bash -c 'echo hi'", false},
		{"sh -c", false},
		{"bash -n x.sh", false},
		{"bash -n x.sh && echo syntax-ok; df -h / | tail -1; sysctl -n vm.swapusage", false},
		{"bash -o noexec x.sh", false},
		{"bash --version", false},
		{"~/slot/goslot.sh status 2>&1 | head -20; date '+%H:%M:%S'", false},
		{"./t.sh --help", false},
		{"./t.sh -h", false},
		{"bash ./t.sh version", false},
		{"chmod +x x.sh", false},
		{"cat ~/slot/goslot.sh; sed -n 1,40p ~/slot/goslot.sh", false},
		{"S=~/slot/goslot.sh; $S status", false},
		{"timeout 60 ./t.sh", false},
		{"env -i PATH=/usr/bin bash ./t.sh", false},
		{"git push && shasum -a 256 x", false},
		{"go test ./...", false},
	} {
		if got := launches(c.command); got != c.want {
			t.Errorf("launches(%q) = %v, want %v", c.command, got, c.want)
		}
		in, err := json.Marshal(map[string]string{"command": c.command})
		if err != nil {
			t.Fatal(err)
		}
		if s := Derive("Bash", in, []byte("k")); s.Launch != c.want {
			t.Errorf("Derive(%q).Launch = %v, want %v", c.command, s.Launch, c.want)
		}
	}
	if s := Derive("Write", json.RawMessage(`{"file_path":"/x/t.sh","content":"./t.sh"}`), []byte("k")); s.Launch {
		t.Error("a Write call launches")
	}
	// Commands itself still stops at a stage of assignments alone: only the
	// launch rule reads past one, so the rules reading Commands are as before.
	if cmds, ok := Commands("d=~/e; ./t.sh"); ok || len(cmds) != 0 {
		t.Errorf("Commands(d=~/e; ./t.sh) = %v, %v; want no command and a stop", cmds, ok)
	}
}

// TestLA1_ARecordBeforeTheBitIsReadByItsProgram: a record of call schema 10
// or later is read by its launch bit, whatever its program; one before it by
// its first program, as its writer read it. Break: read the bit on an older
// record (absent, so false), or the program on a newer one.
func TestLA1_ARecordBeforeTheBitIsReadByItsProgram(t *testing.T) {
	sh, mk := "goslot.sh", "mkdir"
	for _, c := range []struct {
		name             string
		program          *string
		launch, recorded bool
		want             bool
	}{
		{"recorded, a status call of a script", &sh, false, true, false},
		{"recorded, a launch past mkdir", &mk, true, true, true},
		{"before the bit, a script", &sh, false, false, true},
		{"before the bit, mkdir", &mk, false, false, false},
		{"before the bit, no program", nil, false, false, false},
	} {
		if got := RecordedLaunch(c.program, c.launch, c.recorded); got != c.want {
			t.Errorf("%s: RecordedLaunch = %v, want %v", c.name, got, c.want)
		}
	}
}
