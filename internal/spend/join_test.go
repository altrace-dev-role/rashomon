package spend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// recorder writes a store the way the hooks do, at chosen times, so a turn's
// span is exact. The acceptance suite drives the same join through the real
// hooks (test/acceptance/spend_test.go); this one pins the arithmetic.
type recorder struct {
	t  *testing.T
	st *store.Store
	// transcript is the transcript_path the main agent's declarations carry.
	transcript string
}

func newRecorder(t *testing.T) *recorder {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &recorder{t: t, st: st}
}

// call records one declared call and how it ended.
func (r *recorder) call(session, prompt, toolUseID string, declared, ended time.Time, outcome string) {
	r.t.Helper()
	p := prompt
	if err := r.st.AppendDeclaration(store.Declaration{
		Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
		RecordedAtMS: declared.UnixMilli(), ToolUseID: toolUseID, SessionID: session,
		PromptID: &p, ToolName: "Bash", TranscriptPath: r.transcript,
	}); err != nil {
		r.t.Fatal(err)
	}
	if err := r.st.AppendExecution(store.Execution{
		Type: store.TypeExecution, SchemaVersion: store.SchemaVersion,
		RecordedAtMS: ended.UnixMilli(), ToolUseID: toolUseID, SessionID: session,
		ToolName: "Bash", Outcome: outcome,
	}); err != nil {
		r.t.Fatal(err)
	}
}

// agentCall records one call a subagent made: its declaration carries the
// parent turn's prompt_id, the subagent's agent_id, and -- measured on every
// subagent declaration in a real store -- the MAIN transcript as
// transcript_path, not the subagent's own file.
func (r *recorder) agentCall(session, prompt, toolUseID, transcript string, declared, ended time.Time) {
	r.t.Helper()
	p, agent := prompt, "agent-x"
	if err := r.st.AppendDeclaration(store.Declaration{
		Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
		RecordedAtMS: declared.UnixMilli(), ToolUseID: toolUseID, SessionID: session,
		PromptID: &p, AgentID: &agent, ToolName: "Read", TranscriptPath: transcript,
	}); err != nil {
		r.t.Fatal(err)
	}
	if err := r.st.AppendExecution(store.Execution{
		Type: store.TypeExecution, SchemaVersion: store.SchemaVersion,
		RecordedAtMS: ended.UnixMilli(), ToolUseID: toolUseID, SessionID: session,
		ToolName: "Read", Outcome: store.ExecOK,
	}); err != nil {
		r.t.Fatal(err)
	}
}

// silentSession is the join's shape: a turn with a recorded failure and a
// final message that mentions none, then a clean turn whose final message
// DOES carry a failure word -- which must not be read as the first turn's.
//
// Responses, against the first turn's recorded span [T, T+3s]:
//
//	R0  T-0.5s  the response that made the first call: before the span
//	R1  T+1.5s  main, inside
//	S1  T+2.5s  subagent, whose recorded call names its transcript
//	R3  T+4s    the final reply: after the span
//
// All four are p1's: the main transcript ties R0, R1 and R3 to p1's prompt
// line, and the subagent's own transcript ties S1 to p1 the same way -- its
// first line is the task it was handed, which carries the parent turn's
// promptId. The subagent's recorded call names the MAIN transcript, as real
// hooks record it. Every user line carries its turn's promptId, as a real
// transcript's do. between is written after R3 and
// before p2's prompt (at T+9.5s): a test's own lines for what happens between
// two recorded turns.
func silentSession(t *testing.T, c *config, rec *recorder, T time.Time, final1 string, between ...string) (turn int64) {
	t.Helper()
	sec := func(f float64) time.Time { return T.Add(time.Duration(f * float64(time.Second))) }
	rec.transcript = filepath.Join(c.dir, "projects", "proj", "sess-j.jsonl")
	rec.call("sess-j", "p1", "toolu_1", sec(0), sec(1), store.ExecFailed)
	rec.call("sess-j", "p1", "toolu_2", sec(2), sec(3), store.ExecOK)
	rec.agentCall("sess-j", "p1", "toolu_s", rec.transcript, sec(2.4), sec(2.6))
	rec.call("sess-j", "p2", "toolu_3", sec(10), sec(11), store.ExecOK)

	m := func(id string, at time.Time, in int64, text string) resp {
		return resp{id: id, model: "claude-opus-5-5", session: "sess-j", at: at, in: in, stop: "end_turn", text: text}
	}
	lines := []string{
		userLine("sess-j", "p1", sec(-1), false),
		m("R0", sec(-0.5), 1, "").line("tool_use"),
		userLine("sess-j", "p1", sec(1), true),
		m("R1", sec(1.5), 100, "").line("tool_use"),
		userLine("sess-j", "p1", sec(3), true),
		m("R3", sec(4), 10000, final1).line("text"),
	}
	lines = append(lines, between...)
	lines = append(lines,
		userLine("sess-j", "p2", sec(9.5), false),
		m("R4", sec(10.5), 100000, "").line("tool_use"),
		userLine("sess-j", "p2", sec(11), true),
		m("R5", sec(12), 1000000, "There was an error in the earlier step.").line("text"))
	c.write("proj/sess-j.jsonl", lines...)
	sub := m("S1", sec(2.5), 20, "")
	sub.sidechain = true
	c.write("proj/sess-j/subagents/agent-x.jsonl", subUserLine("sess-j", "p1", sec(2.2)), subMetaLine("sess-j", sec(2.3)),
		sub.line("tool_use"))
	return (1 + 100 + 20 + 10000) * opusIn
}

// subUserLine is a user line of a subagent transcript: marked isSidechain,
// with the promptId of the parent turn, as a real subagent file writes it.
func subUserLine(session, promptID string, at time.Time) string {
	return strings.Replace(userLine(session, promptID, at, false), `"isSidechain":false`, `"isSidechain":true`, 1)
}

// subMetaLine is an injected meta user line with no promptId. It is no
// prompt, so it does not end the tie of the responses after it.
func subMetaLine(session string, at time.Time) string {
	return strings.Replace(subUserLine(session, "", at), `"isSidechain":true`, `"isMeta":true,"isSidechain":true`, 1)
}

func TestJoin_SpendInsideASilentlyFailedTurn(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	inside := silentSession(t, c, rec, now.Add(-2*time.Hour), "Ran the command as requested.")
	// A session rashomon never recorded.
	c.write("proj/sess-u.jsonl", resp{id: "U1", model: "claude-opus-5-5", session: "sess-u",
		at: now.Add(-time.Hour), in: 7, stop: "end_turn"}.line("text"))

	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	j := s.SilentFailureTurns
	if j.Turns != 1 {
		t.Errorf("turns = %d, want 1 (p1 fires; p2 has no failure)", j.Turns)
	}
	if j.Cost.Nano != inside {
		t.Errorf("cost = %d, want %d: every response the transcript ties to p1 -- R0 that made its first call, "+
			"R1, and its final reply R3 -- and the subagent's S1, which its own transcript ties to p1; not p2's", j.Cost.Nano, inside)
	}
	if j.CoveredTranscripts != 1 || j.NotCoveredTranscripts != 1 || j.Transcripts != 2 {
		t.Errorf("covered %d, not covered %d of %d; want 1, 1 of 2", j.CoveredTranscripts, j.NotCoveredTranscripts, j.Transcripts)
	}
	if len(j.NotCoveredSessions) != 1 || j.NotCoveredSessions[0] != "sess-u" {
		t.Errorf("not-covered sessions = %v, want [sess-u] named", j.NotCoveredSessions)
	}
	if j.NotCoveredCost.Nano != 7*opusIn {
		t.Errorf("not-covered cost = %d, want %d: an unrecorded session's spend is shown as not covered, never as zero",
			j.NotCoveredCost.Nano, 7*opusIn)
	}
	txt, _ := render(t, s)
	for _, want := range []string{
		"in turns with a failed call the summary never mentioned: at least $0.04 across 1 turn",
		"savings       $0.04 spent in turns with a failed call the summary never mentioned\n",
		"1 of 2 transcripts was recorded",
		"in the other 1 is not covered",
		"(not-covered spend is in: session sess-u)",
		"so this is a floor",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("text lacks %q:\n%s", want, txt)
		}
	}
	// The fixture's failed call is followed by a successful one: the turn did
	// not end with a failure, and neither the line nor the saving may say so.
	for _, bad := range []string{"ended with a failure", "bought a"} {
		if strings.Contains(txt, bad) {
			t.Errorf("text says %q of a turn whose failed call was followed by a success:\n%s", bad, txt)
		}
	}
	if len(s.Savings) != 1 || s.Savings[0].Kind != SavingSilentFailure {
		t.Errorf("savings = %+v, want the silently-failed-turn line", s.Savings)
	}
}

// TestJoin_AnHonestSummaryDoesNotFire: the same recorded failure, with a final
// message that says so. The line is about summaries that mention NO failure,
// and a turn whose summary owns up is not in it.
func TestJoin_AnHonestSummaryDoesNotFire(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	silentSession(t, c, rec, now.Add(-2*time.Hour), "The first command failed; the second one worked.")
	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	if s.SilentFailureTurns.Turns != 0 || s.SilentFailureTurns.Cost.Nano != 0 {
		t.Errorf("an honest summary fired: %+v", s.SilentFailureTurns)
	}
}

// TestJoin_ATurnBeforeTheWindowIsNotCounted.
func TestJoin_ATurnBeforeTheWindowIsNotCounted(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	silentSession(t, c, rec, now.Add(-40*24*time.Hour), "Ran the command as requested.")
	// Something inside the window, so the session is in it at all.
	c.write("proj/sess-j/subagents/agent-late.jsonl", resp{id: "late", model: "claude-opus-5-5", session: "sess-j",
		at: now.Add(-time.Hour), in: 1, stop: "end_turn", sidechain: true}.line("text"))
	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	if s.SilentFailureTurns.Turns != 0 {
		t.Errorf("turns = %d, want 0: the silent turn ended 40 days ago, outside a 30-day window", s.SilentFailureTurns.Turns)
	}
}

// TestJoin_NoStoreIsUnknownNotZero: with no store, every session is not
// covered and the line says unknown, with no dollar figure at all.
func TestJoin_NoStoreIsUnknownNotZero(t *testing.T) {
	c := newConfig(t)
	c.write("proj/sess-a.jsonl", resp{id: "r", model: "claude-opus-5-5", at: now.Add(-time.Hour), in: 1e6, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if err := s.Join(nil); err != nil {
		t.Fatal(err)
	}
	j := s.SilentFailureTurns
	if j.Store != StoreNone || j.NotCoveredTranscripts != 1 || j.NotCoveredCost.Nano != 1e6*opusIn {
		t.Errorf("join = %+v, want store none and the one transcript not covered with its spend", j)
	}
	txt, _ := render(t, s)
	if !strings.Contains(txt, "never mentioned: unknown") ||
		!strings.Contains(txt, "rashomon has recorded nothing on this machine, so none of the 1 transcript is covered") {
		t.Errorf("text does not say the line is unknown:\n%s", txt)
	}
	if strings.Contains(txt, "across 0 turns") {
		t.Errorf("no store rendered as a checked zero:\n%s", txt)
	}
	// The JSON says the same: nothing checked is null, never {"usd": 0}.
	_, js := render(t, s)
	var doc struct {
		Silent map[string]json.RawMessage `json:"silent_failure_turns"`
	}
	if err := json.Unmarshal([]byte(js), &doc); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"turns", "unjudged_turns", "undeclared_failed_calls", "cost"} {
		if v, ok := doc.Silent[k]; !ok || string(v) != "null" {
			t.Errorf("silent_failure_turns.%s = %s, want null: no session was checked, and a zero reads as clean", k, v)
		}
	}
}

// TestJoin_ACoveredZeroIsAZero: once a session IS covered, a turn count of
// zero is a finding and marshals as one.
func TestJoin_ACoveredZeroIsAZero(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	silentSession(t, c, rec, now.Add(-2*time.Hour), "The first command failed; the second one worked.")
	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	txt, js := render(t, s)
	if !strings.Contains(js, `"turns":0,"unjudged_turns":0,"undeclared_failed_calls":0,"cost":{"usd":0,`) {
		t.Errorf("a covered, clean record did not marshal as a checked zero:\n%s", js)
	}
	// And the text says so plainly: "at least none across 0 turns" is a
	// floor of nothing over nothing.
	if !strings.Contains(txt, "never mentioned: none found (no recorded turn with a failed call ended in a summary that left it out; a failed Read, Glob, Grep or NotebookRead alone is not counted)\n") ||
		strings.Contains(txt, "at least") || strings.Contains(txt, "could not be checked") ||
		strings.Contains(txt, "is a floor") {
		t.Errorf("a covered, clean record is not rendered as a plain none:\n%s", txt)
	}
}

// TestJoin_AFiringTurnWithNoResponseInTheWindow: the turn's last record is
// inside the window, so it is judged, but every response the transcript ties
// to it started before the window. It fired, and nothing of it is priced:
// the text counts the turn and names no figure, never "at least none".
func TestJoin_AFiringTurnWithNoResponseInTheWindow(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	edge := now.Add(-30 * 24 * time.Hour)
	rec.transcript = filepath.Join(c.dir, "projects", "proj", "sess-j.jsonl")
	rec.call("sess-j", "p1", "toolu_1", edge.Add(-time.Minute), edge.Add(time.Minute), store.ExecFailed)
	c.write("proj/sess-j.jsonl",
		userLine("sess-j", "p1", edge.Add(-2*time.Minute), false),
		resp{id: "R0", model: "claude-opus-5-5", session: "sess-j", at: edge.Add(-90 * time.Second), in: 5, stop: "tool_use"}.line("tool_use"),
		userLine("sess-j", "p1", edge.Add(-85*time.Second), true),
		resp{id: "R1", model: "claude-opus-5-5", session: "sess-j", at: edge.Add(-80 * time.Second), in: 5, stop: "end_turn",
			text: "Ran it as requested."}.line("text"),
		userLine("sess-j", "p2", now.Add(-time.Hour), false),
		resp{id: "R2", model: "claude-opus-5-5", session: "sess-j", at: now.Add(-time.Hour), in: 5, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	if s.SilentFailureTurns.Turns != 1 {
		t.Fatalf("premise: turns = %d, want 1", s.SilentFailureTurns.Turns)
	}
	txt, _ := render(t, s)
	if !strings.Contains(txt, "never mentioned: 1 turn, with no response in the window tied to it") ||
		strings.Contains(txt, "at least none") {
		t.Errorf("a firing turn with nothing priced is rendered as a figure:\n%s", txt)
	}
}

// TestJoin_CoverageIsPerTranscript: a store that holds a run directory for a
// session id does not cover every conversation under that id. Measured: every
// record came from a headless project while the interactive transcript that
// carried the spend was never recorded, and a per-session rule called it
// covered. Here the store's records name one transcript of sess-j; the second
// transcript under the same id, and a session whose records name a file this
// package never discovered, are not covered, are named, and are priced.
func TestJoin_CoverageIsPerTranscript(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	T := now.Add(-2 * time.Hour)
	silentSession(t, c, rec, T, "The first command failed.")
	c.write("interactive/sess-j.jsonl",
		userLine("sess-j", "i1", T.Add(time.Hour), false),
		resp{id: "I1", model: "claude-opus-5-5", session: "sess-j", at: T.Add(time.Hour), in: 3000, stop: "end_turn"}.line("text"))
	// sess-k has a run directory, but its records name a transcript that is
	// not among the files discovered -- so its failed turn is not judged
	// either: there are no words of its to judge it by.
	rec.transcript = "/elsewhere/sess-k.jsonl"
	rec.call("sess-k", "k1", "toolu_k", T, T.Add(time.Second), store.ExecFailed)
	c.write("proj/sess-k.jsonl", resp{id: "K1", model: "claude-opus-5-5", session: "sess-k", at: T, in: 500, stop: "end_turn"}.line("text"))

	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	j := s.SilentFailureTurns
	if j.Transcripts != 3 || j.CoveredTranscripts != 1 || j.NotCoveredTranscripts != 2 {
		t.Errorf("transcripts %d, covered %d, not covered %d; want 3, 1, 2: only the file the records name is covered",
			j.Transcripts, j.CoveredTranscripts, j.NotCoveredTranscripts)
	}
	if j.Turns != 0 || j.Unjudged != 1 {
		t.Errorf("turns %d, unjudged %d; want 0 and 1: a turn whose records name no discovered transcript was judged on no words, "+
			"or went uncounted", j.Turns, j.Unjudged)
	}
	if j.NotCoveredCost.Nano != (3000+500)*opusIn {
		t.Errorf("not-covered cost = %d, want %d", j.NotCoveredCost.Nano, (3000+500)*opusIn)
	}
	if strings.Join(j.NotCoveredSessions, ",") != "sess-j,sess-k" {
		t.Errorf("not-covered sessions = %v, want sess-j (its interactive transcript) and sess-k", j.NotCoveredSessions)
	}
	cov := map[string]string{}
	for _, p := range s.PerSession {
		cov[p.SessionID] = p.Coverage
	}
	if cov["sess-j"] != CoveragePartly || cov["sess-k"] != CoverageNotRecorded {
		t.Errorf("per-session coverage = %v, want sess-j partly and sess-k not recorded", cov)
	}
	txt, _ := render(t, s)
	if !strings.Contains(txt, "1 of 3 transcripts was recorded") || !strings.Contains(txt, "(not-covered spend is in: sessions sess-j, sess-k)") ||
		!strings.Contains(txt, ", partly recorded by rashomon\n") {
		t.Errorf("text does not say which transcripts are covered:\n%s", txt)
	}
}

// TestJoin_ATurnsSpendIsKeyedByItsPrompt: a turn's recorded span can reach
// past the next prompt -- here p1's second call's execution is recorded at
// T+20s, after a tool-less prompt was answered at T+6s. Priced by span, that
// reply (Q1) was p1's spend; keyed by the prompt the transcript ties it to,
// it is not.
func TestJoin_ATurnsSpendIsKeyedByItsPrompt(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	T := now.Add(-2 * time.Hour)
	quiet := []string{
		userLine("sess-j", "p-quiet", T.Add(5*time.Second), false),
		resp{id: "Q1", model: "claude-opus-5-5", session: "sess-j", at: T.Add(6 * time.Second), in: 777777,
			stop: "end_turn", text: "Here you go."}.line("text"),
	}
	turn := silentSession(t, c, rec, T, "Ran the command as requested.", quiet...)
	rec.transcript = filepath.Join(c.dir, "projects", "proj", "sess-j.jsonl")
	rec.call("sess-j", "p1", "toolu_late", T.Add(3500*time.Millisecond), T.Add(20*time.Second), store.ExecOK)
	// Subagent responses inside p1's span that are not p1's: one its own
	// transcript ties to p2 (a subagent p2 spawned, whose call a record of
	// p1's turn cannot claim), one after a task line with no promptId, which
	// ties it to no turn at all, and one with no user line before it. The
	// record names the main transcript for every subagent call, so nothing
	// but the subagent file's own promptId can key its spend.
	sub := func(id string, in int64) string {
		r := resp{id: id, model: "claude-opus-5-5", session: "sess-j", at: T.Add(2 * time.Second), in: in, stop: "end_turn", sidechain: true}
		return r.line("tool_use")
	}
	c.write("proj/sess-j/subagents/agent-y.jsonl", subUserLine("sess-j", "p2", T.Add(time.Second)), sub("Y1", 55555))
	c.write("proj/sess-j/subagents/agent-z.jsonl",
		subUserLine("sess-j", "p1", T.Add(time.Second)), subUserLine("sess-j", "", T.Add(1500*time.Millisecond)), sub("Z1", 66666))
	c.write("proj/sess-j/subagents/agent-w.jsonl", sub("W1", 44444))
	// And one after a user line that does not decode: it may be a new prompt
	// this reader cannot key (here p2's), so it ends p1's tie.
	c.write("proj/sess-j/subagents/agent-v.jsonl", subUserLine("sess-j", "p1", T.Add(time.Second)),
		strings.Replace(subUserLine("sess-j", "p2", T.Add(1500*time.Millisecond)), `"timestamp":"`, `"timestamp":5,"x":"`, 1),
		sub("V1", 33333))
	// So does a user line cut short before its type, holding no "user" for a
	// byte test to find, and an undecodable usage line, which may follow a
	// prompt the reader never saw.
	c.write("proj/sess-j/subagents/agent-u.jsonl", subUserLine("sess-j", "p1", T.Add(time.Second)),
		`{"parentUuid":"u-1","isSidechain":true,"promptId":"p2","mess`,
		sub("U1", 22222))
	c.write("proj/sess-j/subagents/agent-t.jsonl", subUserLine("sess-j", "p1", T.Add(time.Second)),
		strings.Replace(sub("T0", 1), `"timestamp":"`, `"timestamp":5,"x":"`, 1),
		sub("T1", 11111))
	// A blank or whitespace-only line is no line at all: it keeps the tie,
	// so B1 is p1's.
	c.write("proj/sess-j/subagents/agent-b.jsonl", subUserLine("sess-j", "p1", T.Add(time.Second)), "", " \t ", sub("B1", 8888))
	turn += 8888 * opusIn
	rec.agentCall("sess-j", "p1", "toolu_z1", rec.transcript, T.Add(time.Second), T.Add(2*time.Second))
	rec.agentCall("sess-j", "p2", "toolu_y1", rec.transcript, T.Add(10*time.Second), T.Add(11*time.Second))
	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	if got := s.SilentFailureTurns.Cost.Nano; got != turn {
		t.Errorf("cost = %d, want %d: a response not tied to p1 was priced into it -- the tool-less prompt's reply "+
			"inside p1's recorded span, or a subagent response its own transcript does not tie to p1 -- or one after a "+
			"blank line was left out", got, turn)
	}
}

// TestJoin_ReadsTheStoreWithoutWritingIt: the join is a read.
func TestJoin_ReadsTheStoreWithoutWritingIt(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	silentSession(t, c, rec, now.Add(-2*time.Hour), "Ran the command as requested.")
	before := snapshot(t, rec.st.Root())
	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, rec.st.Root()); after != before {
		t.Errorf("the store changed during the join:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func snapshot(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		b.WriteString(p + " " + info.ModTime().String() + "\n")
		if !info.IsDir() {
			body, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			b.Write(body)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestJoin_OneSessionIDTwoConversations is the collision measured on a real
// machine: a headless run reusing an interactive session's id wrote a second
// main transcript under another project. The hooks recorded which file the
// turn's calls belonged to, and that file alone decides the verdict and the
// spend -- the other conversation's later reply (which says "error") and its
// response inside the same span belong to neither.
func TestJoin_OneSessionIDTwoConversations(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	T := now.Add(-2 * time.Hour)
	inside := silentSession(t, c, rec, T, "Ran the command as requested.")
	c.write("other-proj/sess-j.jsonl",
		userLine("sess-j", "o1", T.Add(time.Second), false),
		resp{id: "O1", model: "claude-opus-5-5", session: "sess-j", at: T.Add(2 * time.Second), in: 5000, stop: "tool_use"}.line("tool_use"),
		resp{id: "O2", model: "claude-opus-5-5", session: "sess-j", at: T.Add(5 * time.Second), in: 1, stop: "end_turn",
			text: "The build hit an error."}.line("text"))
	// A subagent file under the other conversation whose own line names p1:
	// the turn is priced in the transcripts its records name, and a key read
	// from a file outside them is not the turn's, whatever it says.
	c.write("other-proj/sess-j/subagents/agent-o.jsonl", subUserLine("sess-j", "p1", T.Add(time.Second)),
		resp{id: "OS1", model: "claude-opus-5-5", session: "sess-j", at: T.Add(2 * time.Second), in: 8000, stop: "end_turn",
			sidechain: true}.line("tool_use"))

	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	j := s.SilentFailureTurns
	if j.Turns != 1 {
		t.Fatalf("turns = %d, want 1: the verdict was taken on the other conversation's reply", j.Turns)
	}
	if j.Cost.Nano != inside {
		t.Errorf("cost = %d, want %d: the other conversation's response inside the span, or its subagent's, was priced into this turn",
			j.Cost.Nano, inside)
	}
}

// TestJoin_AToolLessTurnBetweenIsNotThisTurnsSummary: a prompt answered
// without any tool call leaves nothing in the store, so it is no turn of the
// join's -- but its reply is not the previous turn's final message either.
// The transcript ties every user line to its prompt, and the verdict is read
// from the words tied to THIS turn's.
//
// Both directions: an honest summary followed by a tool-less reply with no
// failure word must not fire, and a silent one followed by a tool-less reply
// that happens to say "error" must.
func TestJoin_AToolLessTurnBetweenIsNotThisTurnsSummary(t *testing.T) {
	for _, tc := range []struct {
		name, final1, quiet string
		turns               int
	}{
		{"honest then quiet", "The command failed with exit 1.", "Here is the summary you asked for: all done.", 0},
		{"silent then an error word", "Ran the command as requested.", "Earlier there was an error, sorry.", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfig(t)
			rec := newRecorder(t)
			T := now.Add(-2 * time.Hour)
			quiet := []string{
				userLine("sess-j", "p-quiet", T.Add(5*time.Second), false),
				resp{id: "Q1", model: "claude-opus-5-5", session: "sess-j", at: T.Add(6 * time.Second), in: 1,
					stop: "end_turn", text: tc.quiet}.line("text"),
			}
			silentSession(t, c, rec, T, tc.final1, quiet...)
			s := c.summary(30)
			if err := s.Join(rec.st); err != nil {
				t.Fatal(err)
			}
			if s.SilentFailureTurns.Turns != tc.turns {
				t.Errorf("turns = %d, want %d: the verdict was taken on the tool-less turn's reply", s.SilentFailureTurns.Turns, tc.turns)
			}
		})
	}
}

// TestJoin_AnUnkeyedPromptEndsTheTurnsWords: a prompt line with no promptId
// (an older transcript) cannot be keyed, and its reply is not credited to
// the turn before it. That turn then has no final message and no verdict --
// a floor, never the next prompt's words.
func TestJoin_AnUnkeyedPromptEndsTheTurnsWords(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	T := now.Add(-2 * time.Hour)
	unkeyed := []string{
		userLine("sess-j", "", T.Add(5*time.Second), false),
		resp{id: "Q1", model: "claude-opus-5-5", session: "sess-j", at: T.Add(6 * time.Second), in: 1,
			stop: "end_turn", text: "All good, nothing else to do."}.line("text"),
	}
	silentSession(t, c, rec, T, "The command failed with exit 1.", unkeyed...)
	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	if s.SilentFailureTurns.Turns != 0 {
		t.Errorf("turns = %d, want 0: an unkeyed prompt's reply was read as the previous turn's summary", s.SilentFailureTurns.Turns)
	}
}

// TestJoin_AFailedTurnWithNoWordsIsCountedUnchecked: a recorded turn with a
// failed call whose final message cannot be tied to its prompt takes no
// verdict -- and is not a checked none either. Here the transcript's prompt
// line carries no promptId, so nothing in it is p1's. The line used to read
// "none found (no recorded turn with a failed call ended in a summary that
// left it out)" and marshal turns 0 as a checked zero over a turn nobody
// judged.
func TestJoin_AFailedTurnWithNoWordsIsCountedUnchecked(t *testing.T) {
	for _, tc := range []struct {
		prompts []string
		said    string
	}{
		{[]string{"p1"}, "(1 turn with a failed call could not be checked: no final message could be tied to its prompt)"},
		{[]string{"p1", "p2"}, "(2 turns with a failed call could not be checked: no final message could be tied to their prompts)"},
	} {
		t.Run(strings.Join(tc.prompts, ","), func(t *testing.T) {
			c := newConfig(t)
			rec := newRecorder(t)
			T := now.Add(-2 * time.Hour)
			rec.transcript = filepath.Join(c.dir, "projects", "proj", "sess-j.jsonl")
			for i, p := range tc.prompts {
				rec.call("sess-j", p, "toolu_"+p, T.Add(time.Duration(i)*time.Minute), T.Add(time.Duration(i)*time.Minute+time.Second),
					store.ExecFailed)
			}
			c.write("proj/sess-j.jsonl",
				userLine("sess-j", "", T.Add(-time.Second), false),
				resp{id: "R1", model: "claude-opus-5-5", session: "sess-j", at: T.Add(2 * time.Second), in: 5, stop: "end_turn",
					text: "Ran it as requested."}.line("text"))
			s := c.summary(30)
			if err := s.Join(rec.st); err != nil {
				t.Fatal(err)
			}
			j := s.SilentFailureTurns
			if j.Turns != 0 || j.Unjudged != len(tc.prompts) {
				t.Errorf("turns %d, unjudged %d; want 0 and %d: the failed turns had no words to judge", j.Turns, j.Unjudged, len(tc.prompts))
			}
			txt, js := render(t, s)
			if strings.Contains(txt, "no recorded turn with a failed call ended in a summary that left it out") ||
				!strings.Contains(txt, "never mentioned: none found in the turns that could be checked") ||
				!strings.Contains(txt, tc.said) {
				t.Errorf("an unjudged failed turn is rendered as a checked none:\n%s", txt)
			}
			if !strings.Contains(js, fmt.Sprintf(`"unjudged_turns":%d`, len(tc.prompts))) {
				t.Errorf("the JSON does not count the unjudged turns:\n%s", js)
			}
		})
	}
}

// TestJoin_TheBoundNamesTheUnkeyedLinesThatKeepATie: the bound printed beside
// the figure says which responses it leaves out, so it must name exactly the
// user lines with no promptId that end a tie. In the main transcript a tool
// result or an injected meta line keeps it -- R1 and R2 are p1's and priced
// -- and only a prompt ends it (R3 is left out). In a subagent transcript a
// meta line keeps it and any other unkeyed line ends it (S1 is left out). The
// bound used to say every user line with no promptId ends the tie, printed
// beside a figure that had priced R1 and R2.
func TestJoin_TheBoundNamesTheUnkeyedLinesThatKeepATie(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	T := now.Add(-2 * time.Hour)
	sec := func(f float64) time.Time { return T.Add(time.Duration(f * float64(time.Second))) }
	rec.transcript = filepath.Join(c.dir, "projects", "proj", "sess-j.jsonl")
	rec.call("sess-j", "p1", "toolu_1", sec(0), sec(1), store.ExecFailed)
	m := func(id string, at time.Time, in int64, stop string) resp {
		return resp{id: id, model: "claude-opus-5-5", session: "sess-j", at: at, in: in, stop: stop, text: "Ran it as requested."}
	}
	meta := strings.Replace(userLine("sess-j", "", sec(3), false), `"isSidechain":false`, `"isMeta":true,"isSidechain":false`, 1)
	c.write("proj/sess-j.jsonl",
		userLine("sess-j", "p1", sec(-1), false),
		m("R0", sec(-0.5), 100, "tool_use").line("tool_use"),
		userLine("sess-j", "", sec(1.5), true),
		m("R1", sec(2), 7, "tool_use").line("tool_use"),
		meta,
		m("R2", sec(4), 3, "end_turn").line("text"),
		userLine("sess-j", "", sec(5), false),
		m("R3", sec(6), 1000, "end_turn").line("text"))
	subResult := strings.Replace(userLine("sess-j", "", sec(2.3), true), `"isSidechain":false`, `"isSidechain":true`, 1)
	sub := m("S1", sec(2.5), 50000, "tool_use")
	sub.sidechain = true
	c.write("proj/sess-j/subagents/agent-x.jsonl", subUserLine("sess-j", "p1", sec(2.2)), subResult, sub.line("tool_use"))

	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	j := s.SilentFailureTurns
	if j.Turns != 1 || j.Cost.Nano != (100+7+3)*opusIn {
		t.Errorf("turns %d, cost %d; want 1 and %d: R0, R1 and R2 are p1's, R3 and S1 are no turn's",
			j.Turns, j.Cost.Nano, (100+7+3)*opusIn)
	}
	txt, _ := render(t, s)
	if !strings.Contains(txt, "("+TurnBound+")") {
		t.Errorf("text does not print the bound beside the figure:\n%s", txt)
	}
	for _, want := range []string{"tool result", "meta line", "in the main transcript", "sidechain response", "sidechain user line does not end the tie", "cannot be decoded",
		"in the main transcript, unless it is a sidechain line"} {
		if !strings.Contains(TurnBound, want) {
			t.Errorf("the bound does not name %q among the lines that keep a tie, beside a figure that priced the response after one: %q", want, TurnBound)
		}
	}
}

// TestJoin_NoMessageTextReachesTheOutput: the path that reads message content
// -- a firing turn's final words, decoded to take the verdict -- is actually
// reached here (the turn fires), with the canary in those words and in every
// other block, and neither rendering carries a byte of it. The canary test in
// spend_test.go fills the silent-failure line by hand and never reaches that
// read, so a leak of the words it decodes passed it.
//
// A second recorded failed turn's final words name the failure and carry the
// canary: it does not fire, and its words must not reach the output either.
func TestJoin_NoMessageTextReachesTheOutput(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	T := now.Add(-2 * time.Hour)
	honest := []string{
		userLine("sess-j", "p-honest", T.Add(5*time.Second), false),
		resp{id: "H1", model: "claude-opus-5-5", session: "sess-j", at: T.Add(6 * time.Second), in: 1, stop: "end_turn",
			text: "The command failed with exit 1. " + canary}.line("text"),
	}
	silentSession(t, c, rec, T, "Ran the command as requested. "+canary, honest...)
	rec.call("sess-j", "p-honest", "toolu_h", T.Add(5500*time.Millisecond), T.Add(5600*time.Millisecond), store.ExecFailed)
	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	if s.SilentFailureTurns.Turns != 1 {
		t.Fatalf("premise: turns = %d, want 1 -- the final words were not read, so this proves nothing", s.SilentFailureTurns.Turns)
	}
	txt, js := render(t, s)
	for name, out := range map[string]string{"text": txt, "json": js} {
		if strings.Contains(out, canary) {
			t.Errorf("the content canary reached the %s output:\n%s", name, out)
		}
	}
}

// TestJoin_AMainTranscriptSidechainResponseIsItsTurns: a subagent's lines can
// be written into the main transcript itself (isSidechain). Its response is
// spend of the turn it runs in and is priced into the turn's figure; its
// words are never the main agent's final word; and its own user line, keyed
// or not, never moves the tie. The reader skipped every sidechain line, so
// SC1 fell out of the figure while the bound beside it said nothing past an
// unkeyed line was left out.
func TestJoin_AMainTranscriptSidechainResponseIsItsTurns(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	T := now.Add(-2 * time.Hour)
	sec := func(f float64) time.Time { return T.Add(time.Duration(f * float64(time.Second))) }
	rec.transcript = filepath.Join(c.dir, "projects", "proj", "sess-j.jsonl")
	rec.call("sess-j", "p1", "toolu_1", sec(0), sec(1), store.ExecFailed)
	m := func(id string, at time.Time, in int64, text string, side bool) string {
		return resp{id: id, model: "claude-opus-5-5", session: "sess-j", at: at, in: in, stop: "end_turn", text: text,
			sidechain: side}.line("text")
	}
	// The subagent's task line carries no promptId, and the turn's last line
	// is a subagent's: neither ends the tie, and the subagent's "error" is
	// not the main agent's final word.
	c.write("proj/sess-j.jsonl",
		userLine("sess-j", "p1", sec(-1), false),
		resp{id: "R0", model: "claude-opus-5-5", session: "sess-j", at: sec(-0.5), in: 100, stop: "tool_use"}.line("tool_use"),
		subUserLine("sess-j", "", sec(1.2)),
		subUserLine("sess-j", "p-sub", sec(1.5)),
		m("SC1", sec(2), 50000, "There was an error in the subagent.", true),
		m("R3", sec(4), 3, "Ran the command as requested.", false),
		m("SC2", sec(5), 7, "The subagent hit an error.", true))

	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	j := s.SilentFailureTurns
	if j.Turns != 1 {
		t.Fatalf("turns = %d, want 1: a sidechain line's words were read as the turn's final word, or its user line moved the tie", j.Turns)
	}
	if want := int64(100+50000+3+7) * opusIn; j.Cost.Nano != want {
		t.Errorf("cost = %d, want %d: the sidechain responses SC1 and SC2 are p1's spend", j.Cost.Nano, want)
	}
}

// TestJoin_AStoreThatCoversNoTranscriptIsUnknown: a store exists, but none of
// its records names a transcript in the window. Nothing was checked, so the
// line is unknown and says why -- never "none found", a checked-clean claim
// over zero checked transcripts.
func TestJoin_AStoreThatCoversNoTranscriptIsUnknown(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	T := now.Add(-2 * time.Hour)
	rec.transcript = "/elsewhere/sess-a.jsonl"
	rec.call("sess-a", "p1", "toolu_1", T, T.Add(time.Second), store.ExecOK)
	c.write("proj/sess-a.jsonl", resp{id: "r", model: "claude-opus-5-5", session: "sess-a", at: T, in: 5, stop: "end_turn"}.line("text"))
	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	if j := s.SilentFailureTurns; j.Store != StoreRead || j.CoveredTranscripts != 0 || j.NotCoveredTranscripts != 1 {
		t.Fatalf("premise: store %q, covered %d, not covered %d; want read, 0, 1", j.Store, j.CoveredTranscripts, j.NotCoveredTranscripts)
	}
	txt, _ := render(t, s)
	if !strings.Contains(txt, "never mentioned: unknown") ||
		!strings.Contains(txt, "rashomon recorded none of the 1 transcript, so none is covered") ||
		strings.Contains(txt, "none found") {
		t.Errorf("a store that covers no transcript is not rendered as unknown:\n%s", txt)
	}
}

// TestJoin_TheLastWordAcrossTwoMainFilesIsTheLatest: one session id can own
// two main transcripts, and a turn whose records name both is judged on the
// later of their final words (lastSaid). On a tie in time the file that sorts
// later wins, so the verdict does not depend on map order.
func TestJoin_TheLastWordAcrossTwoMainFilesIsTheLatest(t *testing.T) {
	const silent, honest = "Ran it as requested.", "The command failed."
	for _, tc := range []struct {
		name         string
		aText, bText string
		aAt, bAt     float64
		turns        int
	}{
		{"b is later and honest", silent, honest, 4, 5, 0},
		{"a is later and silent", silent, honest, 6, 5, 1},
		{"a tie goes to b, honest", silent, honest, 5, 5, 0},
		{"a tie goes to b, silent", honest, silent, 5, 5, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfig(t)
			rec := newRecorder(t)
			T := now.Add(-2 * time.Hour)
			sec := func(f float64) time.Time { return T.Add(time.Duration(f * float64(time.Second))) }
			file := func(proj, id, text string, at float64) string {
				rec.transcript = filepath.Join(c.dir, "projects", proj, "sess-j.jsonl")
				return c.write(proj+"/sess-j.jsonl", userLine("sess-j", "p1", sec(-1), false),
					resp{id: id, model: "claude-opus-5-5", session: "sess-j", at: sec(at), in: 1, stop: "end_turn", text: text}.line("text"))
			}
			file("proj-a", "A1", tc.aText, tc.aAt)
			rec.call("sess-j", "p1", "toolu_a", sec(0), sec(1), store.ExecFailed)
			file("proj-b", "B1", tc.bText, tc.bAt)
			rec.call("sess-j", "p1", "toolu_b", sec(2), sec(3), store.ExecOK)
			s := c.summary(30)
			if err := s.Join(rec.st); err != nil {
				t.Fatal(err)
			}
			if j := s.SilentFailureTurns; j.Turns != tc.turns || j.Unjudged != 0 {
				t.Errorf("turns %d, unjudged %d; want %d and 0: the verdict was not taken on the latest final word", j.Turns, j.Unjudged, tc.turns)
			}
		})
	}
}

// TestJoin_ATranscriptCutByAnOversizedLineIsUnjudged: a main transcript that
// cannot be read to the end may hold a later reply than any read, so none of
// its words is a turn's final word: the turn is counted as not checked, never
// judged on the earlier text.
func TestJoin_ATranscriptCutByAnOversizedLineIsUnjudged(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	T := now.Add(-2 * time.Hour)
	rec.transcript = c.write("proj/sess-j.jsonl",
		userLine("sess-j", "p1", T.Add(-time.Second), false),
		resp{id: "R1", model: "claude-opus-5-5", session: "sess-j", at: T.Add(2 * time.Second), in: 5, stop: "end_turn",
			text: "Ran it as requested."}.line("text"),
		`{"type":"assistant","x":"`+strings.Repeat("x", maxLine)+`"}`)
	rec.call("sess-j", "p1", "toolu_1", T, T.Add(time.Second), store.ExecFailed)
	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	if j := s.SilentFailureTurns; j.Turns != 0 || j.Unjudged != 1 {
		t.Errorf("turns %d, unjudged %d; want 0 and 1: a turn in a file not read to the end was judged on its earlier words", j.Turns, j.Unjudged)
	}
}

// TestJoin_ADeclarationWithNoPromptIDIsNoTurn: the store holds declarations
// with no prompt_id -- real records from before a session's first input. Such
// a call belongs to no turn: grouping it under "" would make every unkeyed
// assistant line a turn's final word and spend, and dereferencing the nil
// would panic. Here its call failed and the transcript's reply, tied to no
// prompt, says nothing of it: no turn is judged, nothing is priced.
func TestJoin_ADeclarationWithNoPromptIDIsNoTurn(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	T := now.Add(-2 * time.Hour)
	transcript := c.write("proj/sess-j.jsonl",
		resp{id: "R1", model: "claude-opus-5-5", session: "sess-j", at: T.Add(2 * time.Second), in: 5, stop: "end_turn",
			text: "Ran it as requested."}.line("text"))
	if err := rec.st.AppendDeclaration(store.Declaration{
		Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
		RecordedAtMS: T.UnixMilli(), ToolUseID: "toolu_1", SessionID: "sess-j",
		ToolName: "Bash", TranscriptPath: transcript,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rec.st.AppendExecution(store.Execution{
		Type: store.TypeExecution, SchemaVersion: store.SchemaVersion,
		RecordedAtMS: T.Add(time.Second).UnixMilli(), ToolUseID: "toolu_1", SessionID: "sess-j",
		ToolName: "Bash", Outcome: store.ExecFailed,
	}); err != nil {
		t.Fatal(err)
	}
	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	j := s.SilentFailureTurns
	if j.CoveredTranscripts != 1 {
		t.Fatalf("premise: covered %d, want 1: the record names the transcript", j.CoveredTranscripts)
	}
	if j.Turns != 0 || j.Unjudged != 0 || j.Cost.Priced != 0 || j.Cost.Unpriced != 0 {
		t.Errorf("turns %d, unjudged %d, cost %+v; want no turn and no spend: a call with no prompt_id is no turn's", j.Turns, j.Unjudged, j.Cost)
	}
}

// TestJoin_AWhollyUnpricedTurnSaysCostUnknown: every response of the firing
// turn ran on a model the table does not price. The line printed "at least
// unknown across 1 turn"; it says the cost is unknown and why.
func TestJoin_AWhollyUnpricedTurnSaysCostUnknown(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	T := now.Add(-2 * time.Hour)
	rec.transcript = c.write("proj/sess-j.jsonl",
		userLine("sess-j", "p1", T.Add(-time.Second), false),
		resp{id: "R1", model: "claude-mystery-1", session: "sess-j", at: T.Add(2 * time.Second), in: 1234, stop: "end_turn",
			text: "Ran it as requested."}.line("text"))
	rec.call("sess-j", "p1", "toolu_1", T, T.Add(time.Second), store.ExecFailed)
	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	if s.SilentFailureTurns.Turns != 1 {
		t.Fatalf("premise: turns = %d, want 1", s.SilentFailureTurns.Turns)
	}
	txt, _ := render(t, s)
	if !strings.Contains(txt, "never mentioned: cost unknown (1,234 tokens on 1 response with no known rate) across 1 turn\n") ||
		strings.Contains(txt, "at least unknown") {
		t.Errorf("a wholly unpriced turn is not said to be of unknown cost:\n%s", txt)
	}
}

// TestJoin_NoTranscriptInTheWindowIsSaidPlainly: a fresh install, or a quiet
// --days window, has nothing to cover. The line read "unknown ... none of the
// 0 transcripts"; it says there was no transcript.
func TestJoin_NoTranscriptInTheWindowIsSaidPlainly(t *testing.T) {
	for _, withStore := range []bool{false, true} {
		c := newConfig(t)
		s := c.summary(30)
		var st *store.Store
		if withStore {
			st = newRecorder(t).st
		}
		if err := s.Join(st); err != nil {
			t.Fatal(err)
		}
		txt, _ := render(t, s)
		if !strings.Contains(txt, "never mentioned: none: no transcript in the window\n") || strings.Contains(txt, "of the 0 transcripts") {
			t.Errorf("store %v: an empty window is not said plainly:\n%s", withStore, txt)
		}
	}
}

// TestJoin_ADuplicatedResponseIsCoveredOnlyWhenEveryTranscriptHoldingItIs: a
// resumed or branched conversation (/branch, --fork-session) carries the
// original's responses into its own file in the same project folder. Only
// the copy was recorded. Pinned to whichever file sorted first, a shared
// response made the unrecorded original read "recorded" in one path order --
// it held no response of its own -- and the copy's first new response lost
// its predecessor in the other, so its cold write went uncounted. Both orders
// now give the same answer: the original is not covered, the shared
// responses' cost is counted once as not covered, and the cold write is
// counted once.
//
// With the copied lines keeping the original's sessionId, and with them
// carrying the copy's own. /branch keeps the original timestamps on every
// copied line, so the two files start at the same moment, and ties broken by
// session id followed the ids: a copy whose id sorted first took the shared
// responses, and the original's session left the rows and the header while
// its dollars sat in a row that read recorded. A shared response now belongs
// to every session whose file's first dated line is earliest -- on a tie, to
// both -- and every session holding one has a row. So each case gives two
// sessions, the original not recorded, and the copy partly recorded when it
// holds the original's not-covered dollars. An undated first line on the
// original (a file-history snapshot) changes nothing: a file is dated by its
// first dated line.
func TestJoin_ADuplicatedResponseIsCoveredOnlyWhenEveryTranscriptHoldingItIs(t *testing.T) {
	T := now.Add(-3 * time.Hour)
	for _, tc := range []struct {
		name         string
		copy         string // the copy's session id
		keepID       bool   // the copied lines keep the original's sessionId
		undatedFirst bool   // the original starts with an undated line
		copyCoverage string
		named        string // the rows holding the not-covered dollars
		shared       int    // the responses both sessions hold
	}{
		{"copied lines keep sess-o", "sess-r", true, false, CoverageRecorded, "sess-o", 0},
		{"copied lines carry sess-r", "sess-r", false, false, CoveragePartly, "sess-o,sess-r", 2},
		{"copied lines carry sess-a, which sorts first", "sess-a", false, false, CoveragePartly, "sess-a,sess-o", 2},
		{"copied lines carry sess-a, the original's first line undated", "sess-a", false, true, CoveragePartly, "sess-a,sess-o", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x1 := resp{id: "X1", model: "claude-opus-5-5", session: "sess-o", at: T, in: 1000, w5: 100, stop: "end_turn"}
			x2 := resp{id: "X2", model: "claude-opus-5-5", session: "sess-o", at: T.Add(oneMinute), in: 2000, w5: 3000, stop: "end_turn"}
			y1 := resp{id: "Y1", model: "claude-opus-5-5", session: tc.copy, at: T.Add(30 * oneMinute), in: 4000, w5: 3000, stop: "end_turn"}
			cx1, cx2 := x1, x2
			if !tc.keepID {
				cx1.session, cx2.session = tc.copy, tc.copy
			}
			original := []string{x1.line("text"), x2.line("text")}
			if tc.undatedFirst {
				original = append([]string{`{"type":"file-history-snapshot","messageId":"u-0","snapshot":{}}`}, original...)
			}
			run := func(origDir, copyDir string) (string, string, *Summary) {
				c := newConfig(t)
				rec := newRecorder(t)
				c.write(origDir+"/sess-o.jsonl", original...)
				rec.transcript = c.write(copyDir+"/"+tc.copy+".jsonl", cx1.line("text"), cx2.line("text"), y1.line("text"))
				rec.call(tc.copy, "p1", "toolu_r", T.Add(30*oneMinute), T.Add(31*oneMinute), store.ExecOK)
				s := c.summary(30)
				if err := s.Join(rec.st); err != nil {
					t.Fatal(err)
				}
				txt, _ := render(t, s)
				j, err := json.Marshal(struct {
					J SilentFailureTurns
					P []SessionSpend
					C CacheExpiry
				}{s.SilentFailureTurns, s.PerSession, s.CacheExpiry})
				if err != nil {
					t.Fatal(err)
				}
				return txt, string(j), s
			}
			txtA, jsA, s := run("a-proj", "b-proj")
			txtB, jsB, _ := run("b-proj", "a-proj")
			if jsA != jsB {
				t.Errorf("coverage, sessions and cold cache follow the path order:\n%s\n%s", jsA, jsB)
			}
			strip := func(s string) string { return strings.ReplaceAll(s, "a-proj", "b-proj") }
			if strip(txtA) != strip(txtB) {
				t.Errorf("the text follows the path order:\n%s\n%s", txtA, txtB)
			}
			j := s.SilentFailureTurns
			if j.Transcripts != 2 || j.CoveredTranscripts != 1 || j.NotCoveredTranscripts != 1 {
				t.Errorf("transcripts %d, covered %d, not covered %d; want 2, 1, 1: the unrecorded original is never recorded",
					j.Transcripts, j.CoveredTranscripts, j.NotCoveredTranscripts)
			}
			if want := int64(3000*opusIn + 3100*opusW5); j.NotCoveredCost.Nano != want {
				t.Errorf("not-covered cost = %d, want %d: the shared responses, once", j.NotCoveredCost.Nano, want)
			}
			if strings.Join(j.NotCoveredSessions, ",") != tc.named {
				t.Errorf("not-covered sessions = %v, want %s, the rows holding the shared responses", j.NotCoveredSessions, tc.named)
			}
			cov := map[string]string{}
			for _, p := range s.PerSession {
				cov[p.SessionID] = p.Coverage
			}
			if cov["sess-o"] != CoverageNotRecorded || cov[tc.copy] != tc.copyCoverage || s.Sessions != 2 {
				t.Errorf("sessions %d, coverage %v; want 2, sess-o not recorded and %s %s", s.Sessions, cov, tc.copy, tc.copyCoverage)
			}
			if !strings.Contains(txtA, "SPEND  last 30 days · 2 sessions ·") || !strings.Contains(txtA, "sess-o $0.03 (main $0.03, subagents none), not recorded by rashomon\n") {
				t.Errorf("the header does not count both sessions, or the original's row is not its own:\n%s", txtA)
			}
			if want := int64(3000*opusIn + 3100*opusW5 + 4000*opusIn + 3000*opusW5); s.Total.Nano != want {
				t.Errorf("total = %d, want %d: each response once", s.Total.Nano, want)
			}
			if s.CacheExpiry.Responses != 1 || s.CacheExpiry.Tokens != 3000 {
				t.Errorf("cold = %+v, want Y1's write once", s.CacheExpiry)
			}
			if s.SharedResponses != tc.shared {
				t.Errorf("shared responses = %d, want %d", s.SharedResponses, tc.shared)
			}
			if tc.shared == 0 && strings.Contains(txtA, "held by tied sessions") {
				t.Errorf("the text names shared responses where one session owns each:\n%s", txtA)
			}
			if tc.shared > 0 && !strings.Contains(txtA, "\n              2 responses are held by tied sessions and appear in each of their rows, so the rows can add up to more than the total\n") {
				t.Errorf("the text does not say the two shared responses are in each row:\n%s", txtA)
			}
		})
	}
}

// TestJoin_ASharedResponseIsNeverBothCountedAndNotCovered: the original was
// recorded and its failed turn fires; an unrecorded copy (keeping the
// original's sessionId, under the copy's own file name) holds the same
// response. Its dollars are the turn's figure, and they were also printed as
// not covered -- the same response in "at least $X" and in "$X in the other
// 1 is not covered" -- while the recorded original read partly recorded. A
// response a covered turn counted is not also not covered, and the original
// session reads recorded. No row holds a not-covered dollar, so none is
// named: the copy's own session has no row.
func TestJoin_ASharedResponseIsNeverBothCountedAndNotCovered(t *testing.T) {
	c := newConfig(t)
	rec := newRecorder(t)
	T := now.Add(-2 * time.Hour)
	lines := []string{
		userLine("sess-o", "p1", T.Add(-time.Second), false),
		resp{id: "X1", model: "claude-opus-5-5", session: "sess-o", at: T.Add(2 * time.Second), in: 1000, stop: "end_turn",
			text: "Ran it as requested."}.line("text"),
	}
	rec.transcript = c.write("proj/sess-o.jsonl", lines...)
	c.write("proj/sess-c.jsonl", lines...)
	rec.call("sess-o", "p1", "toolu_1", T, T.Add(time.Second), store.ExecFailed)
	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	j := s.SilentFailureTurns
	if j.Turns != 1 || j.Cost.Nano != 1000*opusIn {
		t.Fatalf("premise: turns %d, cost %d; want 1 and %d", j.Turns, j.Cost.Nano, 1000*opusIn)
	}
	if j.NotCoveredTranscripts != 1 || j.NotCoveredCost.Priced != 0 || j.NotCoveredCost.Unpriced != 0 {
		t.Errorf("not covered: %d transcripts, cost %+v; want the copy counted with no dollars, which the turn already counted",
			j.NotCoveredTranscripts, j.NotCoveredCost)
	}
	if len(j.NotCoveredSessions) != 0 {
		t.Errorf("not-covered sessions = %v, want none: no row holds a not-covered dollar", j.NotCoveredSessions)
	}
	if len(s.PerSession) != 1 || s.PerSession[0].SessionID != "sess-o" || s.PerSession[0].Coverage != CoverageRecorded {
		t.Errorf("per session = %+v, want sess-o recorded", s.PerSession)
	}
}

// TestJoin_ASharedResponseInTwoUnrecordedTranscriptsIsNotCoveredOnce: neither
// the original nor its copy was recorded. The shared response is not covered
// in both, and its cost is in not_covered_cost once.
func TestJoin_ASharedResponseInTwoUnrecordedTranscriptsIsNotCoveredOnce(t *testing.T) {
	c := newConfig(t)
	x := resp{id: "X1", model: "claude-opus-5-5", session: "sess-o", at: now.Add(-time.Hour), in: 1000, stop: "end_turn"}.line("text")
	c.write("proj/sess-o.jsonl", x)
	c.write("proj/sess-c.jsonl", x)
	s := c.summary(30)
	if err := s.Join(newRecorder(t).st); err != nil {
		t.Fatal(err)
	}
	j := s.SilentFailureTurns
	if j.NotCoveredTranscripts != 2 || j.NotCoveredCost.Nano != 1000*opusIn || j.NotCoveredCost.Priced != 1 {
		t.Errorf("not covered: %d transcripts, cost %+v; want 2, and the shared response's %d once", j.NotCoveredTranscripts, j.NotCoveredCost, 1000*opusIn)
	}
}

// TestJoin_AFailedCallWhoseDeclarationWasLostIsNotJudged: a declaration can
// be lost (a lock timeout, a paused pre hook, a mid-session install) while
// its failed execution is recorded. No tool_use_id ties the execution to a
// prompt, so it is placed in no turn: placed by recorded time, it landed in
// the clean turn before it, which fired and printed p1's spend as
// silent-failure spend and as a saving. It is counted once as a failed call
// that was not checked, so "none found" is never printed over it -- and only
// when it is a failure recorded in the window. A declaration recorded with no
// prompt_id (a record from before a session's first input) ties the call to
// no turn either, and is counted the same way. A turn that fires beside such
// a call keeps its own figure, with the note printed above the bound.
func TestJoin_AFailedCallWhoseDeclarationWasLostIsNotJudged(t *testing.T) {
	type lost struct {
		at      time.Duration
		outcome string
	}
	const one = "  (1 failed call could not be checked: its declaration was not recorded or carried no prompt id, so no turn is known to hold it)\n"
	for _, tc := range []struct {
		name       string
		lost       []lost
		third      bool // p2 declares no call, and a clean p3 follows it
		promptless bool // each lost call's declaration is recorded, with no prompt_id
		fails      bool // p1's call fails, so p1 fires
		want       int
		said       string
	}{
		{"right after a clean turn", []lost{{9800 * time.Millisecond, store.ExecFailed}}, false, false, false, 1, one},
		{"a turn with every declaration lost", []lost{{10800 * time.Millisecond, store.ExecFailed}}, true, false, false, 1, one},
		{"two in one turn", []lost{{10800 * time.Millisecond, store.ExecFailed}, {10900 * time.Millisecond, store.ExecFailed}}, true, false, false, 2,
			"  (2 failed calls could not be checked: their declarations were not recorded or carried no prompt id, so no turn is known to hold them)\n"},
		{"before every turn", []lost{{-5 * time.Second, store.ExecFailed}}, false, false, false, 1, one},
		{"a success, and a failure before the window", []lost{{10800 * time.Millisecond, store.ExecOK}, {-31 * 24 * time.Hour, store.ExecFailed}}, false, false, false, 0, ""},
		{"a promptless failure", []lost{{10800 * time.Millisecond, store.ExecFailed}}, false, true, false, 1, one},
		{"a promptless success", []lost{{10800 * time.Millisecond, store.ExecOK}}, false, true, false, 0, ""},
		{"beside a turn that fires", []lost{{10800 * time.Millisecond, store.ExecFailed}}, true, false, true, 1, one},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfig(t)
			rec := newRecorder(t)
			T := now.Add(-2 * time.Hour)
			sec := func(f float64) time.Time { return T.Add(time.Duration(f * float64(time.Second))) }
			m := func(id string, at time.Time, in int64, text string) resp {
				return resp{id: id, model: "claude-opus-5-5", session: "sess-j", at: at, in: in, stop: "end_turn", text: text}
			}
			lines := []string{
				userLine("sess-j", "p1", sec(-1), false),
				m("R1", sec(0.5), 100000, "").line("tool_use"),
				userLine("sess-j", "p1", sec(1), true),
				m("R2", sec(3), 1000000, "Ran the command as requested.").line("text"),
				userLine("sess-j", "p2", sec(9.5), false),
				m("R3", sec(10.5), 100, "").line("tool_use"),
				userLine("sess-j", "p2", sec(11), true),
				m("R4", sec(12), 1000, "All done.").line("text"),
			}
			if tc.third {
				lines = append(lines,
					userLine("sess-j", "p3", sec(19.5), false),
					m("R5", sec(20.5), 10, "").line("tool_use"),
					userLine("sess-j", "p3", sec(21), true),
					m("R6", sec(22), 10, "Finished as asked.").line("text"))
			}
			rec.transcript = c.write("proj/sess-j.jsonl", lines...)
			p1 := store.ExecOK
			if tc.fails {
				p1 = store.ExecFailed
			}
			rec.call("sess-j", "p1", "toolu_1", sec(0), sec(1), p1)
			if tc.third {
				rec.call("sess-j", "p3", "toolu_5", sec(20), sec(21), store.ExecOK)
			} else {
				rec.call("sess-j", "p2", "toolu_3", sec(10), sec(11), store.ExecOK)
			}
			for i, l := range tc.lost {
				id := fmt.Sprintf("toolu_lost%d", i)
				if tc.promptless {
					if err := rec.st.AppendDeclaration(store.Declaration{
						Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
						RecordedAtMS: T.Add(l.at - 100*time.Millisecond).UnixMilli(), ToolUseID: id, SessionID: "sess-j",
						ToolName: "Bash", TranscriptPath: rec.transcript,
					}); err != nil {
						t.Fatal(err)
					}
				}
				if err := rec.st.AppendExecution(store.Execution{
					Type: store.TypeExecution, SchemaVersion: store.SchemaVersion,
					RecordedAtMS: T.Add(l.at).UnixMilli(), ToolUseID: id, SessionID: "sess-j",
					ToolName: "Bash", Outcome: l.outcome,
				}); err != nil {
					t.Fatal(err)
				}
			}
			s := c.summary(30)
			if err := s.Join(rec.st); err != nil {
				t.Fatal(err)
			}
			j := s.SilentFailureTurns
			txt, js := render(t, s)
			if !strings.Contains(js, fmt.Sprintf(`"undeclared_failed_calls":%d`, tc.want)) {
				t.Errorf("the JSON does not count the undeclared failed calls:\n%s", js)
			}
			if tc.fails {
				if want := int64(100000+1000000) * opusIn; j.Turns != 1 || j.Cost.Nano != want || j.Unjudged != 0 || j.UndeclaredFailedCalls != 1 {
					t.Errorf("turns %d, cost %d, unjudged %d, undeclared %d; want p1 alone, priced at %d, and 1 undeclared",
						j.Turns, j.Cost.Nano, j.Unjudged, j.UndeclaredFailedCalls, want)
				}
				note, bound := strings.Index(txt, tc.said), strings.Index(txt, "  ("+TurnBound+")\n")
				if !strings.Contains(txt, "never mentioned: at least $4.40 across 1 turn\n") || note < 0 || bound < note {
					t.Errorf("want p1's figure, and the note above the bound:\n%s", txt)
				}
				return
			}
			if j.Turns != 0 || j.Cost.Priced != 0 || len(s.Savings) != 0 || j.Unjudged != 0 || j.UndeclaredFailedCalls != tc.want {
				t.Errorf("turns %d, cost %+v, savings %+v, unjudged %d, undeclared %d; want no turn, no figure and %d undeclared",
					j.Turns, j.Cost, s.Savings, j.Unjudged, j.UndeclaredFailedCalls, tc.want)
			}
			if tc.want == 0 {
				if !strings.Contains(txt, "never mentioned: none found (no recorded turn") || strings.Contains(txt, "could not be checked") {
					t.Errorf("a lost successful call, or a failure recorded before the window, is counted:\n%s", txt)
				}
				return
			}
			if !strings.Contains(txt, "never mentioned: none found in the turns that could be checked (a failed Read, Glob, Grep or NotebookRead alone is not counted)\n") || !strings.Contains(txt, tc.said) {
				t.Errorf("an undeclared failed call is rendered as a checked none:\n%s", txt)
			}
		})
	}
}

// orders runs a fixture written under two project folders in both path
// orders, and fails unless both give the same rows, labels and header.
func orders(t *testing.T, build func(c *config, first, second string) *store.Store) *Summary {
	t.Helper()
	var out []*Summary
	var rows []string
	for _, dirs := range [][2]string{{"a-proj", "b-proj"}, {"b-proj", "a-proj"}} {
		c := newConfig(t)
		st := build(c, dirs[0], dirs[1])
		s := c.summary(30)
		if err := s.Join(st); err != nil {
			t.Fatal(err)
		}
		j, err := json.Marshal(struct {
			N int
			P []SessionSpend
			J SilentFailureTurns
		}{s.Sessions, s.PerSession, s.SilentFailureTurns})
		if err != nil {
			t.Fatal(err)
		}
		out, rows = append(out, s), append(rows, string(j))
	}
	if rows[0] != rows[1] {
		t.Errorf("the rows follow the path order:\n%s\n%s", rows[0], rows[1])
	}
	return out[0]
}

// row is a summary's per-session row for id, or nil.
func row(s *Summary, id string) *SessionSpend {
	for i := range s.PerSession {
		if s.PerSession[i].SessionID == id {
			return &s.PerSession[i]
		}
	}
	return nil
}

// TestJoin_ASharedResponseBelongsToTheFileFirstDated: a shared response
// belongs to the session whose file's first dated line is earliest, whatever
// the session ids. sess-z's file starts with an undated line and then a
// prompt ten minutes before the response; sess-b's copy starts at the
// response. Dated by its literal first line, sess-z's file sorted last and
// the copy took the response. sess-b still has its row -- it holds the
// response -- with none of its dollars, and its recorded transcript is read
// although it owns nothing.
func TestJoin_ASharedResponseBelongsToTheFileFirstDated(t *testing.T) {
	T := now.Add(-2 * time.Hour)
	x := resp{id: "X1", model: "claude-opus-5-5", at: T, in: 1000, stop: "end_turn"}
	s := orders(t, func(c *config, first, second string) *store.Store {
		xz, xb := x, x
		xz.session, xb.session = "sess-z", "sess-b"
		c.write(first+"/sess-z.jsonl", `{"type":"file-history-snapshot","messageId":"u-0","snapshot":{}}`,
			userLine("sess-z", "p0", T.Add(-10*time.Minute), false), xz.line("text"))
		rec := newRecorder(t)
		rec.transcript = c.write(second+"/sess-b.jsonl", xb.line("text"))
		rec.call("sess-b", "p1", "toolu_b", T, T.Add(time.Second), store.ExecOK)
		return rec.st
	})
	z, b := row(s, "sess-z"), row(s, "sess-b")
	if z == nil || b == nil || s.Sessions != 2 {
		t.Fatalf("rows = %+v, want sess-z and sess-b", s.PerSession)
	}
	if z.Main.Nano != 1000*opusIn || b.Main.Priced != 0 || b.Main.Unpriced != 0 {
		t.Errorf("sess-z = %+v, sess-b = %+v; want the response in sess-z's row alone", z.Main, b.Main)
	}
	if txt, _ := render(t, s); s.SharedResponses != 0 || strings.Contains(txt, "held by tied sessions") {
		t.Errorf("shared responses = %d, want 0: one session owns the response\n%s", s.SharedResponses, txt)
	}
	if z.Coverage != CoverageNotRecorded || b.Coverage != CoverageRecorded || s.SilentFailureTurns.CoveredTranscripts != 1 {
		t.Errorf("sess-z %s, sess-b %s, covered %d; want not recorded, recorded and 1: the copy's record was not read",
			z.Coverage, b.Coverage, s.SilentFailureTurns.CoveredTranscripts)
	}
}

// TestJoin_AnAbsentOriginalsRowIsNotRecorded: a copy whose lines keep the
// original's sessionId, with the original absent and the copy unrecorded.
// Coverage was tallied by each file's own session, so the sess-o row, which
// names no discovered file, got no label and printed like a recorded one.
func TestJoin_AnAbsentOriginalsRowIsNotRecorded(t *testing.T) {
	T := now.Add(-2 * time.Hour)
	s := orders(t, func(c *config, first, second string) *store.Store {
		c.write(first+"/sess-c.jsonl", resp{id: "X1", model: "claude-opus-5-5", session: "sess-o", at: T, in: 1000, stop: "end_turn"}.line("text"))
		rec := newRecorder(t)
		rec.transcript = c.write(second+"/sess-q.jsonl", resp{id: "Q1", model: "claude-opus-5-5", session: "sess-q", at: T, in: 10, stop: "end_turn"}.line("text"))
		rec.call("sess-q", "p1", "toolu_q", T, T.Add(time.Second), store.ExecOK)
		return rec.st
	})
	if o := row(s, "sess-o"); o == nil || o.Coverage != CoverageNotRecorded {
		t.Errorf("rows = %+v, want sess-o not recorded", s.PerSession)
	}
	if n := s.SilentFailureTurns.NotCoveredSessions; strings.Join(n, ",") != "sess-o" {
		t.Errorf("not-covered sessions = %v, want sess-o, the row holding the not-covered dollars", n)
	}
	txt, _ := render(t, s)
	if !strings.Contains(txt, "sess-o <$0.01 (main <$0.01, subagents none), not recorded by rashomon\n") {
		t.Errorf("the text does not mark sess-o not recorded:\n%s", txt)
	}
}

// TestJoin_ARecordedOriginalWithAnUnrecordedCopy: the original was recorded
// and an unrecorded /branch copy, under its own id, holds its responses with
// the same timestamps, and no turn fires. Both sessions hold the response,
// so both have a row in either path order: the original partly recorded --
// its dollars are not covered, since the copy holding them was not recorded
// -- and the copy not recorded.
func TestJoin_ARecordedOriginalWithAnUnrecordedCopy(t *testing.T) {
	T := now.Add(-2 * time.Hour)
	x := resp{id: "X1", model: "claude-opus-5-5", at: T, in: 1000, stop: "end_turn"}
	s := orders(t, func(c *config, first, second string) *store.Store {
		xo, xc := x, x
		xo.session, xc.session = "sess-o", "sess-c"
		rec := newRecorder(t)
		rec.transcript = c.write(first+"/sess-o.jsonl", xo.line("text"))
		c.write(second+"/sess-c.jsonl", xc.line("text"))
		rec.call("sess-o", "p1", "toolu_o", T, T.Add(time.Second), store.ExecOK)
		return rec.st
	})
	o, c := row(s, "sess-o"), row(s, "sess-c")
	if o == nil || c == nil || o.Coverage != CoveragePartly || c.Coverage != CoverageNotRecorded || s.Sessions != 2 {
		t.Errorf("rows = %+v; want sess-o partly recorded and sess-c not recorded", s.PerSession)
	}
	if s.Total.Nano != 1000*opusIn || s.SilentFailureTurns.NotCoveredCost.Nano != 1000*opusIn {
		t.Errorf("total %d, not covered %d; want the shared response once in each", s.Total.Nano, s.SilentFailureTurns.NotCoveredCost.Nano)
	}
	// The response is in both rows and once in the total, and the output
	// says so: a reader summing per_session would count it twice.
	txt, js := render(t, s)
	if s.SharedResponses != 1 || !strings.Contains(js, `"shared_responses":1`) ||
		!strings.Contains(txt, "\n              1 response is held by tied sessions and appears in each of their rows, so the rows can add up to more than the total\n") {
		t.Errorf("shared responses = %d; the output does not say the rows can add up to more than the total:\n%s\n%s", s.SharedResponses, txt, js)
	}
}

// TestJoin_AnUnrecordedCopyOfAFiringTurnIsNotRecorded: the original was
// recorded and its failed turn fires; an unrecorded /branch copy under its
// own id holds the turn's responses with the same timestamps, so both
// sessions own them. The turn counted those responses, and a counted
// response marked every owner's row as covered, so the copy read "partly
// recorded" though rashomon recorded nothing of it.
func TestJoin_AnUnrecordedCopyOfAFiringTurnIsNotRecorded(t *testing.T) {
	T := now.Add(-2 * time.Hour)
	lines := func(id string) []string {
		return []string{
			userLine(id, "p1", T.Add(-time.Second), false),
			resp{id: "X1", model: "claude-opus-5-5", session: id, at: T.Add(500 * time.Millisecond), in: 1000, stop: "tool_use"}.line("tool_use"),
			userLine(id, "p1", T.Add(time.Second), true),
			resp{id: "X2", model: "claude-opus-5-5", session: id, at: T.Add(3 * time.Second), in: 1000, stop: "end_turn",
				text: "Ran the command as requested."}.line("text"),
		}
	}
	s := orders(t, func(c *config, first, second string) *store.Store {
		rec := newRecorder(t)
		rec.transcript = c.write(first+"/sess-o.jsonl", lines("sess-o")...)
		c.write(second+"/sess-c.jsonl", lines("sess-c")...)
		rec.call("sess-o", "p1", "toolu_o", T, T.Add(time.Second), store.ExecFailed)
		return rec.st
	})
	if j := s.SilentFailureTurns; j.Turns != 1 || j.Cost.Nano != 2000*opusIn {
		t.Fatalf("premise: turns %d, cost %d; want the turn to fire on both responses", j.Turns, j.Cost.Nano)
	}
	o, c := row(s, "sess-o"), row(s, "sess-c")
	if o == nil || c == nil || o.Coverage != CoverageRecorded || c.Coverage != CoverageNotRecorded {
		t.Errorf("rows = %+v; want sess-o recorded and sess-c not recorded", s.PerSession)
	}
	txt, _ := render(t, s)
	if strings.Contains(txt, "partly recorded") || !strings.Contains(txt, "sess-c <$0.01 (main <$0.01, subagents none), not recorded by rashomon\n") {
		t.Errorf("the unrecorded copy is not marked not recorded:\n%s", txt)
	}
}

// TestJoin_ASightingSessionWithNoFileIsNotRecorded: a response is owned by
// the file first dated, sess-a's; a later file also holds it under lines
// carrying sess-x, a session no discovered file is named for. Both files
// were recorded, but sess-x, which has a row, has no transcript of its own,
// so it is not recorded -- with no tally it had no label and printed like a
// recorded row.
func TestJoin_ASightingSessionWithNoFileIsNotRecorded(t *testing.T) {
	T := now.Add(-2 * time.Hour)
	s := orders(t, func(c *config, first, second string) *store.Store {
		rec := newRecorder(t)
		rec.transcript = c.write(first+"/sess-a.jsonl", userLine("sess-a", "p0", T.Add(-10*time.Minute), false),
			resp{id: "X1", model: "claude-opus-5-5", session: "sess-a", at: T, in: 1000, stop: "end_turn"}.line("text"))
		rec.call("sess-a", "p1", "toolu_a", T, T.Add(time.Second), store.ExecOK)
		rec.transcript = c.write(second+"/sess-b.jsonl",
			resp{id: "X1", model: "claude-opus-5-5", session: "sess-x", at: T, in: 1000, stop: "end_turn"}.line("text"))
		rec.call("sess-x", "p2", "toolu_b", T, T.Add(time.Second), store.ExecOK)
		return rec.st
	})
	x := row(s, "sess-x")
	if x == nil || x.Main != (Cost{}) || x.Coverage != CoverageNotRecorded {
		t.Errorf("rows = %+v; want a sess-x row with no dollars, not recorded", s.PerSession)
	}
	if a := row(s, "sess-a"); a == nil || a.Coverage != CoverageRecorded {
		t.Errorf("rows = %+v; want sess-a recorded", s.PerSession)
	}
}

// TestJoin_ARowWhoseResponsesSitInACoveredCopyIsRecorded: a recorded
// sess-c.jsonl holds responses whose lines carry sessionId sess-o, and no
// sess-o.jsonl was discovered. The sess-o row names no file of its own, but
// every transcript holding its responses was recorded: it read "not
// recorded by rashomon" beside "1 of 1 transcript was recorded", and, when
// the turn fired, a row whose whole spend is in the silent-failure figure
// read not recorded too. It is recorded in both cases.
func TestJoin_ARowWhoseResponsesSitInACoveredCopyIsRecorded(t *testing.T) {
	T := now.Add(-2 * time.Hour)
	for _, tc := range []struct {
		name    string
		lines   []string
		outcome string
	}{
		{"no turn fires", []string{
			resp{id: "X1", model: "claude-opus-5-5", session: "sess-o", at: T, in: 1000, stop: "end_turn"}.line("text")}, store.ExecOK},
		{"the turn fires", []string{
			userLine("sess-o", "p1", T.Add(-time.Second), false),
			resp{id: "X1", model: "claude-opus-5-5", session: "sess-o", at: T.Add(500 * time.Millisecond), in: 1000, stop: "tool_use"}.line("tool_use"),
			userLine("sess-o", "p1", T.Add(time.Second), true),
			resp{id: "X2", model: "claude-opus-5-5", session: "sess-o", at: T.Add(3 * time.Second), in: 1000, stop: "end_turn",
				text: "Ran the command as requested."}.line("text"),
		}, store.ExecFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := orders(t, func(c *config, first, second string) *store.Store {
				rec := newRecorder(t)
				rec.transcript = c.write(first+"/sess-c.jsonl", tc.lines...)
				rec.call("sess-o", "p1", "toolu_c", T, T.Add(time.Second), tc.outcome)
				return rec.st
			})
			if j := s.SilentFailureTurns; j.CoveredTranscripts != 1 || j.NotCoveredTranscripts != 0 {
				t.Fatalf("premise: line %+v; want the one transcript covered", j)
			}
			if o := row(s, "sess-o"); o == nil || o.Coverage != CoverageRecorded {
				t.Errorf("rows = %+v; want sess-o recorded", s.PerSession)
			}
			txt, _ := render(t, s)
			if !strings.Contains(txt, "sess-o <$0.01 (main <$0.01, subagents none)\n") {
				t.Errorf("the text does not show sess-o as recorded:\n%s", txt)
			}
		})
	}
}

// TestJoin_AnUndatedUsageLineDoesNotReDateItsFile: a file is dated by its
// first dated line, and an undated usage line after it changes nothing.
// sess-a's file starts ten minutes before sess-b's, so sess-a owns the
// response both hold, though an undated usage line comes between its first
// dated line and the response. Were the per-line flag to re-open dating, the
// response's own timestamp would date sess-a's file after sess-b's, and the
// response would move to sess-b's row.
func TestJoin_AnUndatedUsageLineDoesNotReDateItsFile(t *testing.T) {
	T := now.Add(-2 * time.Hour)
	x := resp{id: "X1", model: "claude-opus-5-5", at: T, in: 1000, stop: "end_turn"}
	s := orders(t, func(c *config, first, second string) *store.Store {
		xa, xb := x, x
		xa.session, xb.session = "sess-a", "sess-b"
		c.write(first+"/sess-a.jsonl", userLine("sess-a", "p0", T.Add(-10*time.Minute), false),
			resp{id: "U1", model: "claude-opus-5-5", session: "sess-a", in: 5, stop: "end_turn", noTimestamp: true}.line("text"),
			xa.line("text"))
		c.write(second+"/sess-b.jsonl", userLine("sess-b", "p1", T.Add(-5*time.Minute), false), xb.line("text"))
		return newRecorder(t).st
	})
	a, b := row(s, "sess-a"), row(s, "sess-b")
	if s.Read.UndatedResponses != 1 || a == nil || b == nil || a.Main != (Cost{Nano: 1000 * opusIn, Priced: 1}) || b.Main != (Cost{}) {
		t.Errorf("undated %d, rows %+v; want the undated line counted and the response in sess-a's row alone", s.Read.UndatedResponses, s.PerSession)
	}
}

// coverage writes one coverage record as a hook does, at a chosen time:
// verified, or unverified for reason.
func (r *recorder) coverage(session, phase string, at time.Time, reason string) {
	r.t.Helper()
	c := store.Coverage{Type: store.TypeCoverage, SchemaVersion: store.SchemaVersion, RecordedAtMS: at.UnixMilli(),
		SessionID: session, Phase: phase, State: store.StateVerified, HookEntry: store.EntryPresentSettings, Probe: store.ProbeFresh}
	if reason != "" {
		c.State, c.Reason = store.StateUnverified, &reason
	}
	if err := r.st.AppendCoverage(c); err != nil {
		r.t.Fatal(err)
	}
}

// TestJoin_AWatchedSessionWithNoCallIsRecorded: a session rashomon watched
// from its start to its end, in which no tool was called, leaves a start and
// an end record and nothing else, so no record names its transcript.
// Measured on Claude Code 2.1.280: `claude -p "Reply with exactly: ok"`, a
// resume of it, and an interactive session answered without a tool each read
// "not recorded by rashomon", and the silent-failure line "unknown". Such a
// session had no call to miss: its transcript is covered, its row recorded,
// and the line a checked none. A session no hook saw, answering inside the
// same stretch of time, is not covered by it.
func TestJoin_AWatchedSessionWithNoCallIsRecorded(t *testing.T) {
	t0 := now.Add(-2 * time.Hour)
	s := orders(t, func(c *config, first, second string) *store.Store {
		rec := newRecorder(t)
		rec.coverage("sess-q", store.PhaseStart, t0, "")
		c.write(first+"/sess-q.jsonl", userLine("sess-q", "q1", t0.Add(-44*time.Millisecond), false),
			resp{id: "Q1", model: "claude-opus-5-5", session: "sess-q", at: t0.Add(2 * time.Second), in: 200000, stop: "end_turn"}.line("text"))
		rec.coverage("sess-q", store.PhaseEnd, t0.Add(3*time.Second), "")
		c.write(second+"/sess-u.jsonl", userLine("sess-u", "u1", t0.Add(time.Second), false),
			resp{id: "U1", model: "claude-opus-5-5", session: "sess-u", at: t0.Add(2 * time.Second), in: 100000, stop: "end_turn"}.line("text"))
		return rec.st
	})
	j := s.SilentFailureTurns
	if j.CoveredTranscripts != 1 || j.NotCoveredTranscripts != 1 || strings.Join(j.NotCoveredSessions, ",") != "sess-u" {
		t.Errorf("covered %d, not covered %d, not-covered sessions %v; want 1, 1 and sess-u", j.CoveredTranscripts, j.NotCoveredTranscripts, j.NotCoveredSessions)
	}
	if q, u := row(s, "sess-q"), row(s, "sess-u"); q == nil || u == nil || q.Coverage != CoverageRecorded || u.Coverage != CoverageNotRecorded {
		t.Errorf("rows = %+v; want sess-q recorded and sess-u not", s.PerSession)
	}
	txt, js := render(t, s)
	for _, want := range []string{
		"sess-q $0.80 (main $0.80, subagents none)\n",
		"sess-u $0.40 (main $0.40, subagents none), not recorded by rashomon\n",
		"never mentioned: none found (no recorded turn with a failed call ended in a summary that left it out; a failed Read, Glob, Grep or NotebookRead alone is not counted)\n",
		"(from rashomon's record; 1 of 2 transcripts was recorded, so this covers only those; $0.40 in the other 1 is not covered)\n",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("text lacks %q:\n%s", want, txt)
		}
	}
	if !strings.Contains(js, `"turns":0,"unjudged_turns":0,"undeclared_failed_calls":0,"cost":{"usd":0,`) {
		t.Errorf("the covered session did not marshal as a checked zero:\n%s", js)
	}
}

// TestJoin_AWatchedSessionIsRecordedOnlyWhereItWasWatched: what a run with
// no call covers, guard by guard. Its transcript is covered only when the run
// holds no trace of a call and nothing unverified; no response the scan read
// in the transcript -- before the window and undated included -- made a
// call, ended without a stop_reason, or was a subagent's (a subagent is
// started by a call); and every response in the window started inside one
// stretch from a start record to the end record after it. A user line
// stamped before the start record covers or uncovers nothing: measured,
// Claude Code stamps its first attachment lines up to 44 ms before
// SessionStart fires, and only a response's start is dated after it.
func TestJoin_AWatchedSessionIsRecordedOnlyWhereItWasWatched(t *testing.T) {
	t0 := now.Add(-2 * time.Hour)
	at := func(sec float64) time.Time { return t0.Add(time.Duration(sec * float64(time.Second))) }
	// edge is the moment the window opens, in seconds after t0.
	edge := WindowStart(now, 30).Sub(t0).Seconds()
	type cv struct {
		phase  string
		sec    float64
		reason string
	}
	start, end := cv{store.PhaseStart, 0, ""}, cv{store.PhaseEnd, 10, ""}
	resumed := []cv{start, end, {store.PhaseStart, 20, ""}, {store.PhaseEnd, 30, ""}}
	straddle := []cv{{store.PhaseStart, edge - 10, ""}, {store.PhaseEnd, edge + 10, ""}}
	q := func(id string, sec float64, stop string) resp {
		return resp{id: id, model: "claude-opus-5-5", session: "sess-q", at: at(sec), in: 1000, stop: stop}
	}
	sub := q("S1", 6, "end_turn")
	sub.sidechain = true
	undated := q("Q2", 6, "tool_use")
	undated.noTimestamp = true
	ms := func(sec float64) int64 { return at(sec).UnixMilli() }
	for _, tc := range []struct {
		name     string
		coverage []cv
		resps    []resp
		store    func(rec *recorder)
		covered  bool
	}{
		{"a response inside", []cv{start, end}, []resp{q("Q1", 5, "end_turn")}, nil, true},
		{"start only: the session has not ended", []cv{start}, []resp{q("Q1", 5, "end_turn")}, nil, false},
		{"end only", []cv{end}, []resp{q("Q1", 5, "end_turn")}, nil, false},
		{"a start that is not verified", []cv{{store.PhaseStart, 0, store.ReasonProbeUnresolved}, end}, []resp{q("Q1", 5, "end_turn")}, nil, false},
		{"an end that is not verified", []cv{start, {store.PhaseEnd, 10, store.ReasonHookEntryAbsent}}, []resp{q("Q1", 5, "end_turn")}, nil, false},
		{"a paused call", []cv{start, {store.PhaseCall, 4, store.ReasonRecordingPaused}, end}, []resp{q("Q1", 5, "end_turn")}, nil, false},
		// forget removes a call's declaration, execution and terminal and
		// keeps its coverage records.
		{"a forgotten call", []cv{start, {store.PhaseCall, 4, ""}, {store.PhasePost, 4.5, ""}, end}, []resp{q("Q1", 5, "end_turn")}, nil, false},
		{"a call whose record names no discovered transcript", []cv{start, end}, []resp{q("Q1", 5, "end_turn")}, func(rec *recorder) {
			rec.transcript = "/elsewhere/sess-q.jsonl"
			rec.call("sess-q", "q1", "toolu_q", at(4), at(4.5), store.ExecOK)
		}, false},
		{"a record that did not parse", []cv{start, end}, []resp{q("Q1", 5, "end_turn")}, func(rec *recorder) {
			p := filepath.Join(rec.st.RunDir("sess-q"), store.FileRecords)
			if err := os.WriteFile(p, []byte("{\"type\":\"declaration\",\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, false},
		// A headless run reusing an interactive session's id (Join): the
		// interactive conversation answered before the run was watched.
		{"a response before the first start", []cv{start, end}, []resp{q("Q0", -5, "end_turn"), q("Q1", 5, "end_turn")}, nil, false},
		{"a response after the last end", []cv{start, end}, []resp{q("Q1", 5, "end_turn"), q("Q2", 15, "end_turn")}, nil, false},
		// Measured, no response started within a millisecond of either
		// record; the stretch includes both, as a span of recorded time
		// does.
		{"a response at the start record's millisecond", []cv{start, end}, []resp{q("Q1", 0, "end_turn")}, nil, true},
		{"a response at the end record's millisecond", []cv{start, end}, []resp{q("Q1", 10, "end_turn")}, nil, true},
		{"a response that made a call", []cv{start, end}, []resp{q("Q1", 5, "tool_use"), q("Q2", 6, "end_turn")}, nil, false},
		{"a response with no stop_reason", []cv{start, end}, []resp{q("Q1", 5, "")}, nil, false},
		{"a subagent's response", []cv{start, end}, []resp{q("Q1", 5, "end_turn"), sub}, nil, false},
		// --resume keeps the session id and appends to the same transcript,
		// and each process fires its own start and end.
		{"a resumed session, every response watched", resumed, []resp{q("Q1", 5, "end_turn"), q("Q2", 25, "end_turn")}, nil, true},
		{"a response between two watched stretches", resumed, []resp{q("Q1", 5, "end_turn"), q("Q2", 15, "end_turn"), q("Q3", 25, "end_turn")}, nil, false},
		// Measured: a resume run without the hooks made a Bash call between
		// two watched runs, and the envelope from the first start to the
		// last end read the transcript recorded.
		{"an unwatched call between two watched stretches", resumed,
			[]resp{q("Q1", 5, "end_turn"), q("Q2", 14, "tool_use"), q("Q3", 15, "end_turn"), q("Q4", 25, "end_turn")}, nil, false},
		// A process that ended with no end record watched nothing after its
		// start that a later start does not begin again.
		{"a start with no end, then a watched stretch", []cv{start, {store.PhaseStart, 20, ""}, {store.PhaseEnd, 30, ""}},
			[]resp{q("Q1", 5, "end_turn"), q("Q2", 25, "end_turn")}, nil, false},
		// SessionStart fires on a compaction too, on the same id (Claude
		// Code's hooks documentation), and the probe does not read its
		// source: the second start begins the stretch again, and what came
		// before it reads unwatched. Not covered, the fail-closed side.
		{"a compaction's start in mid-session", []cv{start, {store.PhaseStart, 5, ""}, end},
			[]resp{q("Q1", 2, "end_turn"), q("Q2", 7, "end_turn")}, nil, false},
		// Each trace of a call alone keeps the run from vouching. A spilled
		// terminal is the only trace of a call whose declaration lost the
		// lock (store.Terminal); an execution alone is a PostToolUse whose
		// PreToolUse left nothing.
		{"a terminal alone", []cv{start, end}, []resp{q("Q1", 5, "end_turn")}, func(rec *recorder) {
			if err := rec.st.SpillTerminal(store.Terminal{Type: store.TypeTerminal, SchemaVersion: store.SchemaVersion,
				RecordedAtMS: ms(4), ToolUseID: "toolu_q", SessionID: "sess-q", Outcome: store.OutcomeError}); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"an execution alone", []cv{start, end}, []resp{q("Q1", 5, "end_turn")}, func(rec *recorder) {
			if err := rec.st.AppendExecution(store.Execution{Type: store.TypeExecution, SchemaVersion: store.SchemaVersion,
				RecordedAtMS: ms(4), ToolUseID: "toolu_q", SessionID: "sess-q", ToolName: "Bash", Outcome: store.ExecOK}); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"a declaration alone, naming no discovered transcript", []cv{start, end}, []resp{q("Q1", 5, "end_turn")}, func(rec *recorder) {
			p := "q1"
			if err := rec.st.AppendDeclaration(store.Declaration{Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
				RecordedAtMS: ms(4), ToolUseID: "toolu_q", SessionID: "sess-q", PromptID: &p, ToolName: "Bash",
				TranscriptPath: "/elsewhere/sess-q.jsonl"}); err != nil {
				t.Fatal(err)
			}
		}, false},
		// A turn can start before the window and end inside it. Here the
		// call was made by a response dated before the window opened, and
		// the run holds no record of it -- as a review measured with the
		// real hooks: watch, start, detach, a call, watch, end leaves a
		// verified start and end and nothing else. The window's responses
		// alone would read clean.
		{"a call made just before the window", straddle, []resp{q("W1", edge-5, "tool_use"), q("W2", edge+3, "end_turn")}, nil, false},
		{"an undated response that made a call", []cv{start, end}, []resp{q("Q1", 5, "end_turn"), undated}, nil, false},
		// Only the window's responses need to have started while the run
		// watched: one before the window that made no call has nothing in
		// the window to miss.
		{"a response before the window, unwatched, that made no call", straddle,
			[]resp{q("W0", edge-60, "end_turn"), q("W1", edge+3, "end_turn")}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfig(t)
			rec := newRecorder(t)
			for _, v := range tc.coverage {
				rec.coverage("sess-q", v.phase, at(v.sec), v.reason)
			}
			if tc.store != nil {
				tc.store(rec)
			}
			lines := []string{userLine("sess-q", "q1", at(-0.044), false)}
			for _, r := range tc.resps {
				lines = append(lines, r.line("text"))
			}
			c.write("proj/sess-q.jsonl", lines...)
			s := c.summary(30)
			if err := s.Join(rec.st); err != nil {
				t.Fatal(err)
			}
			j := s.SilentFailureTurns
			txt, _ := render(t, s)
			want, label := 0, CoverageNotRecorded
			if tc.covered {
				want, label = 1, CoverageRecorded
			}
			if j.Transcripts != 1 || j.CoveredTranscripts != want || len(s.PerSession) != 1 || s.PerSession[0].Coverage != label {
				t.Errorf("covered %d of %d, rows %+v; want %d covered and the row %s\n%s", j.CoveredTranscripts, j.Transcripts, s.PerSession, want, label, txt)
			}
		})
	}
}

// TestJoin_AWatchedSessionWithACallKeepsTheRecordsRule: a run that holds a
// call is covered by its records' transcript_path, as before, even when its
// start and end records leave every response outside a stretch: the records
// rule does not ask when the run watched.
func TestJoin_AWatchedSessionWithACallKeepsTheRecordsRule(t *testing.T) {
	t0 := now.Add(-2 * time.Hour)
	c := newConfig(t)
	rec := newRecorder(t)
	rec.transcript = c.write("proj/sess-r.jsonl", userLine("sess-r", "r1", t0, false),
		resp{id: "R1", model: "claude-opus-5-5", session: "sess-r", at: t0.Add(time.Second), in: 1000, stop: "tool_use"}.line("tool_use"),
		userLine("sess-r", "r1", t0.Add(2*time.Second), true),
		resp{id: "R2", model: "claude-opus-5-5", session: "sess-r", at: t0.Add(3 * time.Second), in: 1000, stop: "end_turn"}.line("text"))
	rec.call("sess-r", "r1", "toolu_r", t0.Add(1500*time.Millisecond), t0.Add(1800*time.Millisecond), store.ExecOK)
	rec.coverage("sess-r", store.PhaseStart, t0.Add(10*time.Second), "")
	rec.coverage("sess-r", store.PhaseEnd, t0.Add(20*time.Second), "")
	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	if j := s.SilentFailureTurns; j.CoveredTranscripts != 1 || s.PerSession[0].Coverage != CoverageRecorded {
		t.Errorf("covered %d, rows %+v; want the transcript the record names covered, its responses outside the run's one stretch", j.CoveredTranscripts, s.PerSession)
	}
}

// TestJoin_AWatchedSessionNotReadWholeIsNotCovered: a run with no call
// vouches for a transcript only when the scan read every response in it. A
// usage line that could not be counted, or the rest of a file past a line too
// long to read, may be the response that made the call, so the transcript is
// not covered. Read whole, the same fixture is.
func TestJoin_AWatchedSessionNotReadWholeIsNotCovered(t *testing.T) {
	t0 := now.Add(-2 * time.Hour)
	q1 := resp{id: "Q1", model: "claude-opus-5-5", session: "sess-q", at: t0.Add(2 * time.Second), in: 1000, stop: "end_turn"}.line("text")
	q2 := resp{id: "Q2", model: "claude-opus-5-5", session: "sess-q", at: t0.Add(4 * time.Second), in: 1000, stop: "tool_use"}.line("tool_use")
	unparsed := strings.Replace(q2, `"input_tokens":1000`, `"input_tokens":"1000"`, 1)
	if unparsed == q2 {
		t.Fatalf("the fixture did not write input_tokens as expected: %s", q2)
	}
	for _, tc := range []struct {
		name                 string
		tail                 []string
		covered              bool
		unparsed, unreadable int
	}{
		{"read whole", nil, true, 0, 0},
		{"a usage line that did not parse", []string{unparsed}, false, 1, 0},
		{"a line too long to read, and a call after it", []string{`{"type":"assistant","x":"` + strings.Repeat("x", maxLine) + `"}`, q2}, false, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfig(t)
			rec := newRecorder(t)
			rec.coverage("sess-q", store.PhaseStart, t0, "")
			rec.coverage("sess-q", store.PhaseEnd, t0.Add(10*time.Second), "")
			c.write("proj/sess-q.jsonl", append([]string{userLine("sess-q", "q1", t0.Add(time.Second), false), q1}, tc.tail...)...)
			s := c.summary(30)
			if err := s.Join(rec.st); err != nil {
				t.Fatal(err)
			}
			if s.Read.UnparsedUsageLines != tc.unparsed || s.Read.UnreadableFiles != tc.unreadable {
				t.Fatalf("unparsed usage lines %d, unreadable files %d; want %d and %d: the fixture is not the case it names",
					s.Read.UnparsedUsageLines, s.Read.UnreadableFiles, tc.unparsed, tc.unreadable)
			}
			want, label := 0, CoverageNotRecorded
			if tc.covered {
				want, label = 1, CoverageRecorded
			}
			if j := s.SilentFailureTurns; j.Transcripts != 1 || j.CoveredTranscripts != want || len(s.PerSession) != 1 || s.PerSession[0].Coverage != label {
				t.Errorf("covered %d of %d, rows %+v; want %d covered and the row %s", j.CoveredTranscripts, j.Transcripts, s.PerSession, want, label)
			}
		})
	}
}

// TestJoin_AWatchedSessionIDCoversOnlyTheConversationItWatched: the
// --session-id reuse Join describes, with no call made. A headless run
// reusing an interactive session's id writes a second main transcript under
// another project folder, and its hooks write start and end records under
// that id. Every response of the interactive conversation started before the
// headless run did: covering every transcript named for the id, as a first,
// unguarded version of this rule did, read it recorded. Only the headless one
// is, and the row is partly recorded, in either path order.
func TestJoin_AWatchedSessionIDCoversOnlyTheConversationItWatched(t *testing.T) {
	t0 := now.Add(-2 * time.Hour)
	s := orders(t, func(c *config, first, second string) *store.Store {
		c.write(first+"/sess-j.jsonl", userLine("sess-j", "i1", t0.Add(-time.Hour), false),
			resp{id: "I1", model: "claude-opus-5-5", session: "sess-j", at: t0.Add(-time.Hour), in: 3000, stop: "end_turn"}.line("text"))
		rec := newRecorder(t)
		rec.coverage("sess-j", store.PhaseStart, t0, "")
		c.write(second+"/sess-j.jsonl", userLine("sess-j", "h1", t0.Add(time.Second), false),
			resp{id: "H1", model: "claude-opus-5-5", session: "sess-j", at: t0.Add(2 * time.Second), in: 1000, stop: "end_turn"}.line("text"))
		rec.coverage("sess-j", store.PhaseEnd, t0.Add(3*time.Second), "")
		return rec.st
	})
	j := s.SilentFailureTurns
	if j.Transcripts != 2 || j.CoveredTranscripts != 1 || j.NotCoveredCost.Nano != 3000*opusIn ||
		strings.Join(j.NotCoveredSessions, ",") != "sess-j" {
		t.Errorf("line %+v; want 1 of 2 covered, the interactive transcript's spend not covered", j)
	}
	if r := row(s, "sess-j"); r == nil || r.Coverage != CoveragePartly {
		t.Errorf("rows = %+v; want sess-j partly recorded", s.PerSession)
	}
}

// TestJoin_AFailedLookupIsNoTurnToJudge: a turn whose only failure is a lookup
// (a Read of a directory) cannot fire the silent-failure line
// (report.IsLookup), so it is not a turn to judge: a missing transcript leaves
// it unjudged no more than a clean turn, and a lost one is no failed call that
// could not be checked. The Bash case is the control. Break: filter turns on
// Failed rather than Counted, or count every lost failed call, and the Read
// case reads 1.
func TestJoin_AFailedLookupIsNoTurnToJudge(t *testing.T) {
	for _, tc := range []struct {
		tool string
		want int
	}{{"Read", 0}, {"Bash", 1}} {
		t.Run(tc.tool, func(t *testing.T) {
			c := newConfig(t)
			rec := newRecorder(t)
			T := now.Add(-2 * time.Hour)
			p := "k1"
			for _, x := range []struct {
				id     string
				prompt *string
			}{{"toolu_k", &p}, {"toolu_lost", nil}} {
				if x.prompt != nil {
					if err := rec.st.AppendDeclaration(store.Declaration{
						Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
						RecordedAtMS: T.UnixMilli(), ToolUseID: x.id, SessionID: "sess-k",
						PromptID: x.prompt, ToolName: tc.tool, TranscriptPath: "/elsewhere/sess-k.jsonl",
					}); err != nil {
						t.Fatal(err)
					}
				}
				if err := rec.st.AppendExecution(store.Execution{
					Type: store.TypeExecution, SchemaVersion: store.SchemaVersion,
					RecordedAtMS: T.Add(time.Second).UnixMilli(), ToolUseID: x.id, SessionID: "sess-k",
					ToolName: tc.tool, Outcome: store.ExecFailed,
				}); err != nil {
					t.Fatal(err)
				}
			}
			c.write("proj/sess-k.jsonl", resp{id: "K1", model: "claude-opus-5-5", session: "sess-k", at: T, in: 500, stop: "end_turn"}.line("text"))

			s := c.summary(30)
			if err := s.Join(rec.st); err != nil {
				t.Fatal(err)
			}
			j := s.SilentFailureTurns
			if j.Turns != 0 || j.Unjudged != tc.want || j.UndeclaredFailedCalls != tc.want {
				t.Errorf("turns %d, unjudged %d, undeclared %d; want 0, %d, %d", j.Turns, j.Unjudged, j.UndeclaredFailedCalls, tc.want, tc.want)
			}
		})
	}
}

// TestJoin_ANoneSaysAFailedLookupAloneIsNotCounted: a turn whose only failed
// calls are lookups is not judged (report.IsLookup), and a lost failed lookup
// is not counted as unchecked, so a "none found" printed over one would claim
// a check that never ran while `rashomon report` says "failed calls: 1".
// Each "none" says what it leaves out. A failed Read alone reads the first
// none; a failed Grep beside a failed Bash call whose declaration was lost
// reads the second; a failed Bash alone fires, as the control. Break: put
// back either old sentence without the exception.
func TestJoin_ANoneSaysAFailedLookupAloneIsNotCounted(t *testing.T) {
	const aside = "a failed Read, Glob, Grep or NotebookRead alone is not counted"
	for _, tc := range []struct {
		name  string
		calls []struct{ tool, prompt string }
		turns int
		want  string
	}{
		{"read alone", []struct{ tool, prompt string }{{"Read", "n1"}}, 0,
			"never mentioned: none found (no recorded turn with a failed call ended in a summary that left it out; " + aside + ")\n"},
		{"grep beside a lost bash", []struct{ tool, prompt string }{{"Grep", "n1"}, {"Bash", ""}}, 0,
			"never mentioned: none found in the turns that could be checked (" + aside + ")\n"},
		{"bash alone", []struct{ tool, prompt string }{{"Bash", "n1"}}, 1, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newConfig(t)
			rec := newRecorder(t)
			T := now.Add(-2 * time.Hour)
			tr := filepath.Join(c.dir, "projects", "proj", "sess-n.jsonl")
			for i, x := range tc.calls {
				id := fmt.Sprintf("toolu_n%d", i)
				if x.prompt != "" {
					p := x.prompt
					if err := rec.st.AppendDeclaration(store.Declaration{
						Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
						RecordedAtMS: T.UnixMilli(), ToolUseID: id, SessionID: "sess-n",
						PromptID: &p, ToolName: x.tool, TranscriptPath: tr,
					}); err != nil {
						t.Fatal(err)
					}
				}
				if err := rec.st.AppendExecution(store.Execution{
					Type: store.TypeExecution, SchemaVersion: store.SchemaVersion,
					RecordedAtMS: T.Add(time.Second).UnixMilli(), ToolUseID: id, SessionID: "sess-n",
					ToolName: x.tool, Outcome: store.ExecFailed,
				}); err != nil {
					t.Fatal(err)
				}
			}
			c.write("proj/sess-n.jsonl",
				userLine("sess-n", "n1", T.Add(-time.Second), false),
				resp{id: "N0", model: "claude-opus-5-5", session: "sess-n", at: T.Add(-500 * time.Millisecond), in: 5, stop: "tool_use"}.line("tool_use"),
				userLine("sess-n", "n1", T.Add(time.Second), true),
				resp{id: "N1", model: "claude-opus-5-5", session: "sess-n", at: T.Add(2 * time.Second), in: 5, stop: "end_turn", text: "All done."}.line("text"))

			s := c.summary(30)
			if err := s.Join(rec.st); err != nil {
				t.Fatal(err)
			}
			if j := s.SilentFailureTurns; j.Turns != tc.turns || j.Unjudged != 0 {
				t.Errorf("turns %d, unjudged %d; want %d and 0", j.Turns, j.Unjudged, tc.turns)
			}
			txt, _ := render(t, s)
			if tc.want == "" {
				if strings.Contains(txt, "none found") {
					t.Errorf("a failed Bash call under \"All done.\" reads as none:\n%s", txt)
				}
				return
			}
			if !strings.Contains(txt, tc.want) {
				t.Errorf("want %q:\n%s", tc.want, txt)
			}
		})
	}
}
