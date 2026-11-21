package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// catchID is the session the catch_turn fixture writes.
const catchID = "9c3e5a71-2b4d-4f60-8e19-7a0c5d3b2f48"

// catchProject is the fixture's project and cwd; every path it writes is
// under it, so every write is in Touched.
const catchProject = "/Users/dev/src/project"

// turnCalls is a turn's length in the fixture: one prompt every 50 calls, as
// BenchmarkRoute/count_6000's steer log holds 120 prompts of 50 lines.
const turnCalls = 50

// turnEvents is the events one turn adds: a declaration, a terminal and an
// execution per call, and the turn's steer line.
const turnEvents = 3*turnCalls + 1

// writeCalls appends calls [from, from+calls) of the fixture to catchID's two
// run files, straight to the files as the store's own writers marshal each
// record, since through the append API each record costs two fsyncs. Call i
// is records 3i+1 to 3i+3: an Edit's declaration, its terminal and its
// execution. A call that opens a turn is preceded by the turn's delivered
// steer line, after the last record before it, naming the turn's prompt and a
// key of its own.
//
// It is the high side of what calls can add to a frontier: every call writes
// a file no earlier call wrote, so Touched holds one id per call, where the
// 42 cut session fixtures' frontiers hold 334 Touched ids through a summed
// seq of 12,919; and
// each turn adds an intent and a delivered key. Each file id is 38 bytes
// (the fixtures' mean is 36.5) when dir is empty. dir goes between a file's
// package and its name, ending in a slash: the benchmark passes "", and a
// long dir takes the frontier past its limit with no change to the limit
// (TestCompose_PastTheLimitIsASoftMiss).
func writeCalls(tb testing.TB, st *store.Store, from, calls int, dir string) {
	tb.Helper()
	var recs, steer bytes.Buffer
	line := func(buf *bytes.Buffer, v any) {
		data, err := json.Marshal(v)
		if err != nil {
			tb.Fatal(err)
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	project := catchProject
	for i := from; i < from+calls; i++ {
		seq, at := int64(3*i), int64(1793000000000+2000*i)
		prompt := fmt.Sprintf("72fea482-07bb-4309-8de3-%012d", i/turnCalls+1)
		if i%turnCalls == 0 {
			turn := i/turnCalls + 1
			fmt.Fprintf(&steer, `{"type":"steer","v":1,"id":"%08x","at_unix_ms":%d,"session_id":%q,"prompt_id":%q,"kind":"unit","key":"example.com/m/internal/package%02d@%08x","after_seq":%d,"model_text":"some words","user_text":"more words","state":"delivered"}`+"\n",
				turn, at-500, catchID, prompt, turn%50, turn, seq)
		}
		id := fmt.Sprintf("toolu_01%022d", i)
		path := fmt.Sprintf("%s/internal/package%02d/%ssource%05d.go", catchProject, i%50, dir, i)
		d := store.Declaration{Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion, Seq: seq + 1,
			RecordedAtMS: at, ToolUseID: id, SessionID: catchID, PromptID: &prompt, ToolName: "Edit"}
		d.FilePath, d.Project, d.CWD = &path, &project, &project
		line(&recs, d)
		term, exec, duration := seq+2, seq+3, int64(1200)
		line(&recs, store.Terminal{Type: store.TypeTerminal, SchemaVersion: store.SchemaVersion, Seq: &term,
			RecordedAtMS: at + 5, ToolUseID: id, SessionID: catchID, Outcome: store.OutcomeOK})
		line(&recs, store.Execution{Type: store.TypeExecution, SchemaVersion: store.SchemaVersion, Seq: &exec,
			RecordedAtMS: at + duration, ToolUseID: id, SessionID: catchID, ToolName: "Edit",
			Outcome: store.ExecOK, DurationMS: &duration})
	}
	runDir := st.RunDir(catchID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		tb.Fatal(err)
	}
	for _, f := range []struct {
		name string
		data []byte
	}{{store.FileRecords, recs.Bytes()}, {store.FileSteer, steer.Bytes()}} {
		w, err := os.OpenFile(filepath.Join(runDir, f.name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			tb.Fatal(err)
		}
		_, werr := w.Write(f.data)
		if cerr := w.Close(); werr != nil || cerr != nil {
			tb.Fatal(werr, cerr)
		}
	}
}

// catchTurnStore is a store at root whose run catchID holds calls calls with
// the frontier saved through them, then one more turn written after the save:
// the turn a prompt's catch-up reads. It checks the work: the save holds one
// Touched id per call and one intent per turn, and a catch-up continues it
// by the turn's events.
func catchTurnStore(tb testing.TB, root string, calls int) (*store.Store, int) {
	tb.Helper()
	st, err := store.Open(root)
	if err != nil {
		tb.Fatal(err)
	}
	writeCalls(tb, st, 0, calls, "")
	f, saved, err := Compose(st, catchID)
	if err != nil || !saved {
		tb.Fatalf("the first compose: saved %v, err %v", saved, err)
	}
	if len(f.Touched) != calls || len(f.Intents) != calls/turnCalls {
		tb.Fatalf("the saved frontier holds %d touched and %d intents; want %d and %d", len(f.Touched), len(f.Intents), calls, calls/turnCalls)
	}
	data, ok, err := st.LoadFrontier(catchID)
	if err != nil || !ok {
		tb.Fatalf("no frontier file: ok %v, err %v", ok, err)
	}
	writeCalls(tb, st, calls, turnCalls, "")
	catchTurn(tb, st)
	return st, len(data)
}

// catchTurn is what a prompt pays for the session's frontier, since the save
// is made only at Stop and at session boundaries: one hold of the records
// lock, the frontier file loaded and continued by the lines written since it
// was saved (catchUp), and nothing written -- a nil from the fold saves
// nothing (store.ComposeFrontier). A miss, or a tail other than one turn's,
// fails: a benchmark of the whole fold would pass under this one's name.
func catchTurn(tb testing.TB, st *store.Store) {
	tail := -1
	if _, err := st.ComposeFrontier(catchID, func(b store.Bounds, ok bool) ([]byte, error) {
		if !ok {
			return nil, fmt.Errorf("no hold of the records lock")
		}
		if _, r, hit := catchUp(st, catchID, b); hit {
			tail = len(r.evs)
		}
		return nil, nil
	}); err != nil {
		tb.Fatal(err)
	}
	if tail != turnEvents {
		tb.Fatalf("the catch-up read %d events; want a hit on one turn's %d", tail, turnEvents)
	}
}

// BenchmarkCatchTurn is catchTurn on a session of 100 calls and of 6,000,
// each continued by its next turn. A size's store is built on its first run,
// so a run that selects one size builds that one alone; its directory is the
// outer benchmark's, since a sub-benchmark's cleanups run after each of its
// runs and the store has to outlive the first.
func BenchmarkCatchTurn(b *testing.B) {
	// A table of literal names: RES36 reads every b.Run name statically.
	for _, c := range []struct {
		name  string
		calls int
	}{{"run_100", 100}, {"run_6000", 6000}} {
		root := b.TempDir()
		var st *store.Store
		b.Run(c.name, func(b *testing.B) {
			if st == nil {
				st, _ = catchTurnStore(b, root, c.calls)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				catchTurn(b, st)
			}
		})
	}
}

// TestCatchTurn_HighSideFrontierUnderTheLimit: the fixture's frontier file,
// the high side of what a session's calls add (writeCalls), stays under
// store.MaxFrontierBytes at 6,000 calls, and each file a call writes for the
// first time adds under 64 bytes to it, read as the slope between the
// 100-call and the 6,000-call points. The 64 was set after the measurement
// (45.6 bytes per newly written file); it keeps the file count the limit
// allows past the 100-call point, about 16,300 at 64 bytes, from shrinking
// unseen. Each size is logged beside the limit and a quarter of it, and the
// quarter is not asserted.
func TestCatchTurn_HighSideFrontierUnderTheLimit(t *testing.T) {
	const lo, hi, slopeMax = 100, 6000, 64
	size := map[int]int{}
	for _, calls := range []int{lo, hi} {
		_, size[calls] = catchTurnStore(t, t.TempDir(), calls)
		t.Logf("run_%d: frontier file %d bytes, %.2f%% of the limit of %d, %.2f of a quarter of it",
			calls, size[calls], 100*float64(size[calls])/store.MaxFrontierBytes, store.MaxFrontierBytes,
			float64(size[calls])/(store.MaxFrontierBytes/4))
	}
	slope := float64(size[hi]-size[lo]) / (hi - lo)
	t.Logf("%.2f bytes per newly written file; on that slope the limit allows about %d files past the %d-call point",
		slope, int(float64(store.MaxFrontierBytes-size[lo])/slope), lo)
	if size[hi] > store.MaxFrontierBytes {
		t.Errorf("run_%d: the frontier file is %d bytes, past the limit of %d", hi, size[hi], store.MaxFrontierBytes)
	}
	if slope >= slopeMax {
		t.Errorf("%.2f bytes per newly written file; want under %d", slope, slopeMax)
	}
}
