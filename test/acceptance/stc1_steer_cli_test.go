package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSTC1m_ProjectSummaryOffSilencesThatProjectsStopLine (card STEER-CLI1):
// `summary off --project` writes the project's key, not the user's, and the
// Stop hook reads it for the turn's project: the value line stops, and comes
// back with `summary on --project`. The done flag is not the summary.
func TestSTC1m_ProjectSummaryOffSilencesThatProjectsStopLine(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	res := e.run("", nil, "summary", "off", "--project")
	if res.exitCode != 0 || !strings.HasPrefix(res.stdout, "summary: off for this project (") {
		t.Fatalf("summary off --project: exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
	}
	b, err := os.ReadFile(filepath.Join(e.home, "config.json"))
	if err != nil || !strings.Contains(string(b), `"summary_projects"`) || strings.Contains(string(b), `"summary":`) {
		t.Fatalf("config.json is %q (%v), want the project's key and not the user's", b, err)
	}
	if res := e.run("", nil, "summary"); res.stdout != "summary: on\n" {
		t.Errorf("a project's switch changed the user's: %q", res.stdout)
	}
	e.rpCall(t, "prompt-1", "toolu_red", goTestRun("failed"))
	e.rpFinding(t, "prompt-1", 1)
	e.rpGreen(t, "prompt-2")
	if line, ok := e.recapLine(stopEvent("Stop", "Updated main.go.", e.cwd)); ok {
		t.Errorf("with the project's summary off the Stop printed %q, want nothing", line)
	}
	if res := e.run("", nil, "summary", "on", "--project"); res.exitCode != 0 {
		t.Fatalf("summary on --project: %q", res.stderr)
	}
	e.rpGreen(t, "prompt-3")
	if line := e.mustSpeak(stopEvent("Stop", "Updated main.go again.", e.cwd)); !strings.Contains(line, "verified") {
		t.Errorf("with the project's summary back on the Stop printed %q", line)
	}
}
