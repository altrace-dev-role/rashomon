package acceptance

// LA1: a call that may have run a gate is one that launches something, in
// any command of its line (card LAUNCH-ANY1, the manager's ruling of 11-03):
// a script, or a shell running a script or a line, never a syntax check, a
// script asked to report, or a line whose launch is not its first command
// read as no launch at all. The record keeps a line's first program only, so
// `mkdir -p out && ./run-tests.sh` read as mkdir, no wrapper, and its run was
// "not run"; `goslot.sh status` and `bash -n x.sh` read as wrappers, and a
// line named them as the call the gate may have run in. In R67 at main
// e4bb97ee: 28 false "not run after the last edit" lines of the first kind,
// and 4 NR lines naming a call of the second.

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestLA1_ALaunchAnywhereInTheLineIsAWrapperRun: a go.mod project, an edit,
// then one call. A script launched past the line's first command is a
// wrapper run, read by its output like one launched first; a shell's -c line
// that launches is one too. Controls: a syntax check, a script asked for its
// status or its help, a chmod with no launch after it, and a shell's -c line
// that launches nothing are no run, and the gate stays "not run". Break:
// read only a line's first command, launch on -n, on a report word or on any
// -c line, or launch on chmod's script argument; or count a launch's green
// as no wrapped run in the report.
func TestLA1_ALaunchAnywhereInTheLineIsAWrapperRun(t *testing.T) {
	const (
		verified = "verified - "
		wrapped  = "go test (inferred) ran in a wrapper (call "
		passed   = ") and passed."
		unknown  = "go test (inferred) unknown: call "
		notRun   = "go test (inferred) not run after the last edit."
	)
	for _, c := range []struct {
		name       string
		calls      []wr1Call
		want, deny []string
		wrappedRun int
	}{
		{"mkdir, then a script, go green", []wr1Call{{command: "mkdir -p out && ./run-tests.sh", stdout: wr1GoPass}},
			[]string{verified, wrapped, passed}, []string{"not run"}, 1},
		{"chmod, then the script, go green", []wr1Call{{command: "chmod +x scripts/test.sh && bash scripts/test.sh", stdout: wr1GoPass}},
			[]string{verified, wrapped, passed}, []string{"not run"}, 1},
		{"a goslot launch past a prelude, go green", []wr1Call{{command: "d=/x/e; chmod +x $d/s.sh; df -h / | tail -1; GOSLOT_WAIT=1 /x/slot/goslot.sh rv-qa3 /x/wt bash /tmp/s.sh > /tmp/s.log 2>&1; cat /tmp/s.log",
			stdout: "goslot: rv-qa3 START\n" + wr1GoPass + "goslot: rv-qa3 END exit=0\n"}},
			[]string{verified, wrapped, passed}, []string{"not run"}, 1},
		{"chmod, then the script, output no table reads", []wr1Call{{command: "chmod +x scripts/test.sh && bash scripts/test.sh", stdout: "done\n"}},
			[]string{unknown}, []string{verified, "not run"}, 0},
		{"a shell's -c line that launches a script", []wr1Call{{command: "bash -c 'cd sub && ./run-tests.sh'", stdout: wr1GoPass}},
			[]string{verified, wrapped, passed}, []string{"not run"}, 1},
		{"control: a shell's -c line that launches nothing", []wr1Call{{command: "bash -c 'echo hi'", stdout: "hi\n"}},
			[]string{notRun}, []string{verified, unknown}, 0},
		{"control: a syntax check is no run", []wr1Call{{command: "bash -n scripts/test.sh && echo syntax-ok", stdout: "syntax-ok\n"}},
			[]string{notRun}, []string{verified, unknown}, 0},
		{"control: a script asked for its status is no run", []wr1Call{{command: "/x/slot/goslot.sh status 2>&1 | head -20; date", stdout: "holder: none\nqueue:\n12:00:00\n"}},
			[]string{notRun}, []string{verified, unknown}, 0},
		{"control: a script asked for its help is no run", []wr1Call{{command: "./scripts/test.sh --help", stdout: "usage: test.sh [pkg]\n"}},
			[]string{notRun}, []string{verified, unknown}, 0},
		{"control: chmod alone is no run", []wr1Call{{command: "chmod +x scripts/test.sh", stdout: ""}},
			[]string{notRun}, []string{verified, unknown}, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newSteerEnv(t)
			line, raw := e.wr1Session(gi1Project(t, map[string]string{"go.mod": "module m\n"}), "sess-la1", c.calls...)
			for _, w := range c.want {
				if !strings.Contains(line, w) {
					t.Errorf("Stop line %q, want it to contain %q", line, w)
				}
			}
			for _, d := range c.deny {
				if strings.Contains(line, d) {
					t.Errorf("Stop line %q, want no %q", line, d)
				}
			}
			var got wr1Runs
			if err := json.Unmarshal([]byte(raw), &got); err != nil {
				t.Fatalf("test_runs %s: %v", raw, err)
			}
			if got.Wrapped != c.wrappedRun {
				t.Errorf("test_runs %s: wrapped %d, want %d", raw, got.Wrapped, c.wrappedRun)
			}
		})
	}
}

// la1Launch is a background launch of the project's tests whose launch is
// not the line's first command, as R67's goslot lines were.
var la1Launch = map[string]any{"command": "df -h / | tail -1; GOSLOT_WAIT=1 /x/slot/goslot.sh rv-se4 /x/wt bash scripts/test.sh > /tmp/la1.log 2>&1", "run_in_background": true}

// TestLA1_ALineNamesTheLaunchNotALaterCheck: a trusted spec, an edit, a
// background launch of the tests, then, later, a foreground call that ran
// no test: a syntax check of the script, or the slot's status. The done
// claim's line names the background launch, never the later call (R67 at
// main e4bb97ee: eng-3 2177 and eng-1 9509 named a `bash -n`, eng-2 2570
// and eng-3 1535 a `goslot.sh status`). Where the launch ran from another
// directory (eng-3 1535's ran from ~) it is still the launch. Break: launch
// on -n or on a report word, or read only a line's first command.
func TestLA1_ALineNamesTheLaunchNotALaterCheck(t *testing.T) {
	for _, c := range []struct {
		name          string
		launch, later map[string]any
		elsewhere     bool
	}{
		{"a syntax check after a script's launch", map[string]any{"command": "bash scripts/test.sh", "run_in_background": true},
			map[string]any{"command": "bash -n scripts/test.sh && echo syntax-ok; df -h / | tail -1"}, false},
		{"the slot's status after a launch past mkdir", map[string]any{"command": "mkdir -p /tmp/la1-ev && bash scripts/test.sh", "run_in_background": true},
			map[string]any{"command": "/x/slot/goslot.sh status | head -8; date +%H:%M:%S"}, false},
		{"the slot's status after a launch from another directory", la1Launch,
			map[string]any{"command": "/x/slot/goslot.sh status 2>&1 | head -20; date '+%H:%M:%S'"}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			e, dir := dghEnv(t, true)
			e.bgo2Call(dir, "p1", "toolu_e", "Edit", dghEdit(dir), map[string]any{"filePath": filepath.Join(dir, "internal", "a.go")})
			from := dir
			if c.elsewhere {
				from = t.TempDir()
			}
			e.bgo2Call(from, "p1", "toolu_bg", "Bash", c.launch, bgo2Bg)
			e.bgo2Call(dir, "p1", "toolu_c", "Bash", c.later, bgo2Quiet)
			line := e.dghStop(dir)
			if !bgo2Unknown.MatchString(line) {
				t.Errorf("Stop line %q, want %s", line, bgo2Unknown)
			}
			if strings.Contains(line, "names no run") || strings.Contains(line, "not run") {
				t.Errorf("Stop line %q names the later call, or says not run", line)
			}
		})
	}
}

// TestLA1_TheDeclarationRecordsALaunch: the declaration of a line whose
// launch is past its first command carries launch, true; a line that
// launches nothing omits the key, as location_unread is omitted (call schema
// 10). Break: drop the key, or write it false.
func TestLA1_TheDeclarationRecordsALaunch(t *testing.T) {
	e := newSteerEnv(t)
	e.wr1Session(gi1Project(t, map[string]string{"go.mod": "module m\n"}), "sess-la1r",
		wr1Call{command: "mkdir -p out && ./run-tests.sh", stdout: "done\n"},
		wr1Call{command: "/x/slot/goslot.sh status | head -8", stdout: "queue:\n"})
	var got []any
	for _, d := range e.declarations("sess-la1r") {
		if d.fields["tool_name"] != "Bash" {
			continue
		}
		v, ok := d.fields["launch"]
		if !ok {
			v = "absent"
		}
		got = append(got, v)
		if sv := d.fields["schema_version"]; sv != float64(10) {
			t.Errorf("declaration written at schema_version %v, want 10", sv)
		}
	}
	if len(got) != 2 || got[0] != true || got[1] != "absent" {
		t.Errorf("launch per Bash declaration %v, want [true absent]", got)
	}
}
