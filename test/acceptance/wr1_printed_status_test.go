package acceptance

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// wr1PrintedSession runs one session of the project in dir: its SessionStart
// probe, an edit, then one Bash call of command answered with stdout at
// PostToolUse, then the Stop, whose final message claims nothing. It returns
// the call's post stdout, the Stop's message and the call's seq.
func (e *env) wr1PrintedSession(dir, session, command, stdout string) (post, stop string, seq int) {
	e.t.Helper()
	if res := e.probe("start", session); res.exitCode != 0 {
		e.t.Fatalf("probe start: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	envs := []string{stAgent, "CI="}
	call := func(id, tool string, input map[string]any, out string) string {
		extra := map[string]any{"cwd": dir, "session_id": session}
		if res := e.hook(stPayload(e.t, "PreToolUse", id, tool, input, extra), envs...); res.exitCode != 0 || res.stdout != "" {
			e.t.Fatalf("hook %s: exit %d, stdout %q, stderr %q", id, res.exitCode, res.stdout, res.stderr)
		}
		extra["tool_response"] = map[string]any{"stdout": out, "stderr": "", "interrupted": false}
		res := e.post(stPayload(e.t, "PostToolUse", id, tool, input, extra), envs...)
		if res.exitCode != 0 {
			e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
		}
		return res.stdout
	}
	call("toolu_e", "Edit", dghEdit(dir), "ok\n")
	post = call("toolu_p", "Bash", map[string]any{"command": command}, stdout)
	b, err := json.Marshal(map[string]any{"hook_event_name": "Stop", "session_id": session, "cwd": dir,
		"transcript_path": "/tmp/transcripts/" + session + ".jsonl", "stop_hook_active": false,
		"last_assistant_message": "I changed internal/a.go."})
	if err != nil {
		e.t.Fatal(err)
	}
	stop, _ = e.recapLine(string(b), stAgent, "CI=")
	for _, r := range e.records(session) {
		if r.typ() == "declaration" && r.str("tool_use_id") == "toolu_p" {
			n, _ := r.fields["seq"].(float64)
			seq = int(n)
		}
	}
	if seq == 0 {
		e.t.Fatal("no declaration record for the call toolu_p")
	}
	e.t.Logf("post %q; Stop %q; call seq %d", post, stop, seq)
	return post, stop, seq
}

// TestWR1_APrintedStatusOutranksTheWrapperRead (#234, ruling (a)): a delegate
// line that hides its status and then prints it (`make check; echo "EXIT:
// $?"`) keeps the printed status as its outcome (STATUS-PROBE), over the go
// test summary make's output carried (WRAPPER-RUN1): the record holds one
// gate_output, and go's passing summary in it would read make green where
// make exited 2. go test ran inside make, so no line may say it did not run,
// nor that the call's output named no run: the record kept make's status and
// not the runs inside it, and the gate's line says so. Rows: the inferred
// gate (go.mod) and a trusted spec's 'unit', each with go passing and a later
// make step failing (make's red speaks) and with make passing (the gate is
// unknown). The call's post prints nothing. Break: read the printed status as
// an output that names no run, or bind go's summary over make's exit.
func TestWR1_APrintedStatusOutranksTheWrapperRead(t *testing.T) {
	const (
		command = "make check; echo \"EXIT: $?\""
		goOK    = "go test ./...\n" + wr1GoPass
		lintRed = "golangci-lint run ./...\ninternal/a.go:3:2: unused variable x\nmake: *** [check] Error 1\n"
	)
	inferred := func(t *testing.T) (*env, string) {
		return newSteerEnv(t), gi1Project(t, map[string]string{"go.mod": "module m\n"})
	}
	declared := func(t *testing.T) (*env, string) { return dghEnv(t, true) }
	for _, c := range []struct {
		name   string
		env    func(*testing.T) (*env, string)
		stdout string
		kind   string
		line   string // the value line, with %d for the call's seq
	}{
		{"inferred gate, go passed, then make failed", inferred, goOK + lintRed + "EXIT: 2\n", "printed_fail",
			"※ rashomon: 1 file edited; make failed after the last edit (call %d)."},
		{"inferred gate, make passed", inferred, goOK + "EXIT: 0\n", "printed_pass",
			"※ rashomon: go test (inferred) unknown: only call %d's exit status was read."},
		{"spec gate, go passed, then make failed", declared, goOK + lintRed + "EXIT: 2\n", "printed_fail",
			"※ rashomon: 1 file edited; make failed after the last edit (call %d)."},
		{"spec gate, make passed", declared, goOK + "EXIT: 0\n", "printed_pass",
			"※ rashomon: 'unit' unknown: only call %d's exit status was read."},
	} {
		t.Run(c.name, func(t *testing.T) {
			e, dir := c.env(t)
			post, stop, seq := e.wr1PrintedSession(dir, "sess-wr1p", command, c.stdout)
			if post != "" {
				t.Errorf("the call's post printed %q, want nothing", post)
			}
			gb1Columns(t, stop)
			want := strings.Replace(c.line, "%d", strconv.Itoa(seq), 1)
			if first, _, _ := strings.Cut(stop, "\n"); first != want {
				t.Errorf("Stop value line %q, want %q", first, want)
			}
			for _, d := range []string{"not run", "names no run", "in a wrapper", "test (inferred) passed", "'unit' passed"} {
				if strings.Contains(stop, d) {
					t.Errorf("Stop %q holds %q: go test ran inside make, and its result is not on the record", stop, d)
				}
			}
			var found bool
			for _, r := range e.executions("sess-wr1p") {
				if r.str("tool_use_id") != "toolu_p" {
					continue
				}
				found = true
				k, _ := nested(r, "gate_output.kind")
				p, _ := nested(r, "gate_output.parser")
				if k != c.kind || p != "exit_status" {
					t.Errorf("the call's record keeps gate_output %v by %v, want %s by exit_status", k, p, c.kind)
				}
			}
			if !found {
				t.Error("no execution record for the call toolu_p")
			}
		})
	}
}
