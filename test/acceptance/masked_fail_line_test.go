package acceptance

import (
	"strings"
	"testing"
)

// TestMFL1_APipedPytestThatFailedSaysItFailed (MASKED-FAIL-LINE1, #149 F1):
// Claude Code's `python3 -m pytest -q 2>&1 | tail -15` failing under a pass
// claim. The pipe hid the exit status but kept the summary, so the record
// holds the failure: the line raises the claim and names the runner that
// failed, read from its output, and never says the exit status was not
// recorded. Break: count a failure read from its output as a masked run.
func TestMFL1_APipedPytestThatFailedSaysItFailed(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.turnCall(t, "p1", "toolu_edit", editOf("calc.py"))
	e.turnCall(t, "p1", "toolu_test", pytestUnderTail("..F.\n1 failed, 3 passed in 0.20s\n"))
	line := e.mustSpeak(stopEvent("Stop", "All tests pass.", e.cwd))
	for _, want := range []string{
		"done was claimed",
		"python3 -m pytest failed after the last edit (call ",
		"(read from its output; the pipe hid the exit status)",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q does not hold %q", line, want)
		}
	}
	if strings.Contains(line, "not recorded") {
		t.Errorf("line %q says the exit status was not recorded: the record holds the failure, read from the output", line)
	}
}
