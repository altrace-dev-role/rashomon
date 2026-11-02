package acceptance

// WR1: a test run launched through a wrapper -- make, npm, a script of the
// user's own -- is found by its output, read in memory at PostToolUse with
// gateout's parsers, and bound to the project's gate by the runner whose
// table read it (card WRAPPER-RUN1). PROVEN only: output no table reads
// leaves the gate unknown, never "not run".

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// wr1Call is one Bash call of a session: its command, what Claude Code
// returns for it, and, where failErr is set, the PostToolUseFailure error in
// place of a PostToolUse response.
type wr1Call struct {
	command, stdout, stderr, failErr string
}

// wr1Session runs one session of the project in dir: its SessionStart
// probe, an edit, each call, then the Stop, whose final message claims
// nothing. It returns the Stop line and the report's test_runs, as JSON.
func (e *env) wr1Session(dir, session string, calls ...wr1Call) (string, string) {
	e.t.Helper()
	if res := e.probe("start", session); res.exitCode != 0 {
		e.t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	envs := []string{stAgent, "CI="}
	pre := func(id, tool string, input map[string]any) map[string]any {
		extra := map[string]any{"cwd": dir, "session_id": session}
		if res := e.hook(stPayload(e.t, "PreToolUse", id, tool, input, extra), envs...); res.exitCode != 0 || res.stdout != "" {
			e.t.Fatalf("hook %s: exit %d, stdout %q, stderr %q", id, res.exitCode, res.stdout, res.stderr)
		}
		return extra
	}
	edit := dghEdit(dir)
	extra := pre("toolu_e", "Edit", edit)
	extra["tool_response"] = map[string]any{"stdout": "ok\n", "stderr": "", "interrupted": false}
	if res := e.post(stPayload(e.t, "PostToolUse", "toolu_e", "Edit", edit, extra), envs...); res.exitCode != 0 {
		e.t.Fatalf("post edit: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	for i, c := range calls {
		id := "toolu_w" + string(rune('0'+i))
		input := map[string]any{"command": c.command}
		extra := pre(id, "Bash", input)
		event := "PostToolUse"
		if c.failErr != "" {
			event = "PostToolUseFailure"
			extra["error"], extra["is_interrupt"], extra["tool_response"] = c.failErr, false, c.failErr
		} else {
			extra["tool_response"] = map[string]any{"stdout": c.stdout, "stderr": c.stderr, "interrupted": false}
		}
		if res := e.post(stPayload(e.t, event, id, "Bash", input, extra), envs...); res.exitCode != 0 {
			e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
		}
	}
	b, err := json.Marshal(map[string]any{"hook_event_name": "Stop", "session_id": session, "cwd": dir,
		"transcript_path": "/tmp/transcripts/" + session + ".jsonl", "stop_hook_active": false,
		"last_assistant_message": "I changed internal/a.go."})
	if err != nil {
		e.t.Fatal(err)
	}
	line, _ := e.recapLine(string(b), stAgent, "CI=")
	res := e.run("", nil, "report", "--json", "--session", session)
	if res.exitCode != 0 {
		e.t.Fatalf("report: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	var rep struct {
		Sessions []struct {
			TestRuns json.RawMessage `json:"test_runs"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &rep); err != nil || len(rep.Sessions) != 1 {
		e.t.Fatalf("report: %v, %d sessions\n%s", err, len(rep.Sessions), res.stdout)
	}
	e.t.Logf("Stop line (%s): %q; test_runs %s", session, line, rep.Sessions[0].TestRuns)
	return line, string(rep.Sessions[0].TestRuns)
}

const (
	wr1GoPass = "ok  \tm\t0.004s\n"
	wr1GoFail = "--- FAIL: TestA (0.00s)\n    a_test.go:9: got 5, want 4\nFAIL\nFAIL\tm\t0.004s\nFAIL\n"
)

// wr1Runs is the report's test_runs: the runs, those ok and failed, and
// those found by their output in a call no runner named.
type wr1Runs struct {
	Runs    int `json:"runs"`
	OK      int `json:"ok"`
	Failed  int `json:"failed"`
	Wrapped int `json:"wrapped"`
}

// TestWR1_AWrappedRunIsFoundByItsOutput: a go.mod project, an edit, then a
// test run through a wrapper. A run whose output carries go test's own
// summary binds the inferred gate, and the Stop says it ran in a wrapper and
// passed, or failed; an output no table reads leaves the gate unknown, never
// "not run". The capture of 11-01 (se3/wr1-capture.log) printed "go test
// (inferred) not run after the last edit" for every wrapped case here.
// Controls: a direct go test, and a direct run after an unread script, stay
// verified; a log read with cat is no run; a bats run, whose "ok 1" lines
// begin as go's do, is not go's, through a script or through make. Break:
// read no output of a class-execute call, bind a wrapped run to no gate, or
// let an unread wrapper leave the gate "not run".
//
// Where the script exits 1 (PostToolUseFailure) the turn has a recorded
// failure, and that line speaks first: the value line, which holds the
// wrapper clause, speaks only when recap.Line has no sentence (card 2.4 R12),
// as for a direct go test that fails. Those rows prove the read by the word
// the script's record keeps (failureEventKept) and by test_runs, and still
// deny "not run"; the clause itself is the PostToolUse rows'.
func TestWR1_AWrappedRunIsFoundByItsOutput(t *testing.T) {
	const (
		verified        = "verified - "
		wrapped         = "go test (inferred) ran in a wrapper (call "
		passed          = ") and passed."
		failed          = "go test (inferred) failed in a wrapper (call "
		unknown         = "go test (inferred) unknown: call "
		notRun          = "not run"
		recordedFailure = "rashomon: 1 recorded failure."
	)
	// failureEventKept is the gate_output kind the script's execution record
	// keeps, by row, for the rows whose script exits 1; parser go in each.
	failureEventKept := map[string]string{
		"script, go red, failure event":             "summary_fail",
		"script, go passed, then the script failed": "summary_pass",
	}
	for _, c := range []struct {
		name       string
		calls      []wr1Call
		want, deny []string
		runs       wr1Runs
	}{
		{"control: direct go test, green", []wr1Call{{command: "go test ./...", stdout: wr1GoPass}},
			[]string{"verified - go test (inferred) passed after the last edit."}, []string{notRun}, wr1Runs{1, 1, 0, 0}},
		{"script, go green", []wr1Call{{command: "bash scripts/test.sh", stdout: wr1GoPass}},
			[]string{verified, wrapped, passed}, []string{notRun}, wr1Runs{1, 1, 0, 1}},
		{"goslot-shaped, go green (B279)", []wr1Call{{command: "bash /x/slot/goslot.sh rv-qa3 /x/wt bash /tmp/s.sh",
			stdout: "goslot: rv-qa3 START\n" + wr1GoPass + "goslot: rv-qa3 END exit=0\n"}},
			[]string{verified, wrapped, passed}, []string{notRun}, wr1Runs{1, 1, 0, 1}},
		{"make test, go green", []wr1Call{{command: "make test", stdout: "go test ./...\n" + wr1GoPass}},
			[]string{verified, wrapped, passed}, []string{notRun}, wr1Runs{1, 1, 0, 0}},
		{"npm test, go green", []wr1Call{{command: "npm test", stdout: "\n> m@1.0.0 test\n> go test ./...\n\n" + wr1GoPass}},
			[]string{verified, wrapped, passed}, []string{notRun}, wr1Runs{1, 1, 0, 0}},
		{"script, go red, piped", []wr1Call{{command: "bash scripts/test.sh | tail -20", stdout: wr1GoFail}},
			[]string{failed}, []string{verified, notRun}, wr1Runs{1, 0, 1, 1}},
		{"script, go red, failure event", []wr1Call{{command: "bash scripts/test.sh", failErr: "Exit code 1\n" + wr1GoFail}},
			[]string{recordedFailure}, []string{verified, notRun}, wr1Runs{1, 0, 1, 1}},
		// Added with the fix: go's summary says ok and the script exits 1
		// after it (a lint step, say). Neither proven: unknown, and no run
		// counted ok (report.TestRuns.Wrapped: a passed summary beside a
		// failed call is no run), while the record keeps the word it read.
		{"script, go passed, then the script failed", []wr1Call{{command: "bash scripts/test.sh", failErr: "Exit code 1\n" + wr1GoPass}},
			[]string{recordedFailure}, []string{verified, notRun, "failed in a wrapper"}, wr1Runs{0, 0, 0, 0}},
		{"script, output no table reads", []wr1Call{{command: "bash scripts/test.sh", stdout: "done\n"}},
			[]string{unknown}, []string{verified, notRun}, wr1Runs{0, 0, 0, 0}},
		{"script, a bats run is not go's", []wr1Call{{command: "bash scripts/test.sh", stdout: "1..2\nok 1 adds\nok 2 subtracts\n"}},
			[]string{unknown}, []string{verified, notRun}, wr1Runs{0, 0, 0, 0}},
		{"make test, a bats run is not go's", []wr1Call{{command: "make test", stdout: "bats test\n1..2\nok 1 adds\nok 2 subtracts\n"}},
			[]string{unknown}, []string{"go test (inferred) ran", notRun}, wr1Runs{1, 1, 0, 0}},
		{"control: script unread, then direct go test green", []wr1Call{{command: "bash scripts/test.sh", stdout: "done\n"},
			{command: "go test ./...", stdout: wr1GoPass}},
			[]string{"verified - go test (inferred) passed after the last edit."}, []string{notRun, unknown}, wr1Runs{1, 1, 0, 0}},
		{"control: a log read with cat is no run", []wr1Call{{command: "cat test.log", stdout: wr1GoPass}},
			[]string{"go test (inferred) not run after the last edit."}, []string{verified, unknown}, wr1Runs{0, 0, 0, 0}},
		// Added after the L1 replay of 11-01 22:3x: a line that starts with
		// date, echo or sleep is class execute, and its output was read; a
		// status check that greps an old job log ("date; ...; grep -E
		// 'FAIL|ok ' job.log", eng-4's transcript line 811) printed "go test
		// failed in a wrapper (call N)" though no go test ran in it. A log is
		// not a run whatever starts the line.
		{"control: a log grepped after date is no run", []wr1Call{{command: "date; grep -E 'FAIL|ok ' job.log", stdout: wr1GoFail}},
			[]string{"go test (inferred) not run after the last edit."}, []string{verified, unknown, "failed in a wrapper"}, wr1Runs{0, 0, 0, 0}},
		{"control: a log tailed after echo is no run", []wr1Call{{command: "echo checking; tail -5 job.log", stdout: wr1GoPass}},
			[]string{"go test (inferred) not run after the last edit."}, []string{verified, unknown, wrapped}, wr1Runs{0, 0, 0, 0}},
		{"control: a log read after sleep is no run", []wr1Call{{command: "sleep 5; cat job.log", stdout: wr1GoPass}},
			[]string{"go test (inferred) not run after the last edit."}, []string{verified, unknown, wrapped}, wr1Runs{0, 0, 0, 0}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newSteerEnv(t)
			line, raw := e.wr1Session(gi1Project(t, map[string]string{"go.mod": "module m\n"}), "sess-wr1", c.calls...)
			gb1Columns(t, line)
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
			if got != c.runs {
				t.Errorf("test_runs %+v, want %+v", got, c.runs)
			}
			if kind, ok := failureEventKept[c.name]; ok {
				var found bool
				for _, r := range e.records("sess-wr1") {
					if r.typ() != "execution" || r.str("tool_use_id") != "toolu_w0" {
						continue
					}
					found = true
					k, _ := nested(r, "gate_output.kind")
					p, _ := nested(r, "gate_output.parser")
					if k != kind || p != "go" {
						t.Errorf("the script's record keeps gate_output %v by %v, want %s by go", k, p, kind)
					}
				}
				if !found {
					t.Error("no execution record for the script's call toolu_w0")
				}
			}
		})
	}
}

// wr1DeclaredCall runs one Bash call of the session in dir, hook then post,
// as dghCall does, with the response given: stdout at PostToolUse, or, where
// failErr is set, a PostToolUseFailure carrying it. It returns the post's
// stdout.
func (e *env) wr1DeclaredCall(dir, id, command, stdout, failErr string) string {
	e.t.Helper()
	envs := []string{stAgent, "CI="}
	input := map[string]any{"command": command}
	if res := e.hook(stPayload(e.t, "PreToolUse", id, "Bash", input, map[string]any{"cwd": dir}), envs...); res.exitCode != 0 || res.stdout != "" {
		e.t.Fatalf("hook %s: exit %d, stdout %q, stderr %q", id, res.exitCode, res.stdout, res.stderr)
	}
	event, more := "PostToolUse", map[string]any{"cwd": dir, "tool_response": map[string]any{"stdout": stdout, "stderr": "", "interrupted": false}}
	if failErr != "" {
		event, more = "PostToolUseFailure", map[string]any{"cwd": dir, "error": failErr, "is_interrupt": false, "tool_response": failErr}
	}
	res := e.post(stPayload(e.t, event, id, "Bash", input, more), envs...)
	if res.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	return res.stdout
}

// TestWR1_AWrappedRunRaisesNoNoteThatNamesAPipe: under a trusted spec's
// gate (go test ./...), a make run whose output go test's table reads is
// the gate's run, read from its output (WRAPPER-RUN1). No note row says so:
// regression-location's and stale-evidence's output rows say the pipe hid
// the exit status, and no pipe did; their exit rows would give go test the
// exit make returned. So a wrapped run raises neither note, until a row
// says it ran in a wrapper (WRAPPER-RED-STEP1). Controls: the same sessions
// run go test directly, and each raises its note. Break: let a wrapped red
// or green reach either producer.
func TestWR1_AWrappedRunRaisesNoNoteThatNamesAPipe(t *testing.T) {
	const makeFail = "Exit code 2\ngo test ./...\n" + wr1GoFail + "make: *** [check] Error 1\n"
	count := func(e *env, p pattern.ID) int {
		n := 0
		for _, l := range e.steerLines() {
			if l.str("pattern") == string(p) {
				n++
			}
		}
		return n
	}
	for _, c := range []struct {
		name           string
		command, red   string
		redOut, redErr string
		want           int
	}{
		{"control: go test red", "go test ./...", "go test ./...", "", "Exit code 1\n" + wr1GoFail, 1},
		{"make check red", "go test ./...", "make check", "", makeFail, 0},
	} {
		t.Run("regression-location, "+c.name, func(t *testing.T) {
			e, dir := dghEnv(t, true)
			e.wr1DeclaredCall(dir, "toolu_g", c.command, wr1GoPass, "")
			e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
			out := e.wr1DeclaredCall(dir, "toolu_r", c.red, c.redOut, c.redErr)
			if strings.Contains(out, "pipe hid") {
				t.Errorf("the red's post printed %q", out)
			}
			if n := count(e, pattern.RegressionLocation); n != c.want {
				t.Errorf("%d regression-location lines, want %d", n, c.want)
			}
		})
	}
	for _, c := range []struct {
		name, command, stdout string
		want                  int
	}{
		{"control: go test green", "go test ./...", wr1GoPass, 1},
		{"make test green", "make test", "go test ./...\n" + wr1GoPass, 0},
	} {
		t.Run("stale-evidence, "+c.name, func(t *testing.T) {
			e, dir := dghEnv(t, true)
			e.wr1DeclaredCall(dir, "toolu_g", c.command, c.stdout, "")
			e.backdate("toolu_g", 120_000)
			if out := e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir)); strings.Contains(out, "pipe hid") {
				t.Errorf("the edit's post printed %q", out)
			}
			if n := count(e, pattern.StaleEvidence); n != c.want {
				t.Errorf("%d stale-evidence lines, want %d", n, c.want)
			}
		})
	}
}
