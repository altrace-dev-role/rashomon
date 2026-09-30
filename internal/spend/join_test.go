package spend

import (
	"encoding/json"
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

// silentSession is the join's shape: a turn with a recorded failure and a
// final message that mentions none, then a clean turn whose final message
// DOES carry a failure word -- which must not be read as the first turn's.
//
// Responses, against the first turn's recorded span [T, T+3s]:
//
//	R0  T-0.5s  the response that made the first call: before the span
//	R1  T+1.5s  main, inside
//	S1  T+2.5s  subagent, inside
//	R3  T+4s    the final reply: after the span
//
// Every user line carries its turn's promptId, as a real transcript's do.
// between is written after R3 and before p2's prompt (at T+9.5s): a test's
// own lines for what happens between two recorded turns.
func silentSession(t *testing.T, c *config, rec *recorder, T time.Time, final1 string, between ...string) (inside int64) {
	t.Helper()
	sec := func(f float64) time.Time { return T.Add(time.Duration(f * float64(time.Second))) }
	rec.transcript = filepath.Join(c.dir, "projects", "proj", "sess-j.jsonl")
	rec.call("sess-j", "p1", "toolu_1", sec(0), sec(1), store.ExecFailed)
	rec.call("sess-j", "p1", "toolu_2", sec(2), sec(3), store.ExecOK)
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
	c.write("proj/sess-j/subagents/agent-x.jsonl", sub.line("tool_use"))
	return (100 + 20) * opusIn
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
		t.Errorf("cost = %d, want %d: R1 and the subagent's S1, which started inside the turn's recorded span -- "+
			"not R0 before it or the final reply after it", j.Cost.Nano, inside)
	}
	if j.CoveredSessions != 1 || j.NotCoveredSessions != 1 || j.Sessions != 2 {
		t.Errorf("covered %d, not covered %d of %d; want 1, 1 of 2", j.CoveredSessions, j.NotCoveredSessions, j.Sessions)
	}
	if j.NotCoveredCost.Nano != 7*opusIn {
		t.Errorf("not-covered cost = %d, want %d: an unrecorded session's spend is shown as not covered, never as zero",
			j.NotCoveredCost.Nano, 7*opusIn)
	}
	txt, _ := render(t, s)
	for _, want := range []string{
		"in turns that ended with a failure the summary never mentioned: at least <$0.01 across 1 turn",
		"1 of 2 sessions was recorded",
		"in the other 1 is not covered",
		"so this is a floor",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("text lacks %q:\n%s", want, txt)
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
	if j.Store != StoreNone || j.NotCoveredSessions != 1 || j.NotCoveredCost.Nano != 1e6*opusIn {
		t.Errorf("join = %+v, want store none and the one session not covered with its spend", j)
	}
	txt, _ := render(t, s)
	if !strings.Contains(txt, "never mentioned: unknown") ||
		!strings.Contains(txt, "rashomon has recorded nothing on this machine, so none of the 1 session is covered") {
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
	for _, k := range []string{"turns", "cost"} {
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
	_, js := render(t, s)
	if !strings.Contains(js, `"turns":0,"cost":{"usd":0,`) {
		t.Errorf("a covered, clean record did not marshal as a checked zero:\n%s", js)
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

	s := c.summary(30)
	if err := s.Join(rec.st); err != nil {
		t.Fatal(err)
	}
	j := s.SilentFailureTurns
	if j.Turns != 1 {
		t.Fatalf("turns = %d, want 1: the verdict was taken on the other conversation's reply", j.Turns)
	}
	if j.Cost.Nano != inside {
		t.Errorf("cost = %d, want %d: the other conversation's response inside the span was priced into this turn",
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
