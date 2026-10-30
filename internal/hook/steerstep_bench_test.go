package hook

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// benchSteer is one session's record, built through Handler and Post on a
// deferred store as cmd opens it: n calls in BenchmarkEvaluate's two gate
// families (go test every seventh call, go vet every eleventh, an Edit of
// one file otherwise) and a failed go test as call n+1, whose Post the step
// runs on. The call after n is the red so that an Edit lies between it and
// the last green go test at both sizes. Records are a second apart.
type benchSteer struct {
	st   *store.Store
	now  time.Time
	post *Post
}

const benchSteerSession = "sess-bench"

// benchStore opens a fixture's store at a fresh root pinned to payload.KeyHex
// and payload.InstallIDSteer (payload.PinSteer), as the recap, digest and
// ReadRun fixtures are pinned to KeyHex and InstallID. A fresh store's key and
// install id are random, and the step reads the run back through json/v2's
// 256-slot string cache, which the record's keyed digests and its coverage
// records' install id reach: unpinned, eval_6000 moved by up to 5,950
// allocs/op between processes, at base as much as after a change (D104, D126),
// so a D103 window read the draw, not the code (BENCH-KEY1).
func benchStore(tb testing.TB) *store.Store {
	tb.Helper()
	root := tb.TempDir()
	if err := payload.PinSteer(root); err != nil {
		tb.Fatal(err)
	}
	st, err := store.Open(root)
	if err != nil {
		tb.Fatal(err)
	}
	return st
}

// TestBENCHKEY1_FixtureStoreIsPinned: two fixture stores open with KeyHex and
// InstallIDSteer, so every process the steer rows run in reads the same
// digests and install id.
func TestBENCHKEY1_FixtureStoreIsPinned(t *testing.T) {
	want, err := hex.DecodeString(payload.KeyHex)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range []*store.Store{benchStore(t), benchStore(t)} {
		if !bytes.Equal(st.Key(), want) || st.InstallID() != payload.InstallIDSteer {
			t.Fatalf("fixture key %x and install id %s, want KeyHex %x and InstallIDSteer %s",
				st.Key(), st.InstallID(), want, payload.InstallIDSteer)
		}
	}
}

func newBenchSteer(b *testing.B, n int) *benchSteer {
	b.Helper()
	b.Setenv("CLAUDE_CONFIG_DIR", b.TempDir())
	st := benchStore(b)
	st.DeferDurability()
	if err := store.WriteRecordScopeMarker(st.Root(), store.RecordScopeV5); err != nil {
		b.Fatal(err)
	}
	if err := store.WriteSteerDisclosureMarker(st.Root(), steer.DisclosureVersion); err != nil {
		b.Fatal(err)
	}
	f := &benchSteer{st: st, now: time.UnixMilli(1_800_000_000_000)}
	// The post coverage record a verified install writes (card 2.3 A2).
	cov := store.Coverage{Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion, RecordedAtMS: f.now.UnixMilli(),
		SessionID: benchSteerSession, Phase: store.PhasePost, State: store.StateVerified,
		HookEntry: store.EntryPresentSettings, Probe: store.ProbeFresh}
	if err := st.AppendCoverage(cov); err != nil {
		b.Fatal(err)
	}
	edit := map[string]any{"file_path": "/tmp/project/pkg/a.go", "old_string": "a", "new_string": "b"}
	for i := 1; i <= n; i++ {
		switch {
		case i%7 == 0:
			f.call(b, i, "Bash", map[string]any{"command": "go test ./..."}, 0)
		case i%11 == 0:
			f.call(b, i, "Bash", map[string]any{"command": "go vet ./..."}, 0)
		default:
			f.call(b, i, "Edit", edit, 0)
		}
	}
	f.post = f.call(b, n+1, "Bash", map[string]any{"command": "go test ./..."}, 1)
	// cmd flushes the record before the step (post.go).
	if err := f.post.Flush(); err != nil {
		b.Fatal(err)
	}
	return f
}

// call lands call i, its PreToolUse through Handler and its post through
// Post's Capture (a failure event when exit is not 0), and returns the Post.
func (f *benchSteer) call(b *testing.B, i int, tool string, input map[string]any, exit int) *Post {
	b.Helper()
	clock := func() time.Time { return f.now }
	id := fmt.Sprintf("toolu_bench%06d", i)
	h := New(f.st, clock)
	h.Close(&Signals{}, h.Capture(strings.NewReader(f.payload(b, "PreToolUse", id, tool, input, nil))))
	f.now = f.now.Add(time.Second)
	event, more := "PostToolUse", map[string]any{"tool_response": map[string]any{"stdout": ""}}
	if exit != 0 {
		event, more = FailureEvent, map[string]any{"error": fmt.Sprint("Exit code ", exit), "is_interrupt": false, "duration_ms": 30}
	}
	p := NewPost(f.st, clock)
	if err := p.Capture(strings.NewReader(f.payload(b, event, id, tool, input, more))); err != nil {
		b.Fatalf("post %s: %v", id, err)
	}
	f.now = f.now.Add(time.Second)
	return p
}

func (f *benchSteer) payload(b *testing.B, event, id, tool string, input, extra map[string]any) string {
	b.Helper()
	p := map[string]any{"session_id": benchSteerSession, "tool_use_id": id, "prompt_id": "p0", "transcript_path": "/tmp/t.jsonl",
		"permission_mode": "default", "tool_name": tool, "cwd": "/tmp/project", "hook_event_name": event, "tool_input": input}
	maps.Copy(p, extra)
	raw, err := json.Marshal(p)
	if err != nil {
		b.Fatal(err)
	}
	return string(raw)
}

// env is what cmd builds for Claude Code 2.1.280 on the CLI, attended, with
// consent read from the marker as steerEnv reads it (D84).
func (f *benchSteer) env() SteerEnv {
	return SteerEnv{HarnessID: "claude_code", Surface: "cli", MirrorInBand: true, AIAgent: "claude-code_2-1-280_harness",
		Consent: store.SteerDisclosureMarker, Now: f.now}
}

// premise fails the benchmark unless the record is what the rows claim to
// measure: complete, two gates, and one regression-location candidate with
// its note rendered.
func (f *benchSteer) premise(b *testing.B) {
	b.Helper()
	run, err := f.st.ReadRun(benchSteerSession)
	if err != nil {
		b.Fatal(err)
	}
	pt := verify.Point{Seq: f.post.landedSeq, SessionID: benchSteerSession}
	if snap := verify.Evaluate(run, nil, nil, pt, f.now); snap.Recording != verify.Complete || len(snap.Gates) != 2 {
		b.Fatalf("the fixture gives %s with %d gates, want complete with two", snap.Recording, len(snap.Gates))
	}
	_, view, ok := f.post.judge(f.env())
	if !ok || len(view.Findings) != 1 || view.Findings[0].Message.Text() == "" {
		b.Fatalf("judge = %v with %d findings, want one candidate with its note", ok, len(view.Findings))
	}
}

// lines is the session's steer log, decoded.
func (f *benchSteer) lines(b *testing.B) []steer.Line {
	b.Helper()
	raw, skipped, err := f.st.ReadSteer(benchSteerSession)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		b.Fatal(err)
	}
	if skipped != 0 {
		b.Fatalf("the steer log has %d unreadable lines", skipped)
	}
	out := make([]steer.Line, len(raw))
	for i, l := range raw {
		if err := json.Unmarshal(l, &out[i]); err != nil {
			b.Fatal(err)
		}
	}
	return out
}

// keepSteerLog returns the function that puts the session's steer log back
// as it is now: removed if there is none, cut back to its length if there
// is. The step rows call it after each iteration, so every iteration's Route
// reads the same log (D103: a benchmark measures one state).
func (f *benchSteer) keepSteerLog(b *testing.B) func() {
	b.Helper()
	path := filepath.Join(f.st.RunDir(benchSteerSession), store.FileSteer)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return func() {
			if err := os.Remove(path); err != nil {
				b.Fatal(err)
			}
		}
	}
	if err != nil {
		b.Fatal(err)
	}
	return func() {
		if err := os.Truncate(path, info.Size()); err != nil {
			b.Fatal(err)
		}
	}
}

// logged fails the step row unless the step appended want lines past the
// log's first before, each a regression-location line suppressed by
// unverified_channel with its note's slots bound (a suppressed line carries
// no text, RES-32) and no notice, the session having had it.
func (f *benchSteer) logged(b *testing.B, before, want int) {
	b.Helper()
	lines := f.lines(b)
	if got := len(lines) - before; got != want {
		b.Fatalf("%d steer lines appended, want %d", got, want)
	}
	for _, l := range lines[before:] {
		by := "null"
		if l.SuppressedBy != nil {
			by = string(*l.SuppressedBy)
		}
		if l.Pattern != string(pattern.RegressionLocation) || by != string(steer.SuppressedByUnverifiedChannel) || len(l.Slots) == 0 || l.Notice != nil {
			b.Fatalf("a step line is %s suppressed by %s with %d slots, want regression-location, unverified_channel and the note's slots",
				l.Pattern, by, len(l.Slots))
		}
	}
}

// BenchmarkPostSteer measures the steer step on a failed runner call at the
// end of a 100-call and a 6,000-call session (card 2.3 R10). eval is the
// compute, judge: ReadRunConsistent, SteerAt and Route's context, without
// Route, the append or the sync; the 5 ms rule is this row's. step is the
// whole step as cmd calls it, Steer: Route, the line's append and the Flush
// that makes it durable, so the sync's share shows beside the compute.
//
// Every iteration rotates the prompt id, a fresh id assigned inside the
// loop, so Route's budget scan reads one foreign batch and stops, and 2.2's
// three-notes-per-prompt cap never turns later iterations into cheaper
// suppressed ones. Under the shipped client table the note is logged, not
// printed (D69): each step iteration appends one line suppressed by
// unverified_channel, and the work-done check after it, outside the timer,
// reads exactly that, one line and no budget suppression. The log is then
// put back as it was before the loop (keepSteerLog), so every iteration
// steps on the same record and the same log (D103).
func BenchmarkPostSteer(b *testing.B) {
	// A table of literal names: RES36 reads every b.Run name statically.
	for _, c := range []struct {
		eval, step string
		n          int
	}{{eval: "eval_100", step: "step_100", n: 100}, {eval: "eval_6000", step: "step_6000", n: 6000}} {
		f := newBenchSteer(b, c.n)
		f.premise(b)
		env := f.env()
		prompt := 0
		next := func() string { prompt++; return fmt.Sprint("p", prompt) }
		b.Run(c.eval, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				f.post.promptID = next()
				if _, view, ok := f.post.judge(env); !ok || len(view.Findings) != 1 {
					b.Fatalf("judge = %v with %d findings", ok, len(view.Findings))
				}
			}
		})
		b.Run(c.step, f.stepRow(env, next))
	}
	// step_100_spec is step_100 with a spec to find (card 3.1 (c), Gate A
	// 3): the post's cwd is two levels below a .git root that holds a 4 KiB
	// spec, so the row adds discovery's walk, its read and Parse. The record
	// is step_100's; only the cwd discovery reads moves.
	f := newBenchSteer(b, 100)
	f.post.cwd = benchSpecCWD(b)
	f.premise(b)
	env := f.env()
	if _, view, _ := f.post.judge(env); view.Spec == nil || view.Spec.Path == "" || len(view.Spec.Errors) != 0 {
		b.Fatalf("the spec row's view has spec %+v, want the 4 KiB spec found", view.Spec)
	}
	prompt := 0
	b.Run("step_100_spec", f.stepRow(env, func() string { prompt++; return fmt.Sprint("p", prompt) }))
	// write_6000_unarmed and write_6000_armed are the step on an Edit's post
	// after the 6,000-call record (card STALE-NUDGE1, D166). Unarmed, the
	// step stops at the marker's Lstat, which every write pays; armed, it
	// reads the run and judges it under a learned index, which a write pays
	// about once per green gate run. The Edit names a file under a real
	// project with an index requiring both families, so the armed step goes
	// the whole way; no gate's green is left behind by this write (the vet
	// family's was left by an earlier Edit, the test family is red), so it
	// appends no line.
	w, wf := newBenchWrite(b, 6000)
	env = wf.env()
	b.Run("write_6000_unarmed", wf.writeRow(w, env, false))
	b.Run("write_6000_armed", wf.writeRow(w, env, true))
}

// newBenchWrite is newBenchSteer's record at n with an Edit of a file under a
// fresh project landed after the red, and that project's index requiring
// every gate family the record holds, as two sessions within 30 days do. The
// Edit's post is returned, its cwd the project.
func newBenchWrite(b *testing.B, n int) (*Post, *benchSteer) {
	b.Helper()
	f := newBenchSteer(b, n)
	dir, err := filepath.EvalSymlinks(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	edit := map[string]any{"file_path": filepath.Join(dir, "pkg", "a.go"), "old_string": "a", "new_string": "b"}
	w := f.call(b, n+2, "Edit", edit, 0)
	w.cwd = dir
	if err := w.Flush(); err != nil {
		b.Fatal(err)
	}
	run, err := f.st.ReadRun(benchSteerSession)
	if err != nil {
		b.Fatal(err)
	}
	snap := verify.Evaluate(run, nil, nil, verify.Point{Seq: w.landedSeq, SessionID: benchSteerSession}, f.now)
	key, ms := baseline.Key(dir), f.now.UnixMilli()
	idx := &verify.ProjectIndex{ProjectKey: key}
	for _, g := range snap.Gates {
		idx.Entries = append(idx.Entries, verify.IndexEntry{Family: g.Family, Kind: g.Kind, Sessions: 2,
			LastSession: "s0", FirstSeen: ms, LastSeen: ms})
	}
	slices.SortFunc(idx.Entries, func(x, y verify.IndexEntry) int { return strings.Compare(x.Family, y.Family) })
	if _, err := verify.WriteIndex(verify.IndexPath(f.st.Root(), key), idx); err != nil {
		b.Fatal(err)
	}
	return w, f
}

// writeRow is a write row: the whole step on w's post per iteration, armed
// first outside the timer when armed is set. The check after it, outside the
// timer too, reads what the step must leave: no arm, and no steer line.
func (f *benchSteer) writeRow(w *Post, env SteerEnv, armed bool) func(*testing.B) {
	return func(b *testing.B) {
		before := len(f.lines(b))
		f.writePremise(b, w, env, armed)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			if armed {
				b.StopTimer()
				if err := f.st.ArmStale(benchSteerSession); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
			}
			w.Steer(env)
			b.StopTimer()
			if on, err := f.st.StaleArmed(benchSteerSession); err != nil || on {
				b.Fatalf("after the step the trigger is armed (%v, %v)", on, err)
			}
			if got := len(f.lines(b)) - before; got != 0 {
				b.Fatalf("%d steer lines appended, want none", got)
			}
			b.StartTimer()
		}
	}
}

// writePremise fails the row unless judge does what the row claims: an
// unarmed write is not judged, and an armed one is, and takes the arm, with
// no finding.
func (f *benchSteer) writePremise(b *testing.B, w *Post, env SteerEnv, armed bool) {
	b.Helper()
	if armed {
		if err := f.st.ArmStale(benchSteerSession); err != nil {
			b.Fatal(err)
		}
	}
	_, view, ok := w.judge(env)
	if ok != armed || len(view.Findings) != 0 {
		b.Fatalf("judge = %v with %d findings, want %v with none", ok, len(view.Findings), armed)
	}
	if on, err := f.st.StaleArmed(benchSteerSession); err != nil || on {
		b.Fatalf("after judge the trigger is armed (%v, %v)", on, err)
	}
}

// stepRow is a step row: the whole step per iteration, on a fresh prompt id
// from next, with the work-done check and the log put back outside the
// timer.
func (f *benchSteer) stepRow(env SteerEnv, next func() string) func(*testing.B) {
	return func(b *testing.B) {
		before, restore := len(f.lines(b)), f.keepSteerLog(b)
		// The session has had its channel notice, so every iteration is a
		// later shadowed call, which pays the marker's refused create and
		// prints nothing: one state for every iteration.
		f.st.TakeNotice(benchSteerSession, store.FileNoticeChannel)
		b.ReportAllocs()
		b.ResetTimer()
		for range b.N {
			f.post.promptID = next()
			f.post.Steer(env)
			b.StopTimer()
			f.logged(b, before, 1)
			restore()
			b.StartTimer()
		}
	}
}

// benchSpecCWD is a cwd two levels below a .git root whose
// .claude/rashomon/spec.json is benchSpecDoc, made fresh under tb's temporary
// directory with symlinks resolved, as benchProjectCWD (cmd) makes its tree.
func benchSpecCWD(tb testing.TB) string {
	tb.Helper()
	base, err := filepath.EvalSymlinks(tb.TempDir())
	if err != nil {
		tb.Fatal(err)
	}
	cwd := filepath.Join(base, "internal", "batch")
	dir := filepath.Join(base, ".claude", "rashomon")
	for _, d := range []string{cwd, dir, filepath.Join(base, ".git")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			tb.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.json"), []byte(benchSpecDoc()), 0o644); err != nil {
		tb.Fatal(err)
	}
	return cwd
}

// benchSpecDoc is a spec of at least 4 KiB: 16 gates no call of the fixture
// runs, and protect_path rules until the size is reached.
func benchSpecDoc() string {
	var gates, rules []string
	for i := range 16 {
		gates = append(gates, fmt.Sprintf(`{"id": "g%02d", "kind": "custom", "argv": ["make", "check-%02d"]}`, i, i))
	}
	doc := func() string {
		return `{"rashomon_spec": 1, "gates": [` + strings.Join(gates, ", ") + `], "rules": [` + strings.Join(rules, ", ") + `]}`
	}
	for i := 0; len(doc()) < 4096; i++ {
		rules = append(rules, fmt.Sprintf(`{"id": "frozen-%02d", "kind": "protect_path", "paths": ["db/m%02d/**"], `+
			`"why": "Applied migrations are immutable.", "hard_control": "CODEOWNERS on db/m%02d"}`, i, i, i))
	}
	return doc()
}
