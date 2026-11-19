package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Step 1c: a Stop whose named prompt made no call of the main agent, and a
// later Stop of the same prompt that backs a flagged claim. The Stop payloads
// here name their prompt, as Claude Code 2.1.280 and later send them.

// stopIn is a Stop payload for sess-1 that names prompt; active is
// stop_hook_active.
func stopIn(prompt, msg, cwd string, active bool) string {
	b, err := json.Marshal(map[string]any{
		"hook_event_name": "Stop", "session_id": testSession, "transcript_path": "/tmp/transcripts/" + testSession + ".jsonl",
		"cwd": cwd, "permission_mode": "default", "prompt_id": prompt, "stop_hook_active": active, "last_assistant_message": msg,
	})
	if err != nil {
		panic(err)
	}
	return string(b)
}

// sessMarks is recap.json's marks for sess-1: the prompt last spoken for,
// the prompt last checked, the prompt of the flagged Stop whose note is not
// yet taken, the prompt of the flagged Stop whose note was taken, the file's
// last evaluation time and the done marks.
type sessMarks struct {
	Spoken, Checked, Unverified, Noted string
	AtMS                               int64
	Done                               doneMarks
}

func (e *env) marks() sessMarks {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.home, "recap.json"))
	if os.IsNotExist(err) {
		return sessMarks{}
	}
	if err != nil {
		e.t.Fatal(err)
	}
	var s struct {
		Spoken     map[string]string    `json:"spoken"`
		Checked    map[string]string    `json:"checked"`
		Unverified map[string]string    `json:"unverified"`
		Noted      map[string]string    `json:"noted"`
		AtMS       int64                `json:"last_evaluated_at_unix_ms"`
		Done       map[string]doneMarks `json:"done_evidence"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		e.t.Fatalf("recap.json: %v\n%s", err, b)
	}
	return sessMarks{
		Spoken: s.Spoken[testSession], Checked: s.Checked[testSession], Unverified: s.Unverified[testSession],
		Noted: s.Noted[testSession], AtMS: s.AtMS, Done: s.Done[testSession],
	}
}

// flaggedStopIn is flaggedStop with a Stop that names prompt-1.
func (e *env) flaggedStopIn(t *testing.T) string {
	t.Helper()
	e.turnCall(t, "prompt-1", "toolu_test", goTestRun("ok"))
	e.turnCall(t, "prompt-1", "toolu_edit_1", editOf("main.go"))
	e.turnCall(t, "prompt-1", "toolu_edit_2", editOf("util.go"))
	line := e.mustSpeak(stopIn("prompt-1", "All 42 tests pass.", e.cwd, false))
	if !strings.Contains(line, "done was claimed") {
		t.Fatalf("premise: Stop 1 printed %q, want the done flag", line)
	}
	return line
}

// spokeOrSilent names whether a Stop printed a line.
func spokeOrSilent(line string) string {
	if line == "" {
		return "silent"
	}
	return "spoke"
}

// TestDONE1t_TextOnlyStopAndTheCatchUp: a Stop whose named prompt made no
// call reads no done claim and keeps the stored done marks; checked stays on
// the turn before, so the catch-up of prompt 3 finds that turn checked,
// prints no line of prompt 1's turn and leaves the evaluation time as the
// text-only Stop wrote it. The rows are by input and reach the four cells of
// Stop 1 spoke or not by the text-only Stop spoke or not; a cell no row
// reaches reads SKIP. Each row logs both Stops' lines, the marks around each
// Stop and what prompt 3 printed.
func TestDONE1t_TextOnlyStopAndTheCatchUp(t *testing.T) {
	reached := map[string][]string{}
	for _, tc := range []struct {
		name       string
		stop1      string // flag: a done claim over two edits after a run; acted: a call and no claim; edit: an edit after a run and no claim
		summaryOff bool   // the summary switch off first, so a Stop with no flag and no rule prints nothing
		stop2Msg   string // the text-only Stop's message
		cell       string // the cell the row is built for, "" where any cell holds
	}{
		{"stop1 flag, stop2 done claim", "flag", false, "Done.", ""},
		{"stop1 flag, stop2 tests-pass claim", "flag", false, "All 42 tests pass.", ""},
		{"stop1 no claim, stop2 done claim", "acted", false, "Done.", ""},
		{"stop1 no claim, stop2 tests-pass claim", "acted", false, "All 42 tests pass.", ""},
		{"stop1 value line, stop2 tests-pass claim", "edit", false, "All 42 tests pass.", "S1 spoke, S2 spoke"},
		{"stop1 silent under summary off, stop2 tests-pass claim", "edit", true, "All 42 tests pass.", "S1 silent, S2 spoke"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newSteerEnv(t)
			if tc.summaryOff {
				if res := e.run("", nil, "summary", "off"); res.exitCode != 0 {
					t.Fatalf("summary off: exit %d, stderr %q", res.exitCode, res.stderr)
				}
			}
			msg1 := "I ran the check."
			switch tc.stop1 {
			case "flag":
				e.turnCall(t, "prompt-1", "toolu_test", goTestRun("ok"))
				e.turnCall(t, "prompt-1", "toolu_edit_1", editOf("main.go"))
				e.turnCall(t, "prompt-1", "toolu_edit_2", editOf("util.go"))
				msg1 = "All 42 tests pass."
			case "acted":
				e.actedTurn(t, "prompt-1")
			case "edit":
				e.turnCall(t, "prompt-1", "toolu_test", goTestRun("ok"))
				e.turnCall(t, "prompt-1", "toolu_edit_1", editOf("main.go"))
				msg1 = "I made the change."
			}
			at0 := e.marks()
			line1, _ := e.recapLine(stopIn("prompt-1", msg1, e.cwd, false))
			if tc.stop1 == "flag" && !strings.Contains(line1, "done was claimed") {
				t.Fatalf("premise: Stop 1 printed %q, want the done flag", line1)
			}
			at1 := e.marks()
			noteOf(t, e, "prompt-2")
			before := e.marks()
			line2, _ := e.recapLine(stopIn("prompt-2", tc.stop2Msg, e.cwd, false))
			after := e.marks()
			cell := "S1 " + spokeOrSilent(line1) + ", S2 " + spokeOrSilent(line2)
			time.Sleep(5 * time.Millisecond)
			out3 := noteOf(t, e, "prompt-3")
			at3 := e.marks()
			t.Logf("ROW %s: cell (%s); stop1 %q, spoken %q then %q; stop2 %q, spoken %q then %q; marks after stop1 %+v, before stop2 %+v, after stop2 %+v, after prompt-3 %+v; prompt-3 %+v",
				tc.name, cell, line1, at0.Spoken, at1.Spoken, line2, before.Spoken, after.Spoken, at1, before, after, at3, out3)
			reached[cell] = append(reached[cell], tc.name)
			if tc.cell != "" && cell != tc.cell {
				t.Fatalf("premise: the row reached (%s), want (%s)", cell, tc.cell)
			}
			if after.Checked != "prompt-1" {
				t.Errorf("checked = %q after the text-only Stop of prompt-2, want prompt-1: a prompt with no call is never the turn a catch-up selects", after.Checked)
			}
			if at3.AtMS != after.AtMS {
				t.Errorf("last evaluated %d after the text-only Stop, %d after prompt-3: the catch-up reached the claim, want it unchanged", after.AtMS, at3.AtMS)
			}
			if tc.stop2Msg == "Done." && (strings.Contains(line2, "done was claimed") || after.Done != before.Done) {
				t.Errorf("the text-only Stop read a done claim: line %q, done marks %+v then %+v, want no flag and the marks kept", line2, before.Done, after.Done)
			}
			if strings.Contains(out3.SystemMessage, "done was claimed") || strings.Contains(out3.SystemMessage, "rashomon report --session") {
				t.Errorf("prompt-3's catch-up printed %q, want no line of prompt-1's turn", out3.SystemMessage)
			}
		})
	}
	for _, cell := range []string{"S1 spoke, S2 spoke", "S1 spoke, S2 silent", "S1 silent, S2 spoke", "S1 silent, S2 silent"} {
		t.Run("cell "+cell, func(t *testing.T) {
			if len(reached[cell]) == 0 {
				t.Skip("no row above reached this cell")
			}
			t.Logf("CELL (%s): rows %q", cell, reached[cell])
		})
	}
}

// TestDONE1t_TestsPassClaimOnATextOnlyStop: what the tree does with a
// text-only "All 42 tests pass." (i) with a passing run in the turn before,
// (ii) with no run since the last covered edit. The tests-pass class is not
// re-ruled in 1c; both outcomes are pinned. (i) No flag, no entry, no note.
// (ii) The claim is read against the evidence since the last edit and is
// flagged under prompt-2: the flag prints at the text-only Stop, the entry
// holds prompt-2, checked stays prompt-1, and prompt 3 carries the note once
// with no line of prompt 1's turn.
func TestDONE1t_TestsPassClaimOnATextOnlyStop(t *testing.T) {
	for _, tc := range []struct {
		name  string
		green bool
	}{
		{"(i) a passing run in the turn before", true},
		{"(ii) no run since the last covered edit", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newSteerEnv(t)
			if tc.green {
				e.greenTurn(t, "prompt-1")
			} else {
				e.turnCall(t, "prompt-1", "toolu_test", goTestRun("ok"))
				e.turnCall(t, "prompt-1", "toolu_edit_1", editOf("main.go"))
			}
			line1, _ := e.recapLine(stopIn("prompt-1", "I made the change.", e.cwd, false))
			noteOf(t, e, "prompt-2")
			line2, _ := e.recapLine(stopIn("prompt-2", "All 42 tests pass.", e.cwd, false))
			m := e.marks()
			out3 := noteOf(t, e, "prompt-3")
			out4 := noteOf(t, e, "prompt-4")
			t.Logf("OUTCOME %s: stop1 %q; text-only stop2 %q; marks %+v; prompt-3 %+v; prompt-4 %+v", tc.name, line1, line2, m, out3, out4)
			note3 := out3.HookSpecificOutput.AdditionalContext
			if tc.green {
				if strings.Contains(line2, "done was claimed") || m.Unverified != "" || note3 != "" {
					t.Errorf("(i): stop2 %q, entry %q, prompt-3 note %q; want no flag, no entry and no note", line2, m.Unverified, note3)
				}
			} else {
				if !strings.Contains(line2, "done was claimed") {
					t.Errorf("(ii): stop2 printed %q, want the done flag", line2)
				}
				if m.Unverified != "prompt-2" || m.Checked != "prompt-1" {
					t.Errorf("(ii): entry %q, checked %q; want the entry prompt-2 and checked prompt-1", m.Unverified, m.Checked)
				}
				if note3 != doneNote {
					t.Errorf("(ii): prompt-3 carried %q, want the note", note3)
				}
				if strings.Contains(out3.SystemMessage, "rashomon report --session") {
					t.Errorf("(ii): prompt-3 printed %q, want no line of prompt-1's turn", out3.SystemMessage)
				}
				if note4 := out4.HookSpecificOutput.AdditionalContext; note4 != "" {
					t.Errorf("(ii): prompt-4 carried %q, want no note: the note is given once", note4)
				}
			}
		})
	}
}

// TestDONE1u_TextOnlyDoneClaimAfterAnUnflaggedStop (case (i)b): after an
// unflagged Stop over an edit made after a passing run, a text-only done
// claim is not read under the turn before: no flag, no entry, no note.
func TestDONE1u_TextOnlyDoneClaimAfterAnUnflaggedStop(t *testing.T) {
	e := newSteerEnv(t)
	e.turnCall(t, "prompt-1", "toolu_test", goTestRun("ok"))
	e.turnCall(t, "prompt-1", "toolu_edit_1", editOf("main.go"))
	if line, _ := e.recapLine(stopIn("prompt-1", "I made the change.", e.cwd, false)); strings.Contains(line, "done was claimed") {
		t.Fatalf("premise: Stop 1 printed %q, want no done flag", line)
	}
	noteOf(t, e, "prompt-2")
	before := e.marks()
	line2, _ := e.recapLine(stopIn("prompt-2", "Done.", e.cwd, false))
	after := e.marks()
	if strings.Contains(line2, "done was claimed") || after.Done != before.Done || after.Unverified != "" {
		t.Errorf("the text-only Stop: line %q, marks %+v then %+v, want no flag, the done marks kept and no entry", line2, before, after)
	}
	if out := noteOf(t, e, "prompt-3"); out.HookSpecificOutput.AdditionalContext != "" {
		t.Errorf("prompt-3 carried %q, want no note", out.HookSpecificOutput.AdditionalContext)
	}
}

// TestDONE1v_ASecondStopThatBacksTheClaimClearsTheEntry (arm U): a flagged
// Stop, a passing run in the same prompt, and a Stop of that prompt whose
// claim is read and backed. The entry is deleted and the next prompt carries
// no note. Stop 2 prints nothing: the person's last line for the prompt stays
// the flag (a stated limit; debt row 1c-D1).
func TestDONE1v_ASecondStopThatBacksTheClaimClearsTheEntry(t *testing.T) {
	e := newSteerEnv(t)
	e.flaggedStopIn(t)
	if m := e.marks(); m.Unverified != "prompt-1" {
		t.Fatalf("premise: entry %q after Stop 1, want prompt-1", m.Unverified)
	}
	e.turnCall(t, "prompt-1", "toolu_test_again", goTestRun("ok"))
	if line, _ := e.recapLine(stopIn("prompt-1", "All 42 tests pass.", e.cwd, true)); line != "" {
		t.Errorf("Stop 2 printed %q, want nothing", line)
	}
	if m := e.marks(); m.Unverified != "" {
		t.Errorf("entry %q after Stop 2 backed the claim, want none", m.Unverified)
	}
	if out := noteOf(t, e, "prompt-2"); out.HookSpecificOutput.AdditionalContext != "" {
		t.Errorf("prompt-2 carried %q, want no note: the latest Stop of prompt-1 backed its claim", out.HookSpecificOutput.AdditionalContext)
	}
}

// TestDONE1x_TheEntryIsDeletedInOneCell: Stop 2's state (claim read and
// backed; read and flagged; no claim; not read) by the entry it meets (this
// prompt's; another prompt's and untaken; none). The entry is deleted only
// where the claim is read and backed and the entry holds Stop 2's prompt. A
// second Stop with no claim keeps the note. The cell (not read, this
// prompt's) cannot be built and reads SKIP: a prompt that carried Stop 1's
// calls is not text-only.
func TestDONE1x_TheEntryIsDeletedInOneCell(t *testing.T) {
	type state struct {
		name string
		call *fixtureCall
		msg  string
	}
	pass, edit := goTestRun("ok"), editOf("late.go")
	act := fixtureCall{Tool: "Bash", Input: map[string]any{"command": "frobnicate --check"}, Outcome: "ok", Output: "ok\n"}
	states := []state{
		{"backed", &pass, "All 42 tests pass."},
		{"flagged", &edit, "All 42 tests pass."},
		{"no claim", &act, "I looked into it."},
		{"not read", nil, "Done."},
	}
	for _, entry := range []string{"this", "other", "none"} {
		for _, st := range states {
			t.Run(entry+"/"+st.name, func(t *testing.T) {
				if entry == "this" && st.call == nil {
					t.Skip("Stop 2 of prompt-1 cannot be text-only: the prompt carried Stop 1's calls")
				}
				e := newSteerEnv(t)
				stopPrompt := "prompt-2"
				switch entry {
				case "this":
					e.flaggedStopIn(t)
					stopPrompt = "prompt-1"
				case "other":
					e.flaggedStopIn(t)
				case "none":
					e.greenTurn(t, "prompt-1")
					e.recapLine(stopIn("prompt-1", "All 42 tests pass.", e.cwd, false))
				}
				before := e.marks()
				if st.call != nil {
					e.turnCall(t, stopPrompt, "toolu_cell", *st.call)
				}
				line, _ := e.recapLine(stopIn(stopPrompt, st.msg, e.cwd, stopPrompt == "prompt-1"))
				after := e.marks()
				deleted := before.Unverified != "" && after.Unverified == ""
				want := entry == "this" && st.name == "backed"
				t.Logf("CELL %s/%s: line %q; entry %q then %q", entry, st.name, line, before.Unverified, after.Unverified)
				if deleted != want {
					t.Errorf("entry %q then %q: deleted %v, want %v", before.Unverified, after.Unverified, deleted, want)
				}
				if entry == "this" && st.name == "no claim" {
					if out := noteOf(t, e, "prompt-3"); out.HookSpecificOutput.AdditionalContext != doneNote {
						t.Errorf("after a second Stop with no claim the next prompt carried %q, want the note", out.HookSpecificOutput.AdditionalContext)
					}
				}
			})
		}
	}
}

// frame1c is a host frame as the prompt hook receives it.
const frame1c = "<task-notification>\n<task-id>b1c0w</task-id>\n<status>completed</status>\n" +
	"<summary>Background command finished</summary>\n</task-notification>"

// TestDONE1w_AFrameThenABackedStopThenATypedPrompt: Stop 1 flags under
// prompt-1; prompt-2 is a frame; its turn runs the gate green and claims done,
// read and backed; the typed prompt-3 carries no note. The frame takes the
// flagged Stop and is told nothing: after it the noted mark and the entry
// both read prompt-1, so the typed prompt finds the note taken.
func TestDONE1w_AFrameThenABackedStopThenATypedPrompt(t *testing.T) {
	e := newSteerEnv(t)
	e.flaggedStopIn(t)
	res, framed := e.prompt(e.cwd, frame1c, "prompt-2")
	if res.exitCode != 0 {
		t.Fatalf("the frame: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	afterFrame := e.marks()
	if afterFrame.Noted != "prompt-1" || afterFrame.Unverified != "prompt-1" {
		t.Errorf("after the frame: noted %q, entry %q; want both prompt-1: the frame takes the flagged Stop and leaves the entry", afterFrame.Noted, afterFrame.Unverified)
	}
	e.turnCall(t, "prompt-2", "toolu_test_2", goTestRun("ok"))
	if line, _ := e.recapLine(stopIn("prompt-2", "All 42 tests pass.", e.cwd, false)); strings.Contains(line, "done was claimed") {
		t.Fatalf("premise: Stop 2 printed %q, want the claim backed", line)
	}
	out := noteOf(t, e, "prompt-3")
	t.Logf("frame printed %+v; marks after the frame %+v; typed prompt-3 %+v", framed, afterFrame, out)
	if out.HookSpecificOutput.AdditionalContext != "" {
		t.Errorf("typed prompt-3 carried %q, want nothing: the latest Stop backed the claim", out.HookSpecificOutput.AdditionalContext)
	}
}

// TestStopEffortKeyReadsTheSame: the 2.1.280 fixture's Stop and the same Stop
// with the key effort, as 2.1.322 sends it, read the same: exit, stdout and
// recap.json with its time fields dropped.
func TestStopEffortKeyReadsTheSame(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "fixtures", "claude-code", "2.1.280", "payloads.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	stop := ""
	for _, l := range strings.Split(string(b), "\n") {
		if strings.Contains(l, `"hook_event_name":"Stop"`) {
			stop = l
		}
	}
	if stop == "" {
		t.Fatal("premise: the fixture holds no Stop")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(stop), &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["effort"]; ok {
		t.Fatal("premise: the fixture's Stop already holds effort")
	}
	m["effort"] = map[string]any{"level": "medium"}
	withEffort, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	read := func(payload string) string {
		e := newSteerEnv(t)
		res := e.recap(payload)
		state := map[string]json.RawMessage{}
		if sb, err := os.ReadFile(filepath.Join(e.home, "recap.json")); err == nil {
			if err := json.Unmarshal(sb, &state); err != nil {
				t.Fatal(err)
			}
		}
		for k := range state {
			if strings.HasSuffix(k, "_ms") || strings.HasSuffix(k, "_at") {
				delete(state, k)
			}
		}
		sj, err := json.Marshal(state)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Join([]string{string(rune('0' + res.exitCode)), res.stdout, string(sj)}, "\x00")
	}
	plain, effort := read(stop), read(string(withEffort))
	t.Logf("READING plain %q", plain)
	if plain != effort {
		t.Errorf("the Stop with effort read %q, want %q", effort, plain)
	}
}
