package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// DONE-NOTE1 (card c4, PF-PG9): a Stop whose done flag fired (a strict claim
// the record does not back) reaches the agent too: the next prompt of the
// same session carries one note saying so, through the prompt's own channel,
// once. The flag itself is the person's line at the Stop and is unchanged.

const doneNote = "rashomon record: the previous turn ended with a claim of completion that the record does not back " +
	"(no passing test or build run after its last edit, or expected evidence missing)."

// flaggedStop is a turn that ran a green test and then edited two files, and
// a Stop whose final message claims the tests pass: the done flag fires.
func (e *env) flaggedStop(t *testing.T) {
	t.Helper()
	e.turnCall(t, "prompt-1", "toolu_test", goTestRun("ok"))
	e.turnCall(t, "prompt-1", "toolu_edit_1", editOf("main.go"))
	e.turnCall(t, "prompt-1", "toolu_edit_2", editOf("util.go"))
	if line := e.mustSpeak(stopEvent("Stop", "All 42 tests pass.", e.cwd)); !strings.Contains(line, "done was claimed") {
		t.Fatalf("premise: the Stop printed %q, want the done flag", line)
	}
}

// doneLines are the session's claim-vs-record steer lines.
func (e *env) doneLines() []record {
	e.t.Helper()
	var out []record
	for _, l := range e.steerLines() {
		if l.str("pattern") == "claim-vs-record" {
			out = append(out, l)
		}
	}
	return out
}

func noteOf(t *testing.T, e *env, prompt string, extraEnv ...string) preflightOut {
	t.Helper()
	res, out := e.prompt(e.cwd, "next", prompt, extraEnv...)
	if res.exitCode != 0 {
		t.Fatalf("prompt %s: exit %d, stderr %q", prompt, res.exitCode, res.stderr)
	}
	return out
}

// TestDONE1_FlaggedStopIsNotedOnceAtTheNextPrompt: the next prompt carries
// the note in additionalContext with its mirror to the person, and one steer
// line records it (claim-vs-record, done-unverified, delivered at the
// prompt); the prompt after it carries nothing, and a replay of the same Stop
// does not bring it back.
func TestDONE1_FlaggedStopIsNotedOnceAtTheNextPrompt(t *testing.T) {
	e := newSteerEnv(t)
	e.flaggedStop(t)

	out := noteOf(t, e, "prompt-2")
	if out.HookSpecificOutput.AdditionalContext != doneNote {
		t.Errorf("additionalContext = %q, want %q", out.HookSpecificOutput.AdditionalContext, doneNote)
	}
	if want := `rashomon told the agent: "` + doneNote + `"`; !strings.Contains(out.SystemMessage, want) {
		t.Errorf("systemMessage = %q, want the mirror of the note", out.SystemMessage)
	}
	lines := e.doneLines()
	if len(lines) != 1 || lines[0].str("kind") != "done-unverified" || lines[0].str("state") != "delivered" ||
		lines[0].str("event") != "UserPromptSubmit" || lines[0].str("prompt_id") != "prompt-2" || lines[0].str("model_text") != doneNote {
		t.Fatalf("claim-vs-record lines = %v, want one delivered done-unverified line at prompt-2", lines)
	}

	if again := noteOf(t, e, "prompt-3"); again != (preflightOut{}) {
		t.Errorf("the second prompt printed %+v, want nothing", again)
	}
	// A Stop replayed for the same turn (a continuation) does not raise it again.
	e.recapLine(stopEvent("Stop", "All 42 tests pass.", e.cwd))
	if again := noteOf(t, e, "prompt-4"); again.HookSpecificOutput.AdditionalContext != "" {
		t.Errorf("after a replayed Stop the prompt carried %+v, want no second note", again)
	}
	if n := len(e.doneLines()); n != 1 {
		t.Errorf("%d claim-vs-record lines, want 1", n)
	}
}

// TestDONE1b_CleanStopAndOtherSessionsGetNothing: a verified turn leaves no
// note, and a flagged Stop in one session leaves none for another's prompt.
func TestDONE1b_CleanStopAndOtherSessionsGetNothing(t *testing.T) {
	e := newSteerEnv(t)
	e.greenTurn(t, "prompt-1")
	e.mustSpeak(stopEvent("Stop", "All 42 tests pass.", e.cwd))
	if out := noteOf(t, e, "prompt-2"); out != (preflightOut{}) {
		t.Errorf("after a verified Stop the prompt printed %+v, want nothing", out)
	}

	f := newSteerEnv(t)
	f.flaggedStop(t)
	f.watched("sess-other")
	res := f.run(`{"session_id":"sess-other","hook_event_name":"UserPromptSubmit","prompt":"next","prompt_id":"prompt-9","cwd":"`+f.cwd+`","permission_mode":"default"}`,
		f.steerPins(), append([]string{"recap", "pending"}, f.installArgs()...)...)
	if res.exitCode != 0 || res.stdout != "" {
		t.Errorf("another session's prompt: exit %d, stdout %q, want nothing", res.exitCode, res.stdout)
	}
	if out := noteOf(t, f, "prompt-2"); out.HookSpecificOutput.AdditionalContext != doneNote {
		t.Errorf("the flagged session's own next prompt carried %q, want the note", out.HookSpecificOutput.AdditionalContext)
	}
}

// TestDONE1c_SwitchesAndConsentWithholdItAndItDoesNotWaitForThem: with steer
// off (user or project) or no steer consent nothing reaches the agent and
// nothing new is written to the person; the flag was consumed by that prompt,
// so turning steering back on does not deliver it a prompt late.
func TestDONE1c_SwitchesAndConsentWithholdItAndItDoesNotWaitForThem(t *testing.T) {
	for _, tc := range []struct {
		name    string
		off, on func(e *env)
	}{
		{"steer off", func(e *env) { e.mustRun("steer", "off") }, func(e *env) { e.mustRun("steer", "on") }},
		{"steer off --project", func(e *env) { e.mustRun("steer", "off", "--project") }, func(e *env) { e.mustRun("steer", "on", "--project") }},
		{"no consent", func(e *env) { e.steerMarker("") }, func(e *env) { e.steerMarker("2\n") }},
		{"config unreadable", func(e *env) { e.writeConfig("{not json") }, func(e *env) { e.writeConfig("") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newSteerEnv(t)
			e.flaggedStop(t)
			tc.off(e)
			res, out := e.prompt(e.cwd, "next", "prompt-2")
			if res.exitCode != 0 || res.stdout != "" || out != (preflightOut{}) {
				t.Errorf("steering withheld: exit %d, stdout %q, want nothing on any channel", res.exitCode, res.stdout)
			}
			if n := len(e.doneLines()); n != 0 {
				t.Errorf("%d claim-vs-record lines while withheld, want none (a clamped note would print as a flag)", n)
			}
			tc.on(e)
			if out := noteOf(t, e, "prompt-3"); out.HookSpecificOutput.AdditionalContext != "" {
				t.Errorf("steering back on, the next prompt carried %q, want nothing: the flag was that prompt's", out.HookSpecificOutput.AdditionalContext)
			}
		})
	}
}

// TestDONE1d_OffArmDecidesAndWithholds: on a lab store under
// RASHOMON_STEER_ARM=off the note is decided and logged suppressed by
// arm_off, nothing prints, and the steer arm's later prompt does not deliver
// it.
func TestDONE1d_OffArmDecidesAndWithholds(t *testing.T) {
	e := newSteerEnv(t)
	if err := store.WriteLab(e.home); err != nil {
		t.Fatal(err)
	}
	e.flaggedStop(t)
	res, out := e.prompt(e.cwd, "next", "prompt-2", "RASHOMON_STEER_ARM=off")
	if res.exitCode != 0 || res.stdout != "" || out != (preflightOut{}) {
		t.Errorf("off arm: exit %d, stdout %q, want nothing printed", res.exitCode, res.stdout)
	}
	lines := e.doneLines()
	if len(lines) != 1 || lines[0].str("state") != "suppressed" || lines[0].str("suppressed_by") != "arm_off" || lines[0].str("arm") != "off" {
		t.Fatalf("lines = %v, want one done-unverified line suppressed by arm_off", lines)
	}
	if out := noteOf(t, e, "prompt-3", "RASHOMON_STEER_ARM=steer"); out.HookSpecificOutput.AdditionalContext != "" {
		t.Errorf("the steer arm's next prompt carried %q, want nothing: the decision was made", out.HookSpecificOutput.AdditionalContext)
	}
}

// TestDONE1e_NoVersionFloorAtThePrompt: UserPromptSubmit is the one event
// whose row is exempt from the version floor and the channel-evidence
// shadow (VERSION-GATE1, CHANNEL-EVIDENCE1), so the note goes through Route
// like every note and an old client still gets it; the router, not this
// path, decides what a gated event would do.
func TestDONE1e_NoVersionFloorAtThePrompt(t *testing.T) {
	e := newSteerEnv(t)
	e.flaggedStop(t)
	out := noteOf(t, e, "prompt-2", "AI_AGENT=claude-code_2-0-1_harness")
	if out.HookSpecificOutput.AdditionalContext != doneNote {
		t.Errorf("an old client: additionalContext = %q, want the note", out.HookSpecificOutput.AdditionalContext)
	}
}

// TestDONE1f_TheNoteOutranksButDoesNotConsumeThePreflight: a prompt that
// names a path as well gets the one note, the done note (claim-vs-record
// outranks task-preflight); the preflight is superseded and not counted as
// given, so the next prompt carries it. The preflight lists a spec rule: a
// note that lists only paths is the user's flag (card RV1, item 4), which
// never competes for the agent's note.
func TestDONE1f_TheNoteOutranksButDoesNotConsumeThePreflight(t *testing.T) {
	e := newSteerEnv(t)
	e.ruleProject()
	e.flaggedStop(t)
	res, out := e.prompt(e.cwd, "fix the nil check in "+pf1bPath, "prompt-2")
	if res.exitCode != 0 || out.HookSpecificOutput.AdditionalContext != doneNote {
		t.Fatalf("prompt-2: additionalContext = %q (exit %d), want the done note alone", out.HookSpecificOutput.AdditionalContext, res.exitCode)
	}
	_, out = e.prompt(e.cwd, "fix the nil check in "+pf1bPath, "prompt-3")
	if !strings.Contains(out.HookSpecificOutput.AdditionalContext, "'"+pf1bPath+"'") || strings.Contains(out.HookSpecificOutput.AdditionalContext, "previous turn") {
		t.Errorf("prompt-3: additionalContext = %q, want the preflight note naming %s", out.HookSpecificOutput.AdditionalContext, pf1bPath)
	}
}

// mustRun runs the CLI with args and fails on a non-zero exit.
func (e *env) mustRun(args ...string) {
	e.t.Helper()
	if res := e.run("", nil, args...); res.exitCode != 0 {
		e.t.Fatalf("%s: exit %d, stdout %q, stderr %q", strings.Join(args, " "), res.exitCode, res.stdout, res.stderr)
	}
}

// writeConfig replaces config.json with content, or removes it for "".
func (e *env) writeConfig(content string) {
	e.t.Helper()
	path := filepath.Join(e.home, "config.json")
	if content == "" {
		if err := os.Remove(path); err != nil {
			e.t.Fatal(err)
		}
		return
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

// doneMarks is recap.json's done-evidence entry for the session: the digest
// of the last flagged Stop's evidence, the one whose flag the person last
// saw, and the one whose note last reached the agent.
type doneMarks struct {
	Flagged   string `json:"flagged"`
	Shown     string `json:"shown"`
	Delivered string `json:"delivered"`
}

// doneState is the session's done marks, the zero value for none.
func (e *env) doneState() doneMarks {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.home, "recap.json"))
	if os.IsNotExist(err) {
		return doneMarks{}
	}
	if err != nil {
		e.t.Fatal(err)
	}
	var s struct {
		Done map[string]doneMarks `json:"done_evidence"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		e.t.Fatalf("recap.json: %v\n%s", err, b)
	}
	return s.Done[testSession]
}

// actedTurn is a turn that acts (a shell call the record runs) but writes
// nothing and runs no gate: its done window reaches back to the last turn
// that wrote.
func (e *env) actedTurn(t *testing.T, prompt string) {
	t.Helper()
	e.turnCall(t, prompt, "toolu_act_"+prompt, fixtureCall{
		Tool: "Bash", Input: map[string]any{"command": "frobnicate --check"}, Outcome: "ok", Output: "ok\n",
	})
}

// claimDone is a Stop whose final message claims the tests pass; it returns
// the line printed, "" for none.
func (e *env) claimDone() string {
	e.t.Helper()
	line, _ := e.recapLine(stopEvent("Stop", "All 42 tests pass.", e.cwd))
	return line
}

// TestDONE1j_UnchangedEvidenceDeliversOnce: a second flagged Stop over the
// same evidence (a later turn that acted but wrote nothing and ran no gate)
// does not bring the note back; a Stop whose evidence moved (a failing run)
// does.
func TestDONE1j_UnchangedEvidenceDeliversOnce(t *testing.T) {
	e := newSteerEnv(t)
	e.flaggedStop(t)
	first := e.doneState()
	if out := noteOf(t, e, "prompt-2"); out.HookSpecificOutput.AdditionalContext != doneNote {
		t.Fatalf("premise: prompt-2 carried %+v, want the note", out)
	}
	e.actedTurn(t, "prompt-2")
	e.claimDone()
	second := e.doneState()
	if out := noteOf(t, e, "prompt-3"); out.HookSpecificOutput.AdditionalContext != "" {
		t.Errorf("over unchanged evidence prompt-3 carried %q, want no second note", out.HookSpecificOutput.AdditionalContext)
	}
	if first.Flagged == "" || second.Flagged != first.Flagged || second.Delivered != first.Flagged {
		t.Errorf("done marks after Stop 1 %+v and after Stop 2 %+v, want one non-empty digest, flagged and delivered", first, second)
	}

	e.turnCall(t, "prompt-3", "toolu_test_3", goTestRun("failed"))
	e.claimDone()
	third := e.doneState()
	if out := noteOf(t, e, "prompt-4"); out.HookSpecificOutput.AdditionalContext != doneNote {
		t.Errorf("after a failing run prompt-4 carried %q, want the note", out.HookSpecificOutput.AdditionalContext)
	}
	if third.Flagged == "" || third.Flagged == first.Flagged {
		t.Errorf("done marks after the failing run %+v, want a digest other than %q", third, first.Flagged)
	}
}

// TestDONE1m_UnactedTurnBetweenKeepsTheDigest: a turn the main agent made no
// call in prints no flag and leaves the marks as they were; the acted turn
// after it, over the same evidence, brings no note. (A Read call is an act:
// its Stop flags again, as the first RED showed.)
func TestDONE1m_UnactedTurnBetweenKeepsTheDigest(t *testing.T) {
	e := newSteerEnv(t)
	e.flaggedStop(t)
	noteOf(t, e, "prompt-2")
	before := e.doneState()
	if line := e.claimDone(); strings.Contains(line, "done was claimed") {
		t.Fatalf("premise: the Stop of a turn with no call printed %q, want no done flag", line)
	}
	if after := e.doneState(); after != before {
		t.Errorf("done marks %+v after the turn with no call, want %+v unchanged", after, before)
	}
	if out := noteOf(t, e, "prompt-3"); out.HookSpecificOutput.AdditionalContext != "" {
		t.Errorf("prompt-3 carried %q, want nothing", out.HookSpecificOutput.AdditionalContext)
	}
	e.actedTurn(t, "prompt-3")
	e.claimDone()
	if out := noteOf(t, e, "prompt-4"); out.HookSpecificOutput.AdditionalContext != "" {
		t.Errorf("prompt-4 carried %q, want no second note", out.HookSpecificOutput.AdditionalContext)
	}
}

// TestDONE1p_StopLineOncePerEvidence: the person's done flag is printed once
// per evidence too: not at a second Stop over the same evidence, again once
// the evidence moved (a new edit; a failing run would move it too, but its
// failure sentence outranks the flag on the line, as the first RED showed).
func TestDONE1p_StopLineOncePerEvidence(t *testing.T) {
	e := newSteerEnv(t)
	e.flaggedStop(t)
	noteOf(t, e, "prompt-2")
	e.actedTurn(t, "prompt-2")
	if line := e.claimDone(); strings.Contains(line, "done was claimed") {
		t.Errorf("Stop 2 over unchanged evidence printed %q, want no done flag", line)
	}
	noteOf(t, e, "prompt-3")
	e.turnCall(t, "prompt-3", "toolu_edit_3", editOf("extra.go"))
	if line := e.claimDone(); !strings.Contains(line, "done was claimed") {
		t.Errorf("Stop 3 after a new edit printed %q, want the done flag", line)
	}
}
