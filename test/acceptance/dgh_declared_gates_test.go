package acceptance

// DGH: the shipped binary against a trusted spec (cards DECLARED-GATES1,
// STOP-NAMES-GATE1, D222).
//
// `rashomon spec trust` needs a person at a terminal and refuses an agent
// session, so no test here could run it. The test process is not the shipped
// binary, so it writes what the verb writes: the bindings file and the trust
// key, through the same exported API the verb ends in (spec.Bindings.Bind,
// hook.WriteSpecBindings: the key made as the verb makes it, the MAC
// spec.EncodeBindings signs). The binary then reads that file as it reads a
// real trust. The binary gains no bypass; this file is test code only.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

const dghSpec = `{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]}]}`

// dghProject is a git project whose repository spec is doc, in a directory
// of its own, and the spec's path.
func dghProject(t *testing.T, doc string) (dir, path string) {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, ".claude", "rashomon", "spec.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

// trust binds the spec at path in this machine's store, as `rashomon spec
// trust` ends: Parse for the digest, Bind, WriteSpecBindings.
func (e *env) trust(path string) {
	e.t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatal(err)
	}
	_, digest, err := spec.Parse(data)
	if err != nil {
		e.t.Fatal(err)
	}
	b, err := spec.ReadBindings(spec.OSFS{}, e.home)
	if err != nil {
		b = &spec.Bindings{}
	}
	b.Bind(spec.Binding{ProjectRoot: hook.SpecProject(path), SpecPath: path, SpecDigest: digest, BoundAtMS: 1})
	if err := hook.WriteSpecBindings(e.home, b); err != nil {
		e.t.Fatal(err)
	}
}

// dghCall runs one call of the session in dir, hook then post, and returns
// the post's stdout.
func (e *env) dghCall(dir, id, tool string, input map[string]any) string {
	e.t.Helper()
	envs := []string{stAgent, "CI="}
	extra := map[string]any{"cwd": dir}
	if res := e.hook(stPayload(e.t, "PreToolUse", id, tool, input, extra), envs...); res.exitCode != 0 || res.stdout != "" {
		e.t.Fatalf("hook %s: exit %d, stdout %q, stderr %q", id, res.exitCode, res.stdout, res.stderr)
	}
	more := map[string]any{"cwd": dir, "tool_response": map[string]any{"stdout": "ok\n", "stderr": "", "interrupted": false}}
	res := e.post(stPayload(e.t, "PostToolUse", id, tool, input, more), envs...)
	if res.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	return res.stdout
}

// dghStop is the Stop line for the session, cwd dir.
func (e *env) dghStop(dir string) string {
	e.t.Helper()
	line, _ := e.recapLine(stopEvent("Stop", "Done.", dir), stAgent, "CI=")
	return line
}

func dghEdit(dir string) map[string]any {
	return map[string]any{"file_path": filepath.Join(dir, "internal", "a.go"), "old_string": "a", "new_string": "b"}
}

var dghRun = map[string]any{"command": "go test ./..."}

// dghEnv is a watched steer machine with the project trusted when trusted.
func dghEnv(t *testing.T, trusted bool) (*env, string) {
	t.Helper()
	e := newSteerEnv(t)
	dir, path := dghProject(t, dghSpec)
	if trusted {
		e.trust(path)
	}
	return e, dir
}

// TestDGH1_EditWithNoRunIsTheDoneLine: a trusted spec, an edit and no run of
// its gate: the Stop line is the done line, and names no gate.
func TestDGH1_EditWithNoRunIsTheDoneLine(t *testing.T) {
	e, dir := dghEnv(t, true)
	e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
	got := e.dghStop(dir)
	t.Logf("Stop line: %q", got)
	if strings.Contains(got, "verified") || strings.Contains(got, "'unit'") {
		t.Errorf("Stop line %q, want a line that is neither verified nor names 'unit'", got)
	}
	if got == "" {
		t.Error("Stop printed nothing, want the done line")
	}
}

// TestDGH2_EditThenGreenRunNamesTheGate: an edit then a green run of the
// declared gate: the Stop line names 'unit', not go (STOP-NAMES-GATE1).
func TestDGH2_EditThenGreenRunNamesTheGate(t *testing.T) {
	e, dir := dghEnv(t, true)
	e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
	e.dghCall(dir, "toolu_g", "Bash", dghRun)
	got := e.dghStop(dir)
	t.Logf("Stop line: %q", got)
	const want = "verified - 'unit' passed after the last edit."
	if !strings.Contains(got, want) {
		t.Errorf("Stop line %q, want it to contain %q", got, want)
	}
}

// dghStale runs a green run then an edit and returns the edit's post stdout
// and the text it carries to the agent or the person, "" when none.
func (e *env) dghStale(dir string) (stdout, text string) {
	e.t.Helper()
	e.dghCall(dir, "toolu_g", "Bash", dghRun)
	e.backdate("toolu_g", 120_000)
	stdout = e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
	var out struct {
		SystemMessage string `json:"systemMessage"`
		Hook          struct {
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if stdout != "" {
		if err := json.Unmarshal([]byte(stdout), &out); err != nil {
			e.t.Fatalf("post stdout %q is not JSON: %v", stdout, err)
		}
	}
	return stdout, out.Hook.Context + out.SystemMessage
}

// TestDGH3_GreenThenEditWithNoIndexNotesStaleEvidence (D222): no learned
// index, a green run of the declared gate, then an edit: the note names
// 'unit'.
func TestDGH3_GreenThenEditWithNoIndexNotesStaleEvidence(t *testing.T) {
	e, dir := dghEnv(t, true)
	stdout, text := e.dghStale(dir)
	t.Logf("post stdout: %s", stdout)
	if !strings.Contains(text, "gate 'unit' exited 0 earlier in this session (2 min ago); this call then wrote 'internal/a.go'. No later run of the gate is recorded.") {
		t.Errorf("post stdout %q, want the stale-evidence note naming 'unit'", stdout)
	}
}

// TestDGH4_ControlsUntrustedSpecAndStoreWrite: the same sessions with the
// spec not trusted, and with a store write before the run (D173, trust drops
// for the rest of the session), raise no note and name no gate.
func TestDGH4_ControlsUntrustedSpecAndStoreWrite(t *testing.T) {
	t.Run("untrusted", func(t *testing.T) {
		e, dir := dghEnv(t, false)
		if stdout, _ := e.dghStale(dir); stdout != "" {
			t.Errorf("untrusted spec: post printed %q, want nothing", stdout)
		}
		if got := e.dghStop(dir); strings.Contains(got, "'unit'") {
			t.Errorf("untrusted spec: Stop line %q names 'unit'", got)
		}
	})
	t.Run("untrusted, edit then run", func(t *testing.T) {
		e, dir := dghEnv(t, false)
		e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
		e.dghCall(dir, "toolu_g", "Bash", dghRun)
		got := e.dghStop(dir)
		t.Logf("Stop line: %q", got)
		if strings.Contains(got, "'unit'") || !strings.Contains(got, "verified - go test passed after the last edit") {
			t.Errorf("untrusted spec: Stop line %q, want the learned gate's name, go test, and no 'unit'", got)
		}
	})
	t.Run("store write drops trust", func(t *testing.T) {
		e, dir := dghEnv(t, true)
		e.dghCall(dir, "toolu_w", "Write", map[string]any{"file_path": filepath.Join(e.home, "note.txt"), "content": "x"})
		if stdout, _ := e.dghStale(dir); stdout != "" {
			t.Errorf("after a store write: post printed %q, want nothing", stdout)
		}
		e.dghCall(dir, "toolu_e2", "Edit", dghEdit(dir))
		e.dghCall(dir, "toolu_g2", "Bash", dghRun)
		got := e.dghStop(dir)
		t.Logf("Stop line: %q", got)
		if strings.Contains(got, "'unit'") || !strings.Contains(got, "go test passed after the last edit") {
			// The store write is also a record finding, so the row is not the verified one.
			t.Errorf("after a store write: Stop line %q, want go test and no 'unit'", got)
		}
	})
}

// dgh5Spec forbids a forced push and declares the unit gate.
const dgh5Spec = `{"rashomon_spec": 1,
 "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]}],
 "rules": [{"id": "no-force", "kind": "forbid_command", "command": {"argv": ["git", "push"], "flags": ["--force", "-f"]},
            "why": "history is shared", "hard_control": "branch protection"}]}`

// dghFailedCall runs one call of the session in dir whose tool failed with
// exit, hook then a PostToolUseFailure post, and returns the post's stdout.
func (e *env) dghFailedCall(dir, id, tool string, input map[string]any, exit int) string {
	e.t.Helper()
	envs := []string{stAgent, "CI="}
	if res := e.hook(stPayload(e.t, "PreToolUse", id, tool, input, map[string]any{"cwd": dir}), envs...); res.exitCode != 0 || res.stdout != "" {
		e.t.Fatalf("hook %s: exit %d, stdout %q, stderr %q", id, res.exitCode, res.stdout, res.stderr)
	}
	more := map[string]any{"cwd": dir, "error": "Exit code " + strconv.Itoa(exit), "is_interrupt": false, "duration_ms": 30}
	res := e.post(stPayload(e.t, "PostToolUseFailure", id, tool, input, more), envs...)
	if res.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	return res.stdout
}

// dghLedgerRow is the session's ledger row of spec rule id, `report --ledger`
// fields split, or nil with none.
func (e *env) dghLedgerRow(id string) []string {
	e.t.Helper()
	res := e.run("", nil, "report", "--ledger", "--session", testSession)
	if res.exitCode != 0 {
		e.t.Fatalf("report --ledger: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	for _, line := range strings.Split(res.stdout, "\n") {
		if f := strings.Fields(line); len(f) == 7 && f[2] == "spec_rule" && f[6] == id {
			return f
		}
	}
	e.t.Logf("ledger:\n%s", res.stdout)
	return nil
}

// TestDGH5_AForbiddenCallIsAViolationAtStop (card SPEC-RULES-STOP1, #173
// F1): a trusted forbid_command rule matched by `git push --force`. At
// PostToolUse the call raises; at PostToolUseFailure it raises nothing (D210
// (1)'s card stands). Either way the Stop line is not verified and the
// ledger's no-force row is unmet, citing the call.
func TestDGH5_AForbiddenCallIsAViolationAtStop(t *testing.T) {
	push := map[string]any{"command": "git push --force origin main"}
	for _, c := range []struct {
		name  string
		exit  int
		raise bool
	}{
		{"post", 0, true},
		{"failure", 128, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newSteerEnv(t)
			dir, path := dghProject(t, dgh5Spec)
			e.trust(path)
			e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
			var out string
			if c.exit == 0 {
				out = e.dghCall(dir, "toolu_p", "Bash", push)
			} else {
				out = e.dghFailedCall(dir, "toolu_p", "Bash", push, c.exit)
			}
			if got := strings.Contains(out, "matched spec rule no-force"); got != c.raise {
				t.Errorf("push post stdout %q: raised %v, want %v", out, got, c.raise)
			}
			e.dghCall(dir, "toolu_g", "Bash", dghRun)
			got := e.dghStop(dir)
			t.Logf("Stop line: %q", got)
			// A failed call's Stop prints the recorded-failure line in place
			// of the value line, so only the post case counts the finding.
			if strings.Contains(got, "verified - ") || c.raise && !strings.Contains(got, "1 record finding(s)") {
				t.Errorf("Stop line %q, want not verified (and, after a post, 1 record finding(s))", got)
			}
			if row := e.dghLedgerRow("spec:no-force"); row == nil || row[4] != "unmet" || row[5] == "-" {
				t.Errorf("ledger no-force row %v, want unmet citing the push", row)
			}
		})
	}
}
