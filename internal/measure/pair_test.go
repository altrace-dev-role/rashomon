package measure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

const (
	sOff   = "sess-off"
	sSteer = "sess-steer"
	sOther = "sess-other"
)

// fix is one store and one Claude Code configuration directory, written on a
// clock an hour back that moves a minute per record, so every transcript's
// mtime is after the sessions it holds.
type fix struct {
	t   *testing.T
	st  *store.Store
	cfg string
	now time.Time
	n   int
}

func newFix(t *testing.T) *fix {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteLab(st.Root()); err != nil {
		t.Fatal(err)
	}
	return &fix{t: t, st: st, cfg: t.TempDir(), now: time.Now().Add(-time.Hour).Truncate(time.Second)}
}

func (f *fix) tick() int64 {
	f.now = f.now.Add(time.Minute)
	return f.now.UnixMilli()
}

// call records one declared call of tool with shape sh and, when outcome is
// not "", how it ended.
func (f *fix) call(session, tool string, sh shape.Shape, outcome string) {
	f.t.Helper()
	f.n++
	id := fmt.Sprintf("toolu_%d", f.n)
	if err := f.st.AppendDeclaration(store.Declaration{Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
		RecordedAtMS: f.tick(), ToolUseID: id, SessionID: session, ToolName: tool, Shape: sh}); err != nil {
		f.t.Fatal(err)
	}
	if outcome == "" {
		return
	}
	if err := f.st.AppendExecution(store.Execution{Type: store.TypeExecution, SchemaVersion: store.SchemaVersion,
		RecordedAtMS: f.tick(), ToolUseID: id, SessionID: session, ToolName: tool, Outcome: outcome}); err != nil {
		f.t.Fatal(err)
	}
}

var (
	edit    = shape.Shape{VerbClass: shape.VerbWrite}
	read    = shape.Shape{VerbClass: shape.VerbRead}
	goTest  = shape.Shape{VerbClass: shape.VerbTest}
	sedI    = shape.Shape{VerbClass: shape.VerbRead, MayWrite: true}
	gitPush = shape.Shape{VerbClass: shape.VerbVCS}
)

func (f *fix) lines(session string, ls ...*steer.Line) {
	f.t.Helper()
	batch := make([]store.SteerLine, 0, len(ls))
	for _, l := range ls {
		l.Type, l.V, l.SessionID, l.AtUnixMS = steer.LineType, steer.LineVersion, session, f.tick()
		batch = append(batch, l)
	}
	if err := f.st.AppendSteer(session, func(io.ReaderAt) ([]store.SteerLine, error) { return batch, nil }); err != nil {
		f.t.Fatal(err)
	}
}

func ptr[T any](v T) *T { return &v }

// shadow is an arm-off note on prompt p1 that the steer arm would have
// given: suppressed by arm_off, delivery shadow.
func shadow(id string, p pattern.ID, key string, after int64) *steer.Line {
	return &steer.Line{ID: id, PromptID: ptr("p1"), Pattern: string(p), Key: key, AfterSeq: after, Arm: steer.ArmOff,
		Level: "nudge", State: steer.LineStateSuppressed, SuppressedBy: ptr(steer.SuppressedByArmOff), Delivery: ptr(steer.DeliveryShadow)}
}

// delivered is an arm-steer note on prompt p1 the agent was given.
func delivered(id string, p pattern.ID, key string, after int64) *steer.Line {
	return &steer.Line{ID: id, PromptID: ptr("p1"), Pattern: string(p), Key: key, AfterSeq: after, Arm: steer.ArmSteer,
		Level: "nudge", State: steer.LineStateDelivered, Delivery: ptr(steer.DeliveryDelivered), ModelText: "rashomon record: gate 'go' went red"}
}

// budgeted is l refused by the per-key budget, as both arms refuse it.
func budgeted(l *steer.Line) *steer.Line {
	l.State, l.SuppressedBy, l.Delivery, l.ModelText =
		steer.LineStateSuppressed, ptr(steer.SuppressedByBudgetPattern), ptr(steer.DeliverySuppressedBudget), ""
	return l
}

// onPrompt moves l to prompt.
func onPrompt(l *steer.Line, prompt string) *steer.Line {
	l.PromptID = &prompt
	return l
}

// stop appends the session's Stop checkpoint with gates.
func (f *fix) stop(session string, gates ...verify.CheckpointGate) {
	f.t.Helper()
	cp := &verify.Checkpoint{Type: store.TypeVerify, SchemaVersion: store.EventSchemaVersion, RecordedAtUnixMS: f.tick(),
		SessionID: session, Point: verify.PointStop, PointSeq: 1, ProjectRoot: "/repo", Gates: gates}
	if err := f.st.AppendRecord(session, cp); err != nil {
		f.t.Fatal(err)
	}
}

func green(gate string, lastGreen int64) verify.CheckpointGate {
	return verify.CheckpointGate{Gate: gate, Kind: "test", Source: "observed", State: verify.Green, LastGreenSeq: &lastGreen, LastRunSeq: &lastGreen}
}

// transcript writes session's main transcript: n assistant responses of
// model, each in and out tokens.
func (f *fix) transcript(session, model string, n int, in, out int64) {
	f.t.Helper()
	dir := filepath.Join(f.cfg, "projects", "-repo")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		f.t.Fatal(err)
	}
	var b strings.Builder
	for i := range n {
		at := f.now.Add(time.Duration(i) * time.Second).UTC().Format("2006-01-02T15:04:05.000Z")
		fmt.Fprintf(&b, `{"type":"assistant","timestamp":%q,"sessionId":%q,"message":{"id":"msg_%s_%d","model":%q,`+
			`"stop_reason":"end_turn","usage":{"input_tokens":%d,"output_tokens":%d,"cache_read_input_tokens":7,"cache_creation_input_tokens":0}}}`+"\n",
			at, session, session, i, model, in, out)
	}
	if err := os.WriteFile(filepath.Join(dir, session+".jsonl"), []byte(b.String()), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// twoArms is TestMS1e's store: an off session of six calls whose lines are
// three withheld notes and one refused by the per-key budget (three lines of
// one regression over two prompts, one identical-retry), a steer session of
// four calls with three delivered notes and one budget-refused line, the off
// session stopped with its gate green after the regression and the steer
// session with its last green before it, and a third session's transcript
// that belongs to neither.
func twoArms(t *testing.T) *fix {
	t.Helper()
	f := newFix(t)
	f.call(sOff, "Edit", edit, store.ExecOK)
	f.call(sOff, "Bash", goTest, store.ExecFailed)
	f.call(sOff, "Edit", edit, store.ExecOK)
	f.call(sOff, "Bash", goTest, store.ExecOK)
	f.call(sOff, "Read", read, store.ExecOK)
	f.call(sOff, "Bash", sedI, store.ExecOK)
	f.lines(sOff,
		shadow("o1", pattern.RegressionLocation, "go", 4),
		budgeted(shadow("o2", pattern.RegressionLocation, "go", 5)),
		shadow("o3", pattern.IdenticalRetry, "retry-1", 6),
		onPrompt(shadow("o4", pattern.RegressionLocation, "go", 6), "p2"))
	f.stop(sOff, green("go", 8))

	f.call(sSteer, "Edit", edit, store.ExecOK)
	f.call(sSteer, "Bash", goTest, store.ExecFailed)
	f.call(sSteer, "Bash", goTest, store.ExecOK)
	f.call(sSteer, "Bash", gitPush, "")
	f.lines(sSteer,
		delivered("t1", pattern.RegressionLocation, "go", 4),
		delivered("t2", pattern.SubtaskGate, "todo", 4),
		budgeted(delivered("t4", pattern.RegressionLocation, "go", 4)),
		onPrompt(delivered("t3", pattern.LearnedFact, "lesson-1", 5), "p2"))
	f.stop(sSteer, green("go", 3))

	f.transcript(sOff, "claude-haiku-4-5", 3, 100, 10)
	f.transcript(sSteer, "claude-haiku-4-5", 2, 200, 20)
	f.transcript(sOther, "claude-haiku-4-5", 5, 1000, 1000)
	return f
}

// ask is session asked as arm, in f's store and configuration directory.
func (f *fix) ask(session string, arm steer.Arm) Ask {
	return Ask{SessionID: session, Arm: arm, Root: f.st.Root(), ConfigDir: f.cfg}
}

func pairOf(t *testing.T, f *fix) *Pair {
	t.Helper()
	p, err := PairOf(f.ask(sOff, steer.ArmOff), f.ask(sSteer, steer.ArmSteer), f.now)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// TestMS1e_PairReadsBothSides: every row of the pair equals the fixture's
// count, and each side's tokens are its own transcript's, never another
// session's.
func TestMS1e_PairReadsBothSides(t *testing.T) {
	f := twoArms(t)
	p := pairOf(t, f)
	off, on := p.Sides[0], p.Sides[1]
	for _, c := range []struct {
		name      string
		got, want any
	}{
		{"off session", off.SessionID, sOff},
		{"off arm", off.RecordedArm, "off"},
		{"off lines", off.Lines, 4},
		{"off notes given", off.NotesGiven, 0},
		{"off notes withheld", off.NotesWithheld, 3},
		{"off notes not given", off.NotesNotGiven, []Reason{{Reason: "arm_off", Count: 3}, {Reason: "budget_pattern", Count: 1}}},
		{"off findings", len(off.Findings), 2},
		{"off calls", off.Calls, 6},
		{"off executed", off.Executed, 6},
		{"off writes", off.Writes, 3},
		{"off test runs", off.TestRuns, TestRuns{Failed: 1, Passed: 1}},
		{"off gates", off.Gates, []Gate{{Gate: "go", State: "green"}}},
		{"steer arm", on.RecordedArm, "steer"},
		{"steer lines", on.Lines, 4},
		{"steer notes given", on.NotesGiven, 3},
		{"steer notes withheld", on.NotesWithheld, 0},
		{"steer notes not given", on.NotesNotGiven, []Reason{{Reason: "budget_pattern", Count: 1}}},
		{"steer findings", len(on.Findings), 2},
		{"steer calls", on.Calls, 4},
		{"steer executed", on.Executed, 3},
		{"steer writes", on.Writes, 1},
		{"steer test runs", on.TestRuns, TestRuns{Failed: 1, Passed: 1}},
	} {
		if !equal(c.got, c.want) {
			t.Errorf("%s = %+v, want %+v", c.name, c.got, c.want)
		}
	}
	for _, s := range p.Sides {
		sum := s.NotesGiven
		for _, r := range s.NotesNotGiven {
			sum += r.Count
		}
		if sum != s.Lines {
			t.Errorf("%s: %d given plus not given, of %d lines: the identity broke", s.SessionID, sum, s.Lines)
		}
	}
	reg := findingOf(off, pattern.RegressionLocation)
	if reg == nil || reg.Lines != 3 || reg.LastAfterSeq != 6 || reg.ResolvedAtSeq == nil || *reg.ResolvedAtSeq != 8 {
		t.Errorf("off's regression finding = %+v, want 3 lines, last after #6, resolved at #8", reg)
	}
	if r := findingOf(off, pattern.IdenticalRetry); r == nil || r.ResolvedAtSeq != nil {
		t.Errorf("off's identical-retry finding = %+v, want one unresolved: no gate names it", r)
	}
	if r := findingOf(on, pattern.RegressionLocation); r == nil || r.ResolvedAtSeq != nil {
		t.Errorf("steer's regression finding = %+v, want one unresolved: its gate's last green, #3, is before it", r)
	}
	if r := findingOf(on, pattern.LearnedFact); r != nil {
		t.Errorf("a learned-fact note counts as a finding: %+v", r)
	}
	if off.WallMS != 11*60_000 || on.WallMS != 6*60_000 {
		t.Errorf("wall times %d and %d ms, want first to last record: %d and %d", off.WallMS, on.WallMS, 11*60_000, 6*60_000)
	}
	for _, c := range []struct {
		side    Side
		n       int
		in, out int64
	}{{off, 3, 300, 30}, {on, 2, 400, 40}} {
		tr := c.side.Transcript
		if tr == nil {
			t.Fatalf("%s has no transcript", c.side.SessionID)
		}
		if tr.Responses != c.n || tr.Tokens.Input != c.in || tr.Tokens.Output != c.out || tr.Tokens.CacheRead != int64(7*c.n) {
			t.Errorf("%s transcript = %+v, want %d responses, %d in, %d out, %d cache read (its own file only)",
				c.side.SessionID, tr, c.n, c.in, c.out, 7*c.n)
		}
		if !slices.Equal(tr.Models, []string{"claude-haiku-4-5"}) {
			t.Errorf("%s models = %v", c.side.SessionID, tr.Models)
		}
	}
	if !slices.Equal(p.Caveats, []string{CaveatNIsOne}) {
		t.Errorf("caveats = %v, want only %s", p.Caveats, CaveatNIsOne)
	}
}

func findingOf(s Side, p pattern.ID) *Finding {
	for i := range s.Findings {
		if s.Findings[i].Pattern == string(p) {
			return &s.Findings[i]
		}
	}
	return nil
}

func equal(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}

// TestMS1r_PairSaysWhatItCouldNotRead: a side with no transcript, a side
// with no steer line and two sides on different models each carry their
// caveat, and the pair still reads.
func TestMS1r_PairSaysWhatItCouldNotRead(t *testing.T) {
	f := newFix(t)
	f.call(sOff, "Bash", goTest, store.ExecOK)
	f.call(sSteer, "Bash", goTest, store.ExecOK)
	f.lines(sSteer, delivered("t1", pattern.RegressionLocation, "go", 1))
	f.transcript(sSteer, "claude-sonnet-5", 1, 1, 1)
	p := pairOf(t, f)
	if p.Sides[0].RecordedArm != ArmUnknown || p.Sides[0].Transcript != nil {
		t.Errorf("the off side reads arm %q and transcript %+v, want %q and none", p.Sides[0].RecordedArm, p.Sides[0].Transcript, ArmUnknown)
	}
	want := []string{CaveatNIsOne, CaveatArmUnrecorded, CaveatTranscriptMissing}
	if !slices.Equal(p.Caveats, want) {
		t.Errorf("caveats = %v, want %v", p.Caveats, want)
	}

	g := twoArms(t)
	g.transcript(sSteer, "claude-sonnet-5", 2, 200, 20)
	if q := pairOf(t, g); !slices.Contains(q.Caveats, CaveatDifferentModels) {
		t.Errorf("caveats = %v, want %s", q.Caveats, CaveatDifferentModels)
	}
}

// TestMS1s_PairReadsTwoStores: each arm of a paired run starts from its own
// copy of the store and the configuration directory, and each side is read
// from its own, never the other's.
func TestMS1s_PairReadsTwoStores(t *testing.T) {
	f, g := newFix(t), newFix(t)
	f.call(sOff, "Bash", goTest, store.ExecFailed)
	f.lines(sOff, shadow("o1", pattern.RegressionLocation, "go", 1))
	f.transcript(sOff, "claude-haiku-4-5", 1, 5, 5)
	g.call(sSteer, "Bash", goTest, store.ExecOK)
	g.call(sSteer, "Edit", edit, store.ExecOK)
	g.lines(sSteer, delivered("t1", pattern.RegressionLocation, "go", 1))
	g.transcript(sSteer, "claude-haiku-4-5", 2, 5, 5)
	p, err := PairOf(f.ask(sOff, steer.ArmOff), g.ask(sSteer, steer.ArmSteer), g.now)
	if err != nil {
		t.Fatal(err)
	}
	if p.Sides[0].Calls != 1 || p.Sides[0].NotesWithheld != 1 || p.Sides[0].Transcript == nil || p.Sides[0].Transcript.Responses != 1 {
		t.Errorf("the off side = %+v, want its own store's one call and withheld note and its own transcript", p.Sides[0])
	}
	if p.Sides[1].Calls != 2 || p.Sides[1].NotesGiven != 1 || p.Sides[1].Transcript == nil || p.Sides[1].Transcript.Responses != 2 {
		t.Errorf("the steer side = %+v, want its own store's two calls and note and its own transcript", p.Sides[1])
	}
	if _, err := PairOf(f.ask(sOff, steer.ArmOff), f.ask(sSteer, steer.ArmSteer), f.now); err == nil {
		t.Error("the steer session read from the off arm's store, where it has no records")
	}
}

// TestMS1f_PairRefusesMixedArms: a session whose lines name two arms, a
// session whose lines name an arm other than the one asked, a session asked
// for twice, a session with no records, a prompt over the router's cap, an
// unforced arm and a store that is not a lab are each an error naming it.
func TestMS1f_PairRefusesMixedArms(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(f *fix)
		a, b func(f *fix) Ask
		want string
	}{
		{"two arms in one session", func(f *fix) {
			f.lines(sOff, delivered("x1", pattern.RegressionLocation, "go", 7))
		}, on(sOff, steer.ArmOff), on(sSteer, steer.ArmSteer), sOff},
		{"a live session asked as off", func(f *fix) {
			l := delivered("x2", pattern.RegressionLocation, "go", 7)
			l.Arm = steer.ArmLive
			f.lines(sOther, l)
			f.call(sOther, "Bash", goTest, store.ExecOK)
		}, on(sOther, steer.ArmOff), on(sSteer, steer.ArmSteer), sOther},
		{"the arms swapped", func(*fix) {}, on(sOff, steer.ArmSteer), on(sSteer, steer.ArmOff), sOff},
		{"one session twice", func(*fix) {}, on(sOff, steer.ArmOff), on(sOff, steer.ArmOff), sOff},
		{"no records", func(*fix) {}, on("sess-none", steer.ArmOff), on(sSteer, steer.ArmSteer), "sess-none"},
		{"more notes on a prompt than the router gives", func(f *fix) {
			f.lines(sSteer, delivered("x3", pattern.IdenticalRetry, "retry-2", 7), delivered("x4", pattern.IdenticalRetry, "retry-3", 8))
		}, on(sOff, steer.ArmOff), on(sSteer, steer.ArmSteer), sSteer},
		{"an arm that is not forced", func(*fix) {}, on(sOff, steer.ArmLive), on(sSteer, steer.ArmSteer), sOff},
		{"a store that is not a lab", func(f *fix) {
			if err := os.Remove(filepath.Join(f.st.Root(), store.FileLab)); err != nil {
				f.t.Fatal(err)
			}
		}, on(sOff, steer.ArmOff), on(sSteer, steer.ArmSteer), "not a lab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := twoArms(t)
			tc.edit(f)
			_, err := PairOf(tc.a(f), tc.b(f), f.now)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("PairOf = %v, want an error naming %s", err, tc.want)
			}
		})
	}
}

// on is session asked as arm in the case's store.
func on(session string, arm steer.Arm) func(f *fix) Ask {
	return func(f *fix) Ask { return f.ask(session, arm) }
}

// fun15 and msr14 are the words no measurement output may hold (tech
// 2.2.11's list and the MEASURE chapter's), matched whole in any case.
var (
	fun15 = []string{"saved", "prevent", "would have", "without rashomon", "completed", "done", "the task", "fixed",
		"compliant", "%", "rate"}
	msr14 = []string{"compliant", "correct", "steers your agent", "detects lies", "lie", "guarantee"}
)

// TestMS1g_PairSaysHypothesis: the text closes on the n = 1 sentence, and
// neither the text nor the JSON holds a banned word.
func TestMS1g_PairSaysHypothesis(t *testing.T) {
	f := twoArms(t)
	f.call(sSteer, "Bash", gitPush, "")
	g := newFix(t)
	g.call(sOff, "Bash", goTest, store.ExecOK)
	g.call(sSteer, "Bash", goTest, store.ExecOK)
	for _, p := range []*Pair{pairOf(t, f), pairOf(t, g)} {
		var text, js bytes.Buffer
		if err := Text(&text, p); err != nil {
			t.Fatal(err)
		}
		if err := JSON(&js, p); err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimRight(text.String(), "\n"), "\n")
		if last := strings.TrimSpace(lines[len(lines)-1]); last != Hypothesis {
			t.Errorf("the text's last line is %q, want %q", last, Hypothesis)
		}
		for _, out := range []string{text.String(), js.String()} {
			lower := strings.ToLower(out)
			for _, w := range append(slices.Clone(fun15), msr14...) {
				re := regexp.MustCompile(`\b` + regexp.QuoteMeta(w) + `\b`)
				if w == "%" {
					re = regexp.MustCompile(`%`)
				}
				if re.MatchString(lower) {
					t.Errorf("the output holds %q:\n%s", w, out)
				}
			}
		}
	}
}

// TestMS1h_PairWritesNothing: reading the pair and printing both forms leaves
// the store and the transcripts byte- and mtime-identical.
func TestMS1h_PairWritesNothing(t *testing.T) {
	f := twoArms(t)
	before := snapshot(t, f.st.Root()) + snapshot(t, f.cfg)
	p := pairOf(t, f)
	if err := Text(io.Discard, p); err != nil {
		t.Fatal(err)
	}
	if err := JSON(io.Discard, p); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, f.st.Root()) + snapshot(t, f.cfg); after != before {
		t.Errorf("the pair wrote:\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func snapshot(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %s %d\n", p, info.ModTime().Format(time.RFC3339Nano), info.Size())
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

// TestMS1m_PairKeySetsAreClosed: the JSON's key set at every depth is the
// one this test lists, so a field added to a struct is a field added here.
func TestMS1m_PairKeySetsAreClosed(t *testing.T) {
	want := map[string][]string{
		"":                        {"caveats", "generated_at_unix_ms", "rules_version", "schema", "sides"},
		"sides":                   {"arm", "calls", "executed", "findings", "gates", "lines", "notes_given", "notes_not_given", "notes_withheld", "recorded_arm", "session_id", "test_runs", "transcript", "wall_ms", "writes"},
		"sides.findings":          {"key", "last_after_seq", "lines", "pattern", "resolved_at_seq"},
		"sides.gates":             {"gate", "state"},
		"sides.notes_not_given":   {"count", "reason"},
		"sides.test_runs":         {"failed", "passed", "unended"},
		"sides.transcript":        {"files", "models", "responses", "subagent_responses", "tokens", "unread_lines"},
		"sides.transcript.tokens": {"cache_read", "cache_write_1h", "cache_write_5m", "input", "output"},
	}
	f := twoArms(t)
	var b bytes.Buffer
	if err := JSON(&b, pairOf(t, f)); err != nil {
		t.Fatal(err)
	}
	var doc any
	if err := json.Unmarshal(b.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	keysAt(doc, "", got)
	for path, keys := range got {
		slices.Sort(keys)
		keys = slices.Compact(keys)
		if !slices.Equal(keys, want[path]) {
			t.Errorf("keys at %q = %v, want %v", path, keys, want[path])
		}
	}
	for path := range want {
		if _, ok := got[path]; !ok {
			t.Errorf("no object at %q; the fixture should fill every level", path)
		}
	}
}

func keysAt(v any, path string, out map[string][]string) {
	switch x := v.(type) {
	case map[string]any:
		for k, c := range x {
			out[path] = append(out[path], k)
			keysAt(c, strings.TrimPrefix(path+"."+k, "."), out)
		}
		if _, ok := out[path]; !ok {
			out[path] = []string{}
		}
	case []any:
		for _, c := range x {
			keysAt(c, path, out)
		}
	}
}

// TestMS1zd_PairSkipsTheStopsEvidenceLines (CHANNEL-EVIDENCE1): a Stop that
// judges a delivered note appends a steer_evidence line to the steer arm's
// log. It names no arm and gives nothing, so the pair reads the session as
// it did without it: one arm, the same lines and notes, no error.
// Break: decode an evidence line as a steer line.
func TestMS1zd_PairSkipsTheStopsEvidenceLines(t *testing.T) {
	f := twoArms(t)
	before := pairOf(t, f)
	ev := &steer.EvidenceLine{Type: steer.EvidenceLineType, V: 1, SteerID: "s1", SessionID: sSteer, PromptID: ptr("p1"),
		AtUnixMS: 1, Event: "PostToolUseFailure", Evidence: steer.EvidenceSeen}
	if err := f.st.AppendSteer(sSteer, func(io.ReaderAt) ([]store.SteerLine, error) { return []store.SteerLine{ev}, nil }); err != nil {
		t.Fatal(err)
	}
	after, err := PairOf(f.ask(sOff, steer.ArmOff), f.ask(sSteer, steer.ArmSteer), f.now)
	if err != nil {
		t.Fatalf("PairOf with an evidence line = %v, want the pair read as before", err)
	}
	before.GeneratedAt, after.GeneratedAt = 0, 0
	b, _ := json.Marshal(before)
	a, _ := json.Marshal(after)
	if !bytes.Equal(a, b) {
		t.Errorf("an evidence line changed the pair:\n%s\nwant\n%s", a, b)
	}
}
