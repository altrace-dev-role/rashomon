package acceptance

// RS1: a red read in a wrapper -- a script, make -- steps the post as a
// runner's red does, and the notes say the run was read from its output as
// it ran inside another command, never that a pipe hid it (card
// WRAPPER-RED-STEP1). PROVEN only: a wrapper's red is its runner's failing
// summary; its own exit decides nothing.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// rs1Text is the text a post printed for the agent or the person, "" when
// it printed nothing.
func rs1Text(t *testing.T, stdout string) string {
	t.Helper()
	if stdout == "" {
		return ""
	}
	var out struct {
		SystemMessage string `json:"systemMessage"`
		Hook          struct {
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("post stdout %q is not JSON: %v", stdout, err)
	}
	return out.Hook.Context + out.SystemMessage
}

// rs1Lines counts the session's steer lines of pattern p.
func (e *env) rs1Lines(p pattern.ID) int {
	n := 0
	for _, l := range e.steerLines() {
		if l.str("pattern") == string(p) {
			n++
		}
	}
	return n
}

// TestRS1_AWrappedRedRaisesRegressionLocation: under a trusted spec's gate
// (go test ./...), a green go test, an edit, then a red read in a wrapper.
// At PostToolUse (a script that ran go test and exited 0) the note prints
// and says the run was read from its output as it ran inside another
// command; at PostToolUseFailure (the script exited 1; make check exited 2)
// the line is logged, as STR43's are below the failure event's floor. Break:
// step only on a runner's record, or keep WRAPPER-RUN1's refusal of a
// wrapped red.
func TestRS1_AWrappedRedRaisesRegressionLocation(t *testing.T) {
	const ranInside = "and its output reports a failure on this run (read from its output; it ran inside another command). " +
		"Files written between the two runs: 'internal/a.go'."
	t.Run("script, exit 0, PostToolUse", func(t *testing.T) {
		e, dir := dghEnv(t, true)
		e.wr1DeclaredCall(dir, "toolu_g", "go test ./...", wr1GoPass, "")
		e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
		text := rs1Text(t, e.wr1DeclaredCall(dir, "toolu_r", "bash scripts/test.sh", wr1GoFail, ""))
		if !strings.Contains(text, "gate 'go' passed earlier in this session") || !strings.Contains(text, ranInside) || strings.Contains(text, "pipe") {
			t.Errorf("the red's post printed %q, want gate 'go' and %q, and no pipe", text, ranInside)
		}
	})
	for _, c := range []struct{ name, command, failErr string }{
		{"script, exit 1, PostToolUseFailure", "bash scripts/test.sh", "Exit code 1\n" + wr1GoFail},
		{"make check, exit 2, PostToolUseFailure", "make check", "Exit code 2\ngo test ./...\n" + wr1GoFail + "make: *** [check] Error 1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e, dir := dghEnv(t, true)
			e.wr1DeclaredCall(dir, "toolu_g", "go test ./...", wr1GoPass, "")
			e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
			if text := rs1Text(t, e.wr1DeclaredCall(dir, "toolu_r", c.command, "", c.failErr)); strings.Contains(text, "pipe") {
				t.Errorf("the red's post printed %q", text)
			}
			if n := e.rs1Lines(pattern.RegressionLocation); n != 1 {
				t.Errorf("%d regression-location lines, want 1", n)
			}
		})
	}
}

// TestRS1_ADirectRedAfterAWrappedGreenRaisesRegressionLocation: a green
// read in a script, an edit, then go test exits 1: the gate passed earlier
// and is red now, a regression like any other. Break: keep WRAPPER-RUN1's
// refusal of a wrapped green.
func TestRS1_ADirectRedAfterAWrappedGreenRaisesRegressionLocation(t *testing.T) {
	e, dir := dghEnv(t, true)
	e.wr1DeclaredCall(dir, "toolu_g", "bash scripts/test.sh", wr1GoPass, "")
	e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
	e.wr1DeclaredCall(dir, "toolu_r", "go test ./...", "", "Exit code 1\n"+wr1GoFail)
	if n := e.rs1Lines(pattern.RegressionLocation); n != 1 {
		t.Errorf("%d regression-location lines, want 1", n)
	}
}

// TestRS1_AWrappedGreenRaisesStaleEvidence: make test, whose output go
// test's table reads as passed, then an edit: the note says the green was
// read from its output as it ran inside another command, never that a pipe
// hid it. Break: keep WRAPPER-RUN1's refusal of a wrapped green, or render
// the pipe's row.
func TestRS1_AWrappedGreenRaisesStaleEvidence(t *testing.T) {
	e, dir := dghEnv(t, true)
	e.wr1DeclaredCall(dir, "toolu_g", "make test", "go test ./...\n"+wr1GoPass, "")
	e.backdate("toolu_g", 120_000)
	text := rs1Text(t, e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir)))
	const want = "gate 'unit' passed earlier in this session (2 min ago), read from its output as it ran inside another command; " +
		"this call then wrote 'internal/a.go'. No later run of the gate is recorded."
	if !strings.Contains(text, want) || strings.Contains(text, "pipe") {
		t.Errorf("the edit's post printed %q, want %q", text, want)
	}
}

// TestRS1_ControlsAWrapperThatProvesNoRed: after the same green and edit, a
// script whose output no runner signed, and one whose go summary passed
// before it exited 1, raise no regression: neither is a red its runner
// printed. Passes before and after the card.
func TestRS1_ControlsAWrapperThatProvesNoRed(t *testing.T) {
	for _, c := range []struct{ name, stdout, failErr string }{
		{"unsigned output, exit 0", "done\n", ""},
		{"go passed, then the script exited 1", "", "Exit code 1\n" + wr1GoPass},
	} {
		t.Run(c.name, func(t *testing.T) {
			e, dir := dghEnv(t, true)
			e.wr1DeclaredCall(dir, "toolu_g", "go test ./...", wr1GoPass, "")
			e.dghCall(dir, "toolu_e", "Edit", dghEdit(dir))
			if text := rs1Text(t, e.wr1DeclaredCall(dir, "toolu_r", "bash scripts/test.sh", c.stdout, c.failErr)); text != "" {
				t.Errorf("the script's post printed %q, want nothing", text)
			}
			if n := e.rs1Lines(pattern.RegressionLocation); n != 0 {
				t.Errorf("%d regression-location lines, want 0", n)
			}
		})
	}
}
