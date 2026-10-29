package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/replay"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// B2b: the rebuilt timeline, replayed through the live recorder's entry points,
// is held against the captured payloads recorded through the same entry points.
// Helpers first; the tests (TestREPLAY7_ and on) follow in the files named for
// what they hold.

var pairedFixtures = []string{"2.1.280", "2.1.312"}

// fixtureSession is a paired fixture: the payloads Claude Code sent, and the
// timeline the replay rebuilds from its transcript.
type fixtureSession struct {
	version  string
	captured []step
	path     string
	tl       replay.Timeline
	opt      replay.Options
}

func loadFixtureSession(t *testing.T, version string) fixtureSession {
	t.Helper()
	f := fixtureSession{version: version, captured: readCaptured(t, version)}
	for _, s := range f.captured {
		var m struct {
			Path string `json:"transcript_path"`
		}
		_ = json.Unmarshal(s.raw, &m)
		f.path = m.Path
	}
	f.opt = replay.Options{TranscriptPath: f.path}
	f.tl = buildTimeline(t, fixtureTranscript(version), f.opt)
	return f
}

func fixtureTranscript(version string) string {
	return replayFixtures + version + "/transcript.jsonl"
}

func buildTimeline(t *testing.T, file string, opt replay.Options) replay.Timeline {
	t.Helper()
	fh, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer fh.Close()
	tl, err := replay.Build(fh, opt)
	if err != nil {
		t.Fatal(err)
	}
	return tl
}

func (f fixtureSession) openTranscript(t *testing.T) *os.File {
	t.Helper()
	fh, err := os.Open(fixtureTranscript(f.version))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fh.Close() })
	return fh
}

// capturedTool is the tool_use_id a captured payload names, "" for none.
func capturedTool(s step) string {
	var m struct {
		ID string `json:"tool_use_id"`
	}
	_ = json.Unmarshal(s.raw, &m)
	return m.ID
}

// liveSteps is run A: the payloads Claude Code sent, which the rebuilt events
// stand in for, each at the time its rebuilt twin carries. The captured
// SessionStart opens the session at the time of the first event; SessionEnd
// is left out because the replay does not replay it (flagProbeEndAbsent). The
// captured steps keep every key they have, duration_ms among them.
func (f fixtureSession) liveSteps(t *testing.T) (steps []replayStep, ids []string) {
	t.Helper()
	var hooks []step
	var start step
	for _, s := range f.captured {
		switch s.event {
		case install.EventSessionStart:
			start = s
		case install.EventSessionEnd:
		default:
			hooks = append(hooks, s)
		}
	}
	if len(hooks) != len(f.tl.Events) {
		t.Fatalf("%s: %d captured hook payloads and %d rebuilt events", f.version, len(hooks), len(f.tl.Events))
	}
	steps = append(steps, replayStep{start.event, start.raw, f.tl.Events[0].At})
	ids = append(ids, "")
	for i, s := range hooks {
		e := f.tl.Events[i]
		if s.event != e.Kind.String() || capturedTool(s) != e.ToolUseID {
			t.Fatalf("%s: captured step %d is %s %q and the rebuilt event is %s %q", f.version, i, s.event, capturedTool(s), e.Kind, e.ToolUseID)
		}
		steps = append(steps, replayStep{s.event, s.raw, e.At})
		ids = append(ids, e.ToolUseID)
	}
	return steps, ids
}

// replayed is what a run left behind.
type replayed struct {
	root    string
	world   *replayWorld
	outputs []replayOutput
	// verdicts are the Stop views the recorder computed, in order.
	verdicts []*stopVerdict
}

func newWorld(t *testing.T) *replayWorld {
	t.Helper()
	w, err := newReplayWorld(t.TempDir(), replayIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// runLive feeds steps to a fresh world through the entry points.
func runLive(t *testing.T, steps []replayStep, ids []string) replayed {
	t.Helper()
	w := newWorld(t)
	out := replayed{root: w.root, world: w}
	err := w.with(replayConfig{KeepOutput: true}, func(r *replayRun) error {
		for i, s := range steps {
			v, err := r.feed(s, ids[i])
			if err != nil {
				return err
			}
			if s.Event == install.EventStop {
				if v == nil {
					t.Fatalf("step %d: the Stop produced no view", i)
				}
				out.verdicts = append(out.verdicts, &stopVerdict{Snap: v.Snap, Summary: v.Summary, Turn: v.Turn})
			}
		}
		out.outputs = r.outputs
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// runRebuilt replays a transcript into a fresh world.
func runRebuilt(t *testing.T, src *os.File, opt replay.Options) (replayed, *replayResult) {
	t.Helper()
	w := newWorld(t)
	res, err := w.replayTranscript(src, opt, replayConfig{KeepOutput: true})
	if err != nil {
		t.Fatal(err)
	}
	out := replayed{root: w.root, world: w, outputs: res.Outputs}
	for _, s := range res.Stops {
		if s.Evaluated {
			out.verdicts = append(out.verdicts, s.Verdict)
		}
	}
	return out, res
}

// storedRecord is one record of the store: the file it is in, its line, and
// its JSON (a file that is not JSON lines is one record holding its text).
type storedRecord struct {
	File string
	Line int
	V    map[string]any
}

// dumpStore reads every file under the store root, in path order and line
// order.
func dumpStore(t *testing.T, root string) []storedRecord {
	t.Helper()
	var out []storedRecord
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		text := strings.TrimRight(string(b), "\n")
		if text == "" {
			return nil
		}
		for i, l := range strings.Split(text, "\n") {
			var v map[string]any
			if json.Unmarshal([]byte(l), &v) != nil || v == nil {
				v = map[string]any{"text": l}
			}
			out = append(out, storedRecord{filepath.ToSlash(rel), i + 1, v})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out
}

// kindCount counts records of a "type" in a dump.
func kindCount(recs []storedRecord, typ string) int {
	n := 0
	for _, r := range recs {
		if r.V["type"] == typ {
			n++
		}
	}
	return n
}

// What the equivalence comparison does not compare exactly. This is the whole
// list; anything else that differs between run A (the captured payloads) and
// run B (the rebuilt events), at any record, in any field, in any order, is a
// failure.
//
//   - duration_ms of an execution record. Claude Code measured it and sent it
//     in run A's payloads. A transcript holds no client-side duration, and
//     the replay stores null, as the recorder does for a call nobody timed,
//     by ruling (docs/autosteer-prescan.md, revision 7: "an explicit
//     exception to record equivalence"). It is never estimated from the
//     transcript's timestamps (the 210 ms between toolu_fixture03's lines
//     against the 171 ms measured shows why). Excluded from the comparison,
//     and asserted separately: every captured one is a positive number and
//     every replayed one is null, so the exception cannot hide a replay that
//     started storing an estimate, or a fixture that has none to except.
//   - the ids the recorder mints. A steer line's id and steer_id are drawn
//     per delivery, not derived from what was delivered. Each is replaced by
//     its ordinal among the store's minted ids, so which lines share an id
//     (the two keys of one line do) is still compared.
//
// Clock-derived fields are not on the list. Run A and run B are given the same
// clock, the time of the matching event, so every recorded_at_unix_ms, every
// last_seen and every file written from the clock (the probe marker,
// recap.json) is compared exactly. TestREPLAY8_ is what makes that possible.
const (
	exceptDuration = "duration_ms"
)

// normalised copies rec with the list above applied; mints is the store's
// minted ids so far.
func normalised(rec map[string]any, mints map[string]int) map[string]any {
	out := make(map[string]any, len(rec))
	for k, v := range rec {
		out[k] = v
	}
	if rec["type"] == "execution" {
		delete(out, exceptDuration)
	}
	if rec["type"] == "steer" {
		for _, k := range []string{"id", "steer_id"} {
			if s, ok := out[k].(string); ok {
				if _, seen := mints[s]; !seen {
					mints[s] = len(mints) + 1
				}
				out[k] = fmt.Sprintf("minted#%d", mints[s])
			}
		}
	}
	return out
}

// storeDiffs compares two dumps, record by record in order, field by field,
// and describes up to max differences.
func storeDiffs(a, b []storedRecord, max int) []string {
	var diffs []string
	add := func(format string, args ...any) {
		if len(diffs) < max {
			diffs = append(diffs, fmt.Sprintf(format, args...))
		}
	}
	files := map[string][2][]storedRecord{}
	for _, r := range a {
		p := files[r.File]
		p[0] = append(p[0], r)
		files[r.File] = p
	}
	for _, r := range b {
		p := files[r.File]
		p[1] = append(p[1], r)
		files[r.File] = p
	}
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	ma, mb := map[string]int{}, map[string]int{}
	for _, n := range names {
		pa, pb := files[n][0], files[n][1]
		if len(pa) != len(pb) {
			add("%s: %d records in A, %d in B", n, len(pa), len(pb))
		}
		for i := 0; i < len(pa) && i < len(pb); i++ {
			x, y := normalised(pa[i].V, ma), normalised(pb[i].V, mb)
			if reflect.DeepEqual(x, y) {
				continue
			}
			var keys []string
			seen := map[string]bool{}
			for k := range x {
				seen[k] = true
				keys = append(keys, k)
			}
			for k := range y {
				if !seen[k] {
					keys = append(keys, k)
				}
			}
			sort.Strings(keys)
			for _, k := range keys {
				if !reflect.DeepEqual(x[k], y[k]) {
					add("%s record %d (%v) field %q: A %s, B %s", n, i+1, x["type"], k, brief(x[k]), brief(y[k]))
				}
			}
		}
	}
	return diffs
}

func brief(v any) string {
	b, _ := json.Marshal(v)
	if len(b) > 160 {
		return string(b[:160]) + "..."
	}
	return string(b)
}

// verdictJSON is a verdict as comparable text.
func verdictJSON(v *stopVerdict) string {
	b, err := json.Marshal(v)
	if err != nil {
		return err.Error()
	}
	return string(b)
}

// runOf reads a session's run from a world's store.
func runOf(t *testing.T, w *replayWorld, session string) *store.Run {
	t.Helper()
	st, err := store.OpenExisting(w.root)
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.ReadRun(session)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

// gateFamilies lists the families verify reports at a point of a run, by
// GateState.Family, with each gate's last run.
func gateFamilies(run *store.Run, session string, seq int64, now time.Time) map[string]string {
	snap := verify.Evaluate(run, nil, nil, verify.Point{Kind: verify.PointQuery, Seq: seq, SessionID: session}, now)
	out := map[string]string{}
	for _, g := range snap.Gates {
		last := ""
		if g.LastRun != nil {
			last = g.LastRun.ToolUseID
		}
		out[g.Family] = last
	}
	return out
}

// TestREPLAY10_RebuiltTimelineRecordsWhatTheCapturedPayloadsDid replays each
// paired fixture twice through the live entry points, into two fresh worlds
// with the same clock: run A feeds the payloads Claude Code sent, run B the
// events the replay rebuilds from the transcript. Everything the recorder
// wrote must be the same, in order and field by field: declarations,
// executions, terminals, coverage, checkpoints, steer lines, the project
// index and the files the clock writes, and what the entry points printed,
// the steer notes included. The normalisation is the short list above
// (exceptDuration and the minted ids) and nothing else.
func TestREPLAY10_RebuiltTimelineRecordsWhatTheCapturedPayloadsDid(t *testing.T) {
	for _, version := range pairedFixtures {
		t.Run(version, func(t *testing.T) {
			f := loadFixtureSession(t, version)
			steps, ids := f.liveSteps(t)
			a := runLive(t, steps, ids)
			b, res := runRebuilt(t, f.openTranscript(t), f.opt)
			if len(res.Skipped) != 0 {
				t.Fatalf("events not fed: %+v", res.Skipped)
			}
			ra, rb := dumpStore(t, a.root), dumpStore(t, b.root)
			// Each kind of record the comparison is about must be there to
			// compare, so that an empty store cannot pass. 2.1.280's one failed
			// call is no gate, so it has no steer line.
			kinds := []string{"declaration", "execution", "terminal", "verify", "coverage"}
			if version == "2.1.312" {
				kinds = append(kinds, "steer")
			}
			for _, typ := range kinds {
				if n := kindCount(ra, typ); n == 0 {
					t.Errorf("run A recorded no %s record: the comparison would be vacuous for it", typ)
				}
			}
			if diffs := storeDiffs(ra, rb, 12); len(diffs) > 0 {
				t.Errorf("the rebuilt timeline records differently than the captured payloads (%d records in A, %d in B):\n  %s",
					len(ra), len(rb), strings.Join(diffs, "\n  "))
			}

			// The outputs, step for step: the rebuilt run also fed a
			// SessionStart first, as A did.
			if len(a.outputs) != len(b.outputs) {
				t.Fatalf("%d outputs in A, %d in B", len(a.outputs), len(b.outputs))
			}
			for i := range a.outputs {
				x, y := a.outputs[i], b.outputs[i]
				if x.Event != y.Event || x.ToolUseID != y.ToolUseID || x.Exit != y.Exit || x.Stderr != y.Stderr || x.Stdout != y.Stdout {
					t.Errorf("output %d (%s %s): A %+v, B %+v", i, x.Event, x.ToolUseID, x, y)
				}
			}

			// The one exception, asserted from both sides.
			var withDuration, replayedNull int
			for _, r := range ra {
				if r.V["type"] == "execution" {
					if d, ok := r.V[exceptDuration].(float64); ok && d > 0 {
						withDuration++
					}
				}
			}
			for _, r := range rb {
				if r.V["type"] == "execution" {
					if v, ok := r.V[exceptDuration]; ok && v == nil {
						replayedNull++
					} else {
						t.Errorf("a replayed execution stores duration_ms %v, not null", v)
					}
				}
			}
			if nA, nB := kindCount(ra, "execution"), kindCount(rb, "execution"); withDuration != nA || replayedNull != nB || nA != nB {
				t.Errorf("duration_ms exception: %d of %d captured executions carry a positive duration, %d of %d replayed are null", withDuration, nA, replayedNull, nB)
			}
			t.Logf("%s: %d records in each store (%d declarations, %d executions, %d coverage, %d checkpoints, %d steer), %d outputs, equal after the two-item normalisation",
				version, len(ra), kindCount(ra, "declaration"), kindCount(ra, "execution"), kindCount(ra, "coverage"), kindCount(ra, "verify"), kindCount(ra, "steer"), len(a.outputs))
		})
	}
}

// recordsOf returns a world's records of one type in file order, for the one
// session the fixtures hold.
func recordsOf(recs []storedRecord, file, typ string) []map[string]any {
	var out []map[string]any
	for _, r := range recs {
		if strings.HasSuffix(r.File, "/"+file) && r.V["type"] == typ {
			out = append(out, r.V)
		}
	}
	return out
}

func byTool(recs []map[string]any, id string) map[string]any {
	for _, r := range recs {
		if r["tool_use_id"] == id {
			return r
		}
	}
	return nil
}

// TestREPLAY12_CwdThroughTheRecorderResolvesAsLive holds the 2.1.312 session's
// `cd sub && python3 -m unittest` to what the revision-6 table says of cwd:
// a tool_use line names the directory the call was made in (wd) and its
// tool_result line the directory the shell ended in (wd/sub), and the
// recorder wants the first for the declaration and the second for the
// execution's side. Through the live entry points, in a world of its own:
//
//   - the declaration and the execution of toolu_fixture03 are both in the
//     store under one tool_use_id, the execution after the declaration with
//     only the call's own terminal between; the declaration's cwd is wd; the
//     post's coverage record (the execution's cwd) says wd/sub, and the
//     call's says wd;
//   - the declaration's location digest folds the `cd sub`, so it is the
//     digest of wd/sub, which is not the location of the Writes before it and
//     is the location of the two `python3 -m unittest` calls after it, which
//     the payload says ran in wd/sub already;
//   - verify names one gate family for all three runs, the same at the point
//     after the first run (last run toolu_fixture03) and at the Stop (last run
//     toolu_fixture07), the same family as recording the captured payloads
//     gives, and the same location digest in it;
//   - the steer line the second run raises names the first run as its
//     earlier evidence.
func TestREPLAY12_CwdThroughTheRecorderResolvesAsLive(t *testing.T) {
	f := loadFixtureSession(t, "2.1.312")
	steps, ids := f.liveSteps(t)
	a := runLive(t, steps, ids)
	b, res := runRebuilt(t, f.openTranscript(t), f.opt)
	if len(res.Skipped) != 0 || len(res.Stops) != 1 || !res.Stops[0].Evaluated {
		t.Fatalf("skipped %v, stops %+v", res.Skipped, res.Stops)
	}
	ra, rb := dumpStore(t, a.root), dumpStore(t, b.root)

	const session = "00000000-0000-4000-8000-000000000001"
	wd := "$TMPDIR/rashomon-fixture/wd"
	sub := wd + "/sub"
	for name, recs := range map[string][]storedRecord{"A (captured payloads)": ra, "B (rebuilt)": rb} {
		decls, execs := recordsOf(recs, "records.ndjson", "declaration"), recordsOf(recs, "records.ndjson", "execution")
		d3, e3 := byTool(decls, "toolu_fixture03"), byTool(execs, "toolu_fixture03")
		if d3 == nil || e3 == nil {
			t.Fatalf("%s: toolu_fixture03 has declaration %v and execution %v", name, d3 != nil, e3 != nil)
		}
		if d3["cwd"] != wd {
			t.Errorf("%s: the declaration of the cd call has cwd %v, the tool_use line's %s", name, d3["cwd"], wd)
		}
		if d3["session_id"] != session || e3["session_id"] != session {
			t.Errorf("%s: the pair is filed under %v and %v", name, d3["session_id"], e3["session_id"])
		}
		// Between the declaration and the execution is the call's own
		// terminal and nothing else.
		ds, es := d3["seq"].(float64), e3["seq"].(float64)
		for _, r := range recs {
			if sq, ok := r.V["seq"].(float64); ok && sq > ds && sq < es && r.V["tool_use_id"] != "toolu_fixture03" {
				t.Errorf("%s: %v of %v (seq %v) sits between the cd call's declaration (%v) and execution (%v)", name, r.V["type"], r.V["tool_use_id"], sq, ds, es)
			}
		}
		if es <= ds {
			t.Errorf("%s: the execution (seq %v) is not after the declaration (seq %v)", name, es, ds)
		}
		// Every other call's execution follows its own declaration.
		for _, e := range execs {
			d := byTool(decls, e["tool_use_id"].(string))
			if d == nil || d["seq"].(float64) >= e["seq"].(float64) {
				t.Errorf("%s: execution of %v has no earlier declaration", name, e["tool_use_id"])
			}
		}
		// The coverage records of that pair: call at wd, post at wd/sub.
		cov := recordsOf(recs, "coverage.ndjson", "coverage")
		if len(cov) < 7 || cov[5]["phase"] != "call" || cov[5]["cwd"] != wd || cov[6]["phase"] != "post" || cov[6]["cwd"] != sub {
			t.Errorf("%s: the coverage records of the cd call are %s and %s, want a call at %s and a post at %s", name, brief(cov[5]), brief(cov[6]), wd, sub)
		}
		// The location digest.
		loc := func(id string) any { return byTool(decls, id)["cwd_digest"] }
		if loc("toolu_fixture03") != loc("toolu_fixture05") || loc("toolu_fixture05") != loc("toolu_fixture07") {
			t.Errorf("%s: the three unittest runs are at locations %v, %v, %v", name, loc("toolu_fixture03"), loc("toolu_fixture05"), loc("toolu_fixture07"))
		}
		if loc("toolu_fixture03") == loc("toolu_fixture01") {
			t.Errorf("%s: the cd call's location is the one the Writes were made in: the cd was not folded", name)
		}
		t.Logf("%s: unittest location digest %v, Write location digest %v", name, loc("toolu_fixture03"), loc("toolu_fixture01"))
	}
	if diffs := storeDiffs(ra, rb, 5); len(diffs) > 0 {
		t.Errorf("the declarations, executions and coverage differ from live:\n  %s", strings.Join(diffs, "\n  "))
	}

	// verify's own view of the family.
	var fams [2]map[string]map[string]string // run -> point -> family -> last run
	for i, w := range []*replayWorld{a.world, b.world} {
		run := runOf(t, w, session)
		var exec3 int64
		for _, e := range run.Executions {
			if e.ToolUseID == "toolu_fixture03" {
				exec3 = *e.Seq
			}
		}
		fams[i] = map[string]map[string]string{
			"after the first run": gateFamilies(run, session, exec3, f.tl.Events[0].At),
			"at the Stop":         gateFamilies(run, session, run.LastSeq(), f.tl.Events[len(f.tl.Events)-1].At),
		}
	}
	if !reflect.DeepEqual(fams[0], fams[1]) {
		t.Errorf("verify names different families live and replayed:\n  A %v\n  B %v", fams[0], fams[1])
	}
	first, stop := fams[1]["after the first run"], fams[1]["at the Stop"]
	if len(first) != 1 || len(stop) != 1 {
		t.Fatalf("expected one gate family at each point, got %v and %v", first, stop)
	}
	for fam, last := range first {
		if last != "toolu_fixture03" {
			t.Errorf("after the first run the family's last run is %s", last)
		}
		if stop[fam] != "toolu_fixture07" {
			t.Errorf("the family after the first run, %s, is not the family at the Stop (%v): the later runs resolved to another location", fam, stop)
		}
		decls := recordsOf(rb, "records.ndjson", "declaration")
		if want := "@" + byTool(decls, "toolu_fixture03")["cwd_digest"].(string); !strings.HasSuffix(fam, want) {
			t.Errorf("the family %s does not end in the cd call's location digest %s", fam, want)
		}
		t.Logf("gate family (verify GateState.Family), live and replayed: %s", fam)
	}
	// The Stop's own verdict carries the same family.
	if len(a.verdicts) != 1 || len(b.verdicts) != 1 || len(b.verdicts[0].Snap.Gates) != 1 ||
		a.verdicts[0].Snap.Gates[0].Family != b.verdicts[0].Snap.Gates[0].Family {
		t.Errorf("the Stop's gates differ: A %v, B %v", a.verdicts, b.verdicts)
	}
	// The steer line names the first run as evidence of the regression.
	steer := recordsOf(rb, "steer.ndjson", "steer")
	if len(steer) != 1 {
		t.Fatalf("%d steer lines", len(steer))
	}
	decls := recordsOf(rb, "records.ndjson", "declaration")
	ev, _ := steer[0]["evidence_seqs"].([]any)
	if len(ev) != 2 || ev[0] != byTool(decls, "toolu_fixture03")["seq"] || ev[1] != byTool(decls, "toolu_fixture05")["seq"] {
		t.Errorf("the regression note's evidence is %v: it does not tie the failed run (declaration of toolu_fixture05) to the cd call's (toolu_fixture03)", ev)
	}
}
