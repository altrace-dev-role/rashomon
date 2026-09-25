package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// H-107 -- a turn that names no session says so, and describes no other one.
//
// Per Cursor's documentation -- nobody has run Cursor against this recorder
// -- Cursor loads Claude Code's hooks from ~/.claude/settings.json by
// default and maps Stop to its own stop event, and its payloads name a
// conversation_id where Claude Code sends session_id. So the recap can run
// on a turn that names no session. It used to fall back to the newest run in
// the store, which on a machine that also runs Claude Code is some OTHER
// session: the line named that session's finding and pointed at that
// session's report, under a turn it had nothing to do with. That is the one
// failure this program exists to make impossible -- a claim with no evidence
// behind it, printed where it reads as a fact.
//
// A turn that names no session now renders the fixed code no_session_id,
// from the digest's own vocabulary beside no_store, through the same
// coverage-reason path hook_entry_absent takes. Silence would be wrong too:
// under exception-only notification, silence reads as a clean turn, when
// nothing about the turn could be checked at all.
//
// Break: restore the NewestRun fallback and the line names sess-1's failure.

// cursorStopPayload is a Stop body shaped the way the release spec reads
// Cursor's hook documentation: conversation_id and generation_id where
// Claude Code sends session_id. The fields beyond those are illustrative;
// what the test depends on is the absence of the session_id key.
func cursorStopPayload() string {
	b, err := json.Marshal(map[string]any{
		"conversation_id": "c0ffee00-cursor-conversation",
		"generation_id":   "g-1",
		"hook_event_name": "Stop",
		"status":          "completed",
		"loop_count":      0,
		"workspace_roots": []string{"/tmp/project"},
	})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// runDirs names the run directories the store holds, sorted.
func (e *env) runDirs() []string {
	e.t.Helper()
	entries, err := os.ReadDir(filepath.Join(e.home, "runs"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		e.t.Fatalf("reading runs: %v", err)
	}
	var names []string
	for _, d := range entries {
		if d.IsDir() {
			names = append(names, d.Name())
		}
	}
	sort.Strings(names)
	return names
}

// wholeReportPointer is the second row a line carries when it has no session
// to name: the unscoped report.
const wholeReportPointer = "→ rashomon report"

func TestH107_AStopThatNamesNoSessionSaysSoAndDescribesNoOtherSession(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	// A real Claude Code turn with a finding: a failed call its summary never
	// mentions. This is what a fallback to the newest run would find and
	// print -- so its absence below is evidence, not a vacuous pass.
	e.mustHook(defaultPayload().build(t))
	e.mustPost(failurePayload(t, testToolUseID, "Exit code 1", false, 30))

	// Premise: the newest run IS sess-1, and it carries the failure. `digest`
	// with no --session resolves the newest run by the same rule the recap
	// used to fall back on, and it writes nothing (H-86).
	if d := e.digest(); d.SessionID != testSession || d.SilentFailures.Failed != 1 {
		t.Fatalf("premise broken: the newest run is %q with %d failure(s), want %q with 1 -- "+
			"without it a fallback would have nothing to leak", d.SessionID, d.SilentFailures.Failed, testSession)
	}
	before := e.runDirs()

	for _, tc := range []struct {
		name, payload string
	}{
		// The case this item exists for.
		{"cursor-shaped: no session_id key at all", cursorStopPayload()},
		// The key present and empty is the same absence.
		{"session_id present and empty", stopPayload("", "Ran it.", false)},
		// A payload this process could not read names no session it can
		// trust either. Guessing one from the store is the defect; saying
		// there is none is the fact.
		{"a payload that is not JSON", "not json at all"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			line, ok := e.recapLine(tc.payload)
			if !ok {
				t.Fatal("recap printed nothing for a turn it could not scope. Silence here " +
					"reads as a clean turn, and nothing about this turn was checked")
			}
			if !strings.Contains(line, "no_session_id") {
				t.Errorf("line = %q, want the fixed reason code no_session_id", line)
			}
			// The fallback's signature: another session's finding, or a pointer
			// at another session's report.
			for _, leak := range []string{testSession, "recorded failure", "--session"} {
				if strings.Contains(line, leak) {
					t.Errorf("line = %q carries %q: a turn that named no session was described "+
						"as another session's turn", line, leak)
				}
			}
			rows := strings.SplitN(line, "\n", 2)
			if len(rows) != 2 || strings.TrimLeft(rows[1], " ") != wholeReportPointer {
				t.Errorf("line = %q, want its second row to be %q: with no session to name, the "+
					"pointer is the whole report", line, wholeReportPointer)
			}
		})
	}

	// Read-only: no run directory appeared, for "" or for the unattributed
	// bucket. The recap has nothing to record, only something to say.
	if after := e.runDirs(); strings.Join(after, ",") != strings.Join(before, ",") {
		t.Errorf("runs before the sessionless recaps = %v, after = %v; the recap wrote a run", before, after)
	}

	// And it consumed nothing. The fallback also CLAIMED sess-1's turn in the
	// recap bookkeeping, so the real Stop for sess-1 was then suppressed as a
	// duplicate: the sessionless turn silenced the Claude Code turn's finding.
	line, ok := e.recapLine(stopPayload(testSession, "Ran the command as requested.", false))
	if !ok || !strings.Contains(line, "1 recorded failure") {
		t.Errorf("the Stop that names sess-1 printed %q (spoke: %v), want its own failure: a "+
			"sessionless recap must not claim a turn it never read", line, ok)
	}
}

// TestH107_TheCatchUpThatNamesNoSessionReportsNoOtherTurn: the
// UserPromptSubmit catch-up had the same fallback. It speaks only for a turn
// it can show no Stop reached (H-106), and with no session id it can show
// nothing -- recap.Pending's own rule answers false when there is nothing to
// say which turn was checked, because catching up could then only repeat a
// line. So it prints nothing, and in particular not the refused turn of
// whichever session happened to write last. The no_session_id statement is
// the sessionless Stop's to make, above.
func TestH107_TheCatchUpThatNamesNoSessionReportsNoOtherTurn(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	// H-106's shape: a refused call, no post, no Stop. The catch-up naming
	// sess-1 WOULD print for this turn -- asserted at the end, so the silence
	// below cannot be a fixture that had nothing to say.
	p := defaultPayload()
	p.PromptID = "prompt-1"
	e.mustHook(p.build(t))

	sessionless := `{"conversation_id":"c0ffee00-cursor-conversation","hook_event_name":"UserPromptSubmit","prompt":"next"}`
	res := e.run(sessionless, nil, append([]string{"recap", "pending"}, e.installArgs()...)...)
	if res.exitCode != 0 {
		t.Fatalf("recap pending: exit %d, stderr %q -- exit 2 on UserPromptSubmit erases the prompt",
			res.exitCode, res.stderr)
	}
	if res.stdout != "" {
		t.Errorf("a catch-up that named no session printed %q; it reported sess-1's refused turn "+
			"under a prompt from somewhere else", res.stdout)
	}

	line, ok := e.recapPending(testSession, "try something else")
	if !ok || !strings.Contains(line, "without recorded execution") {
		t.Errorf("the catch-up naming sess-1 printed %q (spoke: %v), want its refused call: the "+
			"sessionless catch-up must not have consumed it", line, ok)
	}
}
