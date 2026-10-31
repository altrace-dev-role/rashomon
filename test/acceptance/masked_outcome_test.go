package acceptance

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// MASKED-OUTCOME1's card acceptance, through the binary: Claude Code's default
// habit, `python3 -m pytest -q 2>&1 | tail -N`, which hides pytest's exit
// status behind tail's, is recorded as the runner pytest with its outcome read
// from the summary line the pipe kept, and the Stop line says so (D229).

func pytestUnderTail(output string) fixtureCall {
	return fixtureCall{Tool: "Bash", Outcome: "ok", Output: output,
		Input: map[string]any{"command": "python3 -m pytest -q 2>&1 | tail -15"}}
}

// TestMO1_APipedPytestIsRecordedAndSaidWithItsRunner: a piped pytest that
// passed is a record naming python3 -m pytest and the stage tail, with a
// summary_pass beside the exit status the pipe hid, and the value line says
// pytest passed after the last edit, read from its output, not that its exit
// status was hidden.
func TestMO1_APipedPytestIsRecordedAndSaidWithItsRunner(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.turnCall(t, "p1", "toolu_edit", editOf("calc.py"))
	e.turnCall(t, "p1", "toolu_test", pytestUnderTail("....\n4 passed in 0.12s\n"))
	line := e.mustSpeak(stopEvent("Stop", "All tests pass.", e.cwd))

	var found bool
	for _, r := range e.records(testSession) {
		if r.typ() != "execution" || r.fields["runner_name"] == nil {
			continue
		}
		found = true
		if r.str("runner_name") != "python3 -m pytest" || r.str("masked_by") != "tail" || r.fields["schema_version"] != float64(store.CallSchemaVersion) {
			t.Errorf("execution names runner %v stage %v at schema %v, want python3 -m pytest, tail, %d",
				r.fields["runner_name"], r.fields["masked_by"], r.fields["schema_version"], store.CallSchemaVersion)
		}
		if r.str("status_masked") != "test" {
			t.Errorf("status_masked = %v, want test: the exit status stays masked beside the output outcome", r.fields["status_masked"])
		}
		if out, _ := nested(r, "gate_output.kind"); out != "summary_pass" {
			t.Errorf("gate_output kind = %v, want summary_pass", out)
		}
	}
	if !found {
		t.Fatal("no execution record names a runner")
	}
	if !strings.Contains(line, "python3 -m pytest passed after the last edit (read from its output; the pipe hid the exit status)") {
		t.Errorf("value line %q does not say python3 -m pytest passed, read from its output", line)
	}
	for _, bad := range []string{"done was claimed", "hidden by its line", "not recorded"} {
		if strings.Contains(line, bad) {
			t.Errorf("value line %q holds %q", line, bad)
		}
	}
}

// TestMO1_APipedPytestThatFailedRaisesTheClaim: the same line failing under a
// pass claim still raises the masked-runs sentence, as it did: a summary that
// says it failed is the detector's true positive, and the record holds it.
func TestMO1_APipedPytestThatFailedRaisesTheClaim(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.turnCall(t, "p1", "toolu_edit", editOf("calc.py"))
	e.turnCall(t, "p1", "toolu_test", pytestUnderTail("..F.\n1 failed, 3 passed in 0.20s\n"))
	line := e.mustSpeak(stopEvent("Stop", "All tests pass.", e.cwd))
	if !strings.Contains(line, "pass claimed; exit status of 1 build or test run not recorded") {
		t.Errorf("line %q, want the masked-runs sentence: the pass claim stands over a run that failed", line)
	}
}
