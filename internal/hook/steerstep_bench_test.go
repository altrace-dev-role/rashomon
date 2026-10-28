package hook

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
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

func newBenchSteer(b *testing.B, n int) *benchSteer {
	b.Helper()
	b.Setenv("CLAUDE_CONFIG_DIR", b.TempDir())
	st, err := store.Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
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

// logged fails the step row unless each of its b.N iterations appended one
// regression-location line suppressed by unverified_channel, its note's
// slots bound (a suppressed line carries no text, RES-32).
func (f *benchSteer) logged(b *testing.B, before int) {
	b.Helper()
	lines := f.lines(b)
	if got := len(lines) - before; got != b.N {
		b.Fatalf("%d steer lines appended over %d iterations, want one each", got, b.N)
	}
	for _, l := range lines[before:] {
		by := "null"
		if l.SuppressedBy != nil {
			by = string(*l.SuppressedBy)
		}
		if l.Pattern != string(pattern.RegressionLocation) || by != string(steer.SuppressedByUnverifiedChannel) || len(l.Slots) == 0 {
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
// unverified_channel, and the work-done check after the loop reads exactly
// that, b.N lines and no budget suppression.
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
		b.Run(c.step, func(b *testing.B) {
			before := len(f.lines(b))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				f.post.promptID = next()
				f.post.Steer(env)
			}
			b.StopTimer()
			f.logged(b, before)
		})
	}
}
