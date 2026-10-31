package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// H-118 -- a note the transcript does not show shadows its event next session
// (card CHANNEL-EVIDENCE1, D226).
//
// The floor table trusts every Claude Code at or above an event's floor. A
// release that stopped forwarding additionalContext would be trusted until a
// probe failed on it. At Stop, rashomon reads the transcript's end for each
// delivered note, as the text of a hook_additional_context attachment, labels
// it seen, absent or unreadable on the steer log, and, for one absent on a
// trusted version, shadows that event for the next session with a line that
// says why. Break: skip the Stop's read and a channel that dropped its notes
// stays trusted for ever.
//
// The transcripts here are written by the test in the form Claude Code 2.1.304
// writes a delivered note (p-tru-1.sh's capture: a hook_additional_context
// attachment whose content is a list of strings). They hold nothing a session
// said, only the note text the binary printed.

// stNow304 is the agent whose PostToolUseFailure the floor trusts (2.1.304).
const stNow304 = "AI_AGENT=claude-code_2-1-304_harness"

// ceNotice is the channel notice the next session prints, as 2.1.304.
const ceNotice = "rashomon: a note was built and not sent to the agent (Claude Code 2.1.304, PostToolUseFailure; " +
	"delivery verified from 2.1.304): the last session's transcript did not show a delivered note for this event. " +
	"Shown once per session."

var atStamp = regexp.MustCompile(`"at_unix_ms":(\d{13})`)

// ceNote runs a regression in session as 2.1.304 and returns the note the red's
// post printed to the agent. The green and the edit are backdated, so the red
// states its minutes.
func (e *env) ceNote(session string) string {
	e.t.Helper()
	call := func(id, tool string, input map[string]any, exit int) result {
		envs := []string{stAgent, "CI=", stNow304}
		if res := e.hook(stPayload(e.t, "PreToolUse", id, tool, input, map[string]any{"session_id": session}), envs...); res.exitCode != 0 {
			e.t.Fatalf("hook %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
		}
		event, more := "PostToolUse", map[string]any{"tool_response": map[string]any{"stdout": "", "stderr": "", "interrupted": false}}
		if exit != 0 {
			event, more = "PostToolUseFailure", map[string]any{"error": "Exit code " + strconv.Itoa(exit), "is_interrupt": false, "duration_ms": 30}
		}
		more["session_id"] = session
		res := e.post(stPayload(e.t, event, id, tool, input, more), envs...)
		if res.exitCode != 0 {
			e.t.Fatalf("post %s: exit %d, stderr %q", id, res.exitCode, res.stderr)
		}
		return res
	}
	if session != testSession {
		e.probe("start", session)
	}
	call("toolu_g", "Bash", stGoTest, 0)
	call("toolu_e", "Edit", stEdit, 0)
	e.backdateIn(session, "toolu_g", 180_000)
	e.backdateIn(session, "toolu_e", 120_000)
	res := call("toolu_r", "Bash", stGoTest, 1)
	var out struct {
		Hook struct {
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &out); err != nil || out.Hook.Context == "" {
		e.t.Fatalf("the red's post as 2.1.304 delivered no note: %q (%v)", res.stdout, err)
	}
	return out.Hook.Context
}

// backdateIn is backdate for any session's records.
func (e *env) backdateIn(session, id string, ms int64) {
	e.t.Helper()
	path := filepath.Join(e.home, "runs", session, "records.ndjson")
	b, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatal(err)
	}
	lines := strings.SplitAfter(string(b), "\n")
	for i, l := range lines {
		if !strings.Contains(l, `"type":"declaration"`) || !strings.Contains(l, `"tool_use_id":"`+id+`"`) {
			continue
		}
		m := stamp.FindStringSubmatchIndex(l)
		at, _ := strconv.ParseInt(l[m[2]:m[3]], 10, 64)
		lines[i] = l[:m[2]] + strconv.FormatInt(at-ms, 10) + l[m[3]:]
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "")), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

// ageNotes moves every steer line of session back by ms, so a Stop sees its
// notes as settled.
func (e *env) ageNotes(session string, ms int64) {
	e.t.Helper()
	path := filepath.Join(e.home, "runs", session, "steer.ndjson")
	b, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatal(err)
	}
	aged := atStamp.ReplaceAllStringFunc(string(b), func(m string) string {
		at, _ := strconv.ParseInt(atStamp.FindStringSubmatch(m)[1], 10, 64)
		return `"at_unix_ms":` + strconv.FormatInt(at-ms, 10)
	})
	if err := os.WriteFile(path, []byte(aged), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

// ceTranscript writes a transcript whose attachments carry the texts, and
// returns its path.
func (e *env) ceTranscript(texts ...string) string {
	e.t.Helper()
	lines := []string{`{"type":"user","timestamp":"2026-10-30T00:00:00.000Z","message":{"role":"user","content":"a prompt"}}`}
	for _, text := range texts {
		b, err := json.Marshal(map[string]any{"type": "attachment", "timestamp": "2026-10-30T00:00:01.000Z",
			"attachment": map[string]any{"type": "hook_additional_context", "hookEvent": "PostToolUseFailure",
				"hookName": "PostToolUseFailure:Bash", "toolUseID": "toolu_r-context", "content": []string{text}}})
		if err != nil {
			e.t.Fatal(err)
		}
		lines = append(lines, string(b))
	}
	p := filepath.Join(e.t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		e.t.Fatal(err)
	}
	return p
}

// ceStop runs the Stop for session over transcript and returns the exit code
// and stdout.
func (e *env) ceStop(session, transcript string) result {
	e.t.Helper()
	return e.ceStopEvent("Stop", session, transcript)
}

// ceStopEvent is ceStop for any Stop-family event.
func (e *env) ceStopEvent(event, session, transcript string) result {
	e.t.Helper()
	b, err := json.Marshal(map[string]any{"hook_event_name": event, "session_id": session, "transcript_path": transcript,
		"stop_hook_active": false, "last_assistant_message": "Done."})
	if err != nil {
		e.t.Fatal(err)
	}
	res := e.recap(string(b), stAgent, "CI=", stNow304)
	if res.exitCode != 0 {
		e.t.Fatalf("Stop: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	return res
}

// evidenceLines are the steer_evidence lines of session's log.
func (e *env) evidenceLines(session string) []record {
	e.t.Helper()
	var out []record
	for _, l := range e.read(session, "steer.ndjson") {
		if l.str("type") == "steer_evidence" {
			out = append(out, l)
		}
	}
	return out
}

func (e *env) markerPath(event string) string {
	return filepath.Join(e.home, "channel-evidence-"+event)
}

// TestH118_AbsentNoteShadowsItsEventNextSession drives the whole loop through
// the binary: a delivered note, a Stop over a transcript that lacks it, the
// marker, and the next session's call, which prints the notice with the
// reason and no note.
func TestH118_AbsentNoteShadowsItsEventNextSession(t *testing.T) {
	e := newSteerEnv(t)
	e.ceNote(testSession)
	e.ageNotes(testSession, 60_000)

	res := e.ceStop(testSession, e.ceTranscript("some other hook's context"))
	if strings.Contains(res.stdout, "evidence") {
		t.Errorf("the Stop printed about the evidence: %q", res.stdout)
	}
	ev := e.evidenceLines(testSession)
	if len(ev) != 1 || ev[0].str("evidence") != "absent" || ev[0].str("event") != "PostToolUseFailure" {
		t.Fatalf("evidence lines = %v, want one absent for PostToolUseFailure", ev)
	}
	note := e.read(testSession, "steer.ndjson")[0]
	if ev[0].str("steer_id") != note.str("id") {
		t.Errorf("the evidence names %q, want the note %q", ev[0].str("steer_id"), note.str("id"))
	}
	b, err := os.ReadFile(e.markerPath("PostToolUseFailure"))
	if err != nil || string(b) != "2.1.304\n" {
		t.Fatalf("marker = %q (%v), want 2.1.304", b, err)
	}
	if _, err := os.Stat(e.markerPath("PostToolUse")); err == nil {
		t.Error("the shadow reached an event that had no absent note")
	}

	// The next session on the same version: the event is shadowed, the note
	// not sent, and the line says why.
	call := func(id, tool string, input map[string]any, exit int) result {
		envs := []string{stAgent, "CI=", stNow304}
		e.hook(stPayload(t, "PreToolUse", id, tool, input, map[string]any{"session_id": "sess-2"}), envs...)
		event, more := "PostToolUse", map[string]any{"tool_response": map[string]any{"stdout": "", "stderr": "", "interrupted": false}}
		if exit != 0 {
			event, more = "PostToolUseFailure", map[string]any{"error": "Exit code 1", "is_interrupt": false, "duration_ms": 30}
		}
		more["session_id"] = "sess-2"
		return e.post(stPayload(t, event, id, tool, input, more), envs...)
	}
	if res := e.probe("start", "sess-2"); res.exitCode != 0 {
		t.Fatalf("probe start: exit %d", res.exitCode)
	}
	call("toolu_g", "Bash", stGoTest, 0)
	call("toolu_e", "Edit", stEdit, 0)
	e.backdateIn("sess-2", "toolu_g", 180_000)
	e.backdateIn("sess-2", "toolu_e", 120_000)
	red := call("toolu_r", "Bash", stGoTest, 1)
	if !printsOnly(red.stdout, ceNotice) {
		t.Errorf("the next session's red printed %q, want the notice alone: %q", red.stdout, ceNotice)
	}
	last := e.read("sess-2", "steer.ndjson")
	if l := last[len(last)-1]; l.str("suppressed_by") != "unverified_channel" || l.str("notice") != "channel" {
		t.Errorf("the next session's line = %s", l.raw)
	}

	// A probe pass removes the marker: the same call then delivers.
	if err := os.Remove(e.markerPath("PostToolUseFailure")); err != nil {
		t.Fatal(err)
	}
	e.ceNote("sess-3")
}

// TestH118_OnlyAStopReadsTheTranscript: a StopFailure, which is no turn's end,
// reads nothing and labels nothing.
func TestH118_OnlyAStopReadsTheTranscript(t *testing.T) {
	e := newSteerEnv(t)
	e.ceNote(testSession)
	e.ageNotes(testSession, 60_000)
	e.ceStopEvent("StopFailure", testSession, e.ceTranscript("another hook's context"))
	if ev := e.evidenceLines(testSession); len(ev) != 0 {
		t.Errorf("a StopFailure wrote evidence: %v", ev)
	}
	if _, err := os.Stat(e.markerPath("PostToolUseFailure")); err == nil {
		t.Error("a StopFailure wrote a marker")
	}
}

// TestH118_SeenAndUnreadableChangeNoState: a transcript that shows the note,
// and one that is gone, leave no marker, and the Stop still exits 0.
func TestH118_SeenAndUnreadableChangeNoState(t *testing.T) {
	for name, tc := range map[string]struct {
		transcript func(e *env, note string) string
		want       string
	}{
		"seen":       {func(e *env, note string) string { return e.ceTranscript(note) }, "seen"},
		"unreadable": {func(e *env, _ string) string { return filepath.Join(e.home, "no-such-transcript.jsonl") }, "unreadable"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newSteerEnv(t)
			note := e.ceNote(testSession)
			e.ageNotes(testSession, 60_000)
			e.ceStop(testSession, tc.transcript(e, note))
			ev := e.evidenceLines(testSession)
			if len(ev) != 1 || ev[0].str("evidence") != tc.want {
				t.Fatalf("evidence lines = %v, want one %s", ev, tc.want)
			}
			if _, err := os.Stat(e.markerPath("PostToolUseFailure")); err == nil {
				t.Error("a marker was written")
			}
		})
	}
}
