package report

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestReadCodexRolloutDeadline: a read past its deadline fails whole, so a
// turn's calls stay unobserved rather than judged on part of the file; no
// deadline reads it all. The turn digest runs inside recap's hook timeout and
// passes one; the report passes none.
func TestReadCodexRolloutDeadline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	line := `{"type":"event_msg","payload":{"type":"item_completed","item":{"type":"CommandExecution","id":"exec-1","exit_code":2}}}` + "\n"
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}

	if r, err := readCodexRollout(path, time.Now().Add(-time.Second)); !errors.Is(err, errRolloutBudget) || r != nil {
		t.Errorf("past its deadline: %v, %v; want nil and errRolloutBudget", r, err)
	}
	r, err := readCodexRollout(path, time.Time{})
	if err != nil || r.outcomes["exec-1"] != store.ExecFailed || r.codes["exec-1"] != 2 {
		t.Fatalf("no deadline: %+v, %v; want exec-1 failed with exit 2", r, err)
	}

	run := &store.Run{
		Declarations: []store.Declaration{{ToolUseID: "exec-1", TranscriptPath: path}},
		Executions:   []store.Execution{{ToolUseID: "exec-1"}},
	}
	CodexOutcomesFromRollouts(run, time.Now().Add(-time.Second))
	if run.Executions[0].Outcome != "" {
		t.Errorf("an expired budget still judged the call: %q", run.Executions[0].Outcome)
	}
	CodexOutcomesFromRollouts(run, time.Now().Add(time.Minute))
	if run.Executions[0].Outcome != store.ExecFailed {
		t.Errorf("within budget: outcome %q, want failed", run.Executions[0].Outcome)
	}
}
