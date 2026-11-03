package acceptance

// BG-OUTCOME phase 2a (manager ruling 11-03 06:0x): the done flag judges the
// evidence since the last edit in the gate's project, across turns, not the
// turn's alone; and a background wrapper run after that edit reads unknown,
// naming the call, never "no test or build run". It says no more about the
// call's output than the record holds: no read clause where the hook knows
// neither the output's path nor a later read of it.

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// bgo2Call runs tool in dir under promptID: the hook, then a post with resp
// as its tool_response.
func (e *env) bgo2Call(dir, promptID, id, tool string, input, resp map[string]any) {
	e.t.Helper()
	envs := []string{stAgent, "CI="}
	extra := map[string]any{"cwd": dir, "prompt_id": promptID}
	if res := e.hook(stPayload(e.t, "PreToolUse", id, tool, input, extra), envs...); res.exitCode != 0 {
		e.t.Fatalf("hook %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
	more := map[string]any{"cwd": dir, "prompt_id": promptID, "tool_response": resp}
	if res := e.post(stPayload(e.t, "PostToolUse", id, tool, input, more), envs...); res.exitCode != 0 {
		e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
	}
}

var (
	bgo2Script = map[string]any{"command": "bash scripts/test.sh", "run_in_background": true}
	bgo2Bg     = map[string]any{"stdout": "", "stderr": "", "interrupted": false, "backgroundTaskId": "bgo2task"}
	bgo2Read   = map[string]any{"command": "cat /private/tmp/claude-501/bgo2/tasks/bgo2task.output"}
	bgo2Quiet  = map[string]any{"stdout": "ok\n", "stderr": "", "interrupted": false}
)

// bgo2First is turn p1 in a trusted project: an edit, then the project's
// tests launched in the background through a script, and the turn's Stop.
func bgo2First(t *testing.T) (*env, string) {
	t.Helper()
	e, dir := dghEnv(t, true)
	e.bgo2Call(dir, "p1", "toolu_e", "Edit", dghEdit(dir), map[string]any{"filePath": filepath.Join(dir, "internal", "a.go")})
	e.bgo2Call(dir, "p1", "toolu_bg", "Bash", bgo2Script, bgo2Bg)
	e.dghStop(dir)
	return e, dir
}

// bgo2Unknown is the background wrapper's sentence (the manager's ruling of
// 11-03 09:3x): the record cannot show the gate ran in the call, so it says
// the call may have run it, as the unread wrapper's hedge does.
var bgo2Unknown = regexp.MustCompile(`'unit' unknown: background call \d+ may have run it`)

// TestBGO2a_ALaterTurnsDoneClaimNamesTheBackgroundRun: turn p2 only reads the
// background job's output file and claims done. The edit and the launch are
// p1's, so the turn alone holds no edit and no run, and today the flag says
// "no test or build run is recorded for this turn", though the claim rests on
// the run p1 launched after the last edit. The done flag reads the evidence
// since that edit: the gate is unknown, and the line names the background
// call. The hook saw the read but cannot show what it held, so the line
// claims no read clause.
func TestBGO2a_ALaterTurnsDoneClaimNamesTheBackgroundRun(t *testing.T) {
	e, dir := bgo2First(t)
	e.bgo2Call(dir, "p2", "toolu_r", "Bash", bgo2Read, bgo2Quiet)
	line := e.dghStop(dir)
	if !bgo2Unknown.MatchString(line) {
		t.Errorf("a later turn's done claim after a background run: Stop line %q, want %s", line, bgo2Unknown)
	}
	for _, deny := range []string{"no test or build run", "not read", "not run"} {
		if strings.Contains(line, deny) {
			t.Errorf("Stop line %q says %q", line, deny)
		}
	}
}

// TestBGO2b_AnEarlierTurnsGreenAfterTheLastEditRaisesNoFlag: p1 edits and
// runs the gate in the foreground, green; p2 runs nothing and claims done.
// No flag: the green is after the last edit, whichever turn it ran in.
// Control: passes today (the claim's verdict reads the gate's state).
func TestBGO2b_AnEarlierTurnsGreenAfterTheLastEditRaisesNoFlag(t *testing.T) {
	e, dir := dghEnv(t, true)
	e.bgo2Call(dir, "p1", "toolu_e", "Edit", dghEdit(dir), map[string]any{"filePath": filepath.Join(dir, "internal", "a.go")})
	e.bgo2Call(dir, "p1", "toolu_t", "Bash", dghRun, map[string]any{"stdout": "ok  \texample.com/p\t0.01s\n", "stderr": "", "interrupted": false})
	e.dghStop(dir)
	e.bgo2Call(dir, "p2", "toolu_s", "Bash", map[string]any{"command": "git status"}, bgo2Quiet)
	if line := e.dghStop(dir); strings.Contains(line, "done was claimed") {
		t.Errorf("a done claim after an earlier turn's green after the last edit: Stop line %q, want no flag", line)
	}
}

// TestBGO2c_AnEditInAnotherWorktreeOpensNoWindow: p1 edits the project and
// launches its tests in the background; p2 edits a file in another
// directory, which no gate of the project covers, and claims done. The last
// edit in the gate's project is p1's, so the window opens there and the line
// names the background call. Break: open the window at any edit (p2's turn
// counts no covered write and reads "no test or build run").
func TestBGO2c_AnEditInAnotherWorktreeOpensNoWindow(t *testing.T) {
	e, dir := bgo2First(t)
	other := filepath.Join(t.TempDir(), "elsewhere.go")
	e.bgo2Call(dir, "p2", "toolu_o", "Edit", map[string]any{"file_path": other, "old_string": "a", "new_string": "b"},
		map[string]any{"filePath": other})
	line := e.dghStop(dir)
	if !bgo2Unknown.MatchString(line) || strings.Contains(line, "no test or build run") {
		t.Errorf("an edit outside the project after a background run: Stop line %q, want %s", line, bgo2Unknown)
	}
}

// TestBGO2d_ARedirectedOutputIsNoReadClause: p1 edits and launches the gate in
// the background with its output sent to a file of the agent's choosing
// (`go test ./... > x.log`); p2 reads that file and claims done. The agent may
// have read the run's result there, so the line says nothing of the output at
// either polarity: no "not read", no "read since". With no end on the record
// the run is still in the background, and the done flag, reading the evidence
// since the last edit, says so, never "no test or build run".
func TestBGO2d_ARedirectedOutputIsNoReadClause(t *testing.T) {
	e, dir := dghEnv(t, true)
	e.bgo2Call(dir, "p1", "toolu_e", "Edit", dghEdit(dir), map[string]any{"filePath": filepath.Join(dir, "internal", "a.go")})
	e.bgo2Call(dir, "p1", "toolu_bg", "Bash", map[string]any{"command": "go test ./... > x.log 2>&1", "run_in_background": true}, bgo2Bg)
	e.dghStop(dir)
	e.bgo2Call(dir, "p2", "toolu_r", "Bash", map[string]any{"command": "cat x.log"}, bgo2Quiet)
	line := e.dghStop(dir)
	if !strings.Contains(line, "moved to the background") {
		t.Errorf("a later turn's done claim after a redirected background run: Stop line %q, want it moved to the background", line)
	}
	for _, deny := range []string{"no test or build run", "not read", "read since"} {
		if strings.Contains(line, deny) {
			t.Errorf("Stop line %q says %q", line, deny)
		}
	}
}
