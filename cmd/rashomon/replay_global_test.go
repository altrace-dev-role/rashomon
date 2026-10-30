package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/replay"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// What leaving SessionEnd out changes, and what a replay leaves behind in the
// process's global state however it ends.

// TestREPLAY24_LeavingSessionEndOutChangesOnlyWhatReadsTheEnd feeds the
// 2.1.312 session three ways: the captured payloads without SessionEnd, with
// it, and the rebuilt timeline (which never has one). The rebuilt records
// equal the captured ones without it (TestREPLAY10_), and the captured ones
// with it differ by exactly one coverage record, the end probe's. Nothing the
// recorder's verdicts read depends on it: the evaluation at the last point, the
// Stop's view and the recording state are equal. What does read it is report
// coverage, which reads a session with no end record as not closed
// (run_not_closed), and the session window the sandbox trail is joined on,
// which is then open at its end. Both are reporting; the seed uses neither, and
// the summary says session_end_unknown.
func TestREPLAY24_LeavingSessionEndOutChangesOnlyWhatReadsTheEnd(t *testing.T) {
	f := loadFixtureSession(t, "2.1.312")
	steps, ids := f.liveSteps(t)
	var end step
	for _, s := range f.captured {
		if s.event == install.EventSessionEnd {
			end = s
		}
	}
	if end.raw == nil {
		t.Fatal("the fixture has no SessionEnd")
	}
	last := f.tl.Events[len(f.tl.Events)-1].At
	withEnd := append(append([]replayStep(nil), steps...), replayStep{end.event, end.raw, last})
	a := runLive(t, steps, ids)
	a2 := runLive(t, withEnd, append(append([]string(nil), ids...), ""))
	b, _ := runRebuilt(t, f.openTranscript(t), f.opt)

	ra, ra2, rb := dumpStore(t, a.root), dumpStore(t, a2.root), dumpStore(t, b.root)
	if d := storeDiffs(ra, rb, 5); len(d) != 0 {
		t.Fatalf("the rebuilt records differ from the captured ones: %v", d)
	}
	var kept []storedRecord
	ends := 0
	for _, r := range ra2 {
		if r.V["type"] == "coverage" && r.V["phase"] == "end" {
			ends++
			continue
		}
		kept = append(kept, r)
	}
	if ends != 1 {
		t.Fatalf("%d end coverage records in the run that has SessionEnd", ends)
	}
	if d := storeDiffs(kept, rb, 5); len(d) != 0 {
		t.Errorf("SessionEnd changed more than its own coverage record: %v", d)
	}

	// The verdicts do not read it.
	const session = "00000000-0000-4000-8000-000000000001"
	ev := func(w *replayWorld) string {
		run := runOf(t, w, session)
		snap := verify.Evaluate(run, nil, nil, verify.Point{Kind: verify.PointQuery, Seq: run.LastSeq(), SessionID: session}, last)
		b, err := json.Marshal(snap.Gates)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%s|%v|%s", snap.Recording, gateFamilies(run, session, run.LastSeq(), last), b)
	}
	if x, y := ev(a2.world), ev(b.world); x != y {
		t.Errorf("the evaluation differs with and without SessionEnd:\n  with    %s\n  without %s", x, y)
	}
	if len(a2.verdicts) != len(b.verdicts) || verdictJSON(a2.verdicts[0]) != verdictJSON(b.verdicts[0]) {
		t.Errorf("the Stop's view differs")
	}

	// What does read it.
	cov := func(w *replayWorld) report.CoverageFacts {
		return report.RollupCoverage(runOf(t, w, session).Coverage)
	}
	if !cov(a2.world).EndRecorded || cov(b.world).EndRecorded {
		t.Errorf("end recorded: with %v, without %v", cov(a2.world).EndRecorded, cov(b.world).EndRecorded)
	}
	if !reflect.DeepEqual(cov(a2.world).Reasons, cov(b.world).Reasons) || cov(a2.world).StartRecorded != cov(b.world).StartRecorded {
		t.Errorf("coverage facts differ beyond the end: %+v and %+v", cov(a2.world), cov(b.world))
	}
	wa, wb := report.SessionWindow(runOf(t, a2.world, session)), report.SessionWindow(runOf(t, b.world, session))
	if wa.End.IsZero() || !wb.End.IsZero() {
		t.Errorf("session window end: with %v, without %v", wa.End, wb.End)
	}
	// The name says what is known of it.
	if !contains(replayFlags, "session_end_unknown") || contains(replayFlags, "probe_end_not_replayed") {
		t.Errorf("flags %v", replayFlags)
	}
}

// globalsState is the process-wide state a replay swaps.
type globalsState struct {
	env     []string
	now     uintptr
	tapNil  bool
	mutexOK bool
}

func snapGlobals() globalsState {
	env := os.Environ()
	sort.Strings(env)
	g := globalsState{env: env, now: reflect.ValueOf(entryNow).Pointer(), tapNil: stopSeen == nil}
	if g.mutexOK = replayMu.TryLock(); g.mutexOK {
		replayMu.Unlock()
	}
	return g
}

func (g globalsState) check(t *testing.T, before globalsState, what string) {
	t.Helper()
	if !g.mutexOK {
		t.Errorf("%s: the replay mutex is still held", what)
	}
	if g.now != before.now {
		t.Errorf("%s: the recorder's clock was not put back", what)
	}
	if !g.tapNil {
		t.Errorf("%s: the Stop tap was not put back", what)
	}
	if !reflect.DeepEqual(g.env, before.env) {
		t.Errorf("%s: the environment was not put back:\n before %v\n after  %v", what, before.env, g.env)
	}
}

// TestREPLAY26_AReplayPutsTheProcessBackWhateverEndsIt: an error half way, a
// panic inside an entry point and a cancelled context each leave the
// environment, the clock, the Stop tap and the replay mutex as they were, and
// the next replay works.
func TestREPLAY26_AReplayPutsTheProcessBackWhateverEndsIt(t *testing.T) {
	f := loadFixtureSession(t, "2.1.312")
	before := snapGlobals()
	if !before.tapNil || !before.mutexOK {
		t.Fatal("the test starts with a replay in progress")
	}

	t.Run("an error half way", func(t *testing.T) {
		w := newWorld(t)
		n := 0
		_, err := w.replayTranscript(f.openTranscript(t), f.opt, replayConfig{Observe: func(replay.Event, bool) error {
			if n++; n == 4 {
				return errors.New("halfway")
			}
			return nil
		}})
		if err == nil || err.Error() != "halfway" || n != 4 {
			t.Fatalf("err %v after %d events", err, n)
		}
		snapGlobals().check(t, before, "after an error")
	})

	t.Run("a panic in an entry point", func(t *testing.T) {
		// The recorder's entry points contain a panic themselves (safe.Guard):
		// a hook that crashed would crash the host it runs in. So the panic does
		// not reach the replay; what the test holds is that the process is as
		// it was at the end, and that the event it hit was handled as a failure
		// and the replay went on.
		w := newWorld(t)
		reads := 0
		w.afterClock = func() {
			if reads++; reads == 5 {
				panic("inside an entry point")
			}
		}
		var got any
		func() {
			defer func() { got = recover() }()
			_, _ = w.replayTranscript(f.openTranscript(t), f.opt, replayConfig{})
		}()
		if reads <= 5 {
			t.Fatalf("the replay stopped at the panic: %d clock reads", reads)
		}
		if got != nil {
			t.Logf("a panic reached the replay: %v", got)
		}
		snapGlobals().check(t, before, "after a panic in an entry point")
	})

	t.Run("a panic that escapes", func(t *testing.T) {
		w := newWorld(t)
		n := 0
		var got any
		func() {
			defer func() { got = recover() }()
			_, _ = w.replayTranscript(f.openTranscript(t), f.opt, replayConfig{Observe: func(replay.Event, bool) error {
				if n++; n == 3 {
					panic("outside an entry point")
				}
				return nil
			}})
		}()
		if got != "outside an entry point" || n != 3 {
			t.Fatalf("recovered %v after %d events", got, n)
		}
		snapGlobals().check(t, before, "after a panic that escaped")
	})

	t.Run("a cancelled context", func(t *testing.T) {
		w := newWorld(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		n := 0
		_, err := w.replayTranscript(f.openTranscript(t), f.opt, replayConfig{Ctx: ctx, Observe: func(replay.Event, bool) error {
			if n++; n == 3 {
				cancel()
			}
			return nil
		}})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err %v", err)
		}
		if n >= len(f.tl.Events) {
			t.Errorf("the replay went on to the end (%d events) after it was cancelled", n)
		}
		snapGlobals().check(t, before, "after a cancel")
	})

	t.Run("the next replay works", func(t *testing.T) {
		a, res := runRebuilt(t, f.openTranscript(t), f.opt)
		if len(res.Skipped) != 0 || len(a.outputs) == 0 {
			t.Errorf("skipped %v", res.Skipped)
		}
		snapGlobals().check(t, before, "after a replay")
	})
}

// TestREPLAY26b_TwoReplaysNeverRunAtOnce starts several at the same moment,
// each in a world of its own, and makes every entry point say which store it
// resolves to. A replay that ran beside another would see the other's
// environment; none does, each ends with the records of a replay alone, and
// the process is as it was. (Run under -race.)
func TestREPLAY26b_TwoReplaysNeverRunAtOnce(t *testing.T) {
	f := loadFixtureSession(t, "2.1.312")
	before := snapGlobals()
	solo, _ := runRebuilt(t, f.openTranscript(t), f.opt)
	want := dumpStore(t, solo.root)

	const n = 6
	var mu sync.Mutex
	var problems []string
	note := func(s string) { mu.Lock(); problems = append(problems, s); mu.Unlock() }
	roots := make([]string, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			w, err := newReplayWorld(t.TempDir(), replayIdentity{})
			if err != nil {
				note(fmt.Sprint("world: ", err))
				return
			}
			roots[i] = w.root
			w.afterClock = func() {
				if got := os.Getenv("RASHOMON_HOME"); got != w.root {
					note(fmt.Sprintf("replay %d: an entry point resolved RASHOMON_HOME to %s, not %s", i, got, w.root))
				}
			}
			fh, err := os.Open(fixtureTranscript("2.1.312"))
			if err != nil {
				note(err.Error())
				return
			}
			defer fh.Close()
			if _, err := w.replayTranscript(fh, f.opt, replayConfig{}); err != nil {
				note(fmt.Sprint("replay: ", err))
			}
		}()
	}
	close(start)
	wg.Wait()
	if len(problems) > 0 {
		t.Fatalf("%s", strings.Join(problems, "\n"))
	}
	for i, r := range roots {
		if d := storeDiffs(want, dumpStore(t, r), 3); len(d) != 0 {
			t.Errorf("replay %d recorded differently than one alone: %v", i, d)
		}
	}
	snapGlobals().check(t, before, "after six replays")
}

// TestREPLAY31_TheImportIsBoundedAndOneSessionAtATime: past the cap on the
// tool_use_id digests a session is partial, with the reason, and the calls past
// it do not count; and two imports started together replay their sessions one
// after the other, each ending as it would alone.
func TestREPLAY31_TheImportIsBoundedAndOneSessionAtATime(t *testing.T) {
	const sid = "77777777-0000-4000-8000-000000000007"
	tr := newTree(t)
	tr.write("projects/-repo/s.jsonl", gateDoc(sid, []string{"ok", "ok", "fail", "ok"}).String())
	im := newTestImporter(t, func(c *importConfig) { c.MaxCallIDs = 2 })
	importAll(t, im, tr)
	s := im.ledger.Get(sid)
	f := s.Families[onlyKey(t, s.Families)]
	if !s.Partial || !contains(s.PartialReasons, string(replay.SessionIDCap)) || s.TaintReason != string(replay.SessionIDCap) {
		t.Errorf("partial %v %v taint %q", s.Partial, s.PartialReasons, s.TaintReason)
	}
	if f.RunsVerified != 2 || f.SessionsVerified != 1 || f.Reds != 0 {
		t.Errorf("counts %+v: only the two calls within the cap count", *f)
	}

	before := snapGlobals()
	var trees []*ftree
	var ims []*importer
	for i := range 3 {
		x := newTree(t)
		x.write("p/a.jsonl", gateDoc(fmt.Sprintf("88888888-0000-4000-8000-00000000000%d", i), []string{"ok", "fail"}, []string{"ok"}).String())
		trees, ims = append(trees, x), append(ims, newTestImporter(t, nil))
	}
	var wg sync.WaitGroup
	errs := make([]error, len(ims))
	for i := range ims {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = ims[i].Import(trees[i].files())
		}()
	}
	wg.Wait()
	for i, im := range ims {
		if errs[i] != nil {
			t.Fatal(errs[i])
		}
		alone := newTestImporter(t, nil)
		importAll(t, alone, trees[i])
		if !reflect.DeepEqual(alone.ledger.Totals(), im.ledger.Totals()) {
			t.Errorf("import %d together differs from alone", i)
		}
	}
	snapGlobals().check(t, before, "after concurrent imports")
}

// TestREPLAY26c_AWorldIsNotMadeWhileAReplayHasTheEnvironment: a world is
// refused if it overlaps the store or configuration the process's environment
// names, and a replay in progress has swapped that environment for its own. A
// world made then would be checked against the other replay's world and not the
// real one. So making a world waits for the replay, and the check is made
// against the caller's own environment: a world aimed at the real store while a
// replay runs is refused all the same.
func TestREPLAY26c_AWorldIsNotMadeWhileAReplayHasTheEnvironment(t *testing.T) {
	home := plantHome(t)
	realRoot := filepath.Join(home, ".local", "state", "rashomon")
	f := loadFixtureSession(t, "2.1.312")
	w := newWorld(t)
	inside, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	w.afterClock = func() {
		once.Do(func() { close(inside); <-release })
	}
	replayed := make(chan error, 1)
	go func() {
		_, err := w.replayTranscript(f.openTranscript(t), f.opt, replayConfig{})
		replayed <- err
	}()
	<-inside // the replay holds the environment, set to its own world

	made := make(chan error, 1)
	go func() {
		_, err := newReplayWorld(filepath.Join(realRoot, "scratch"), replayIdentity{})
		made <- err
	}()
	var early bool
	select {
	case <-made:
		early = true // it did not wait
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-replayed; err != nil {
		t.Fatal(err)
	}
	if early {
		t.Fatal("a world was made while a replay had the environment")
	}
	if err := <-made; err == nil {
		t.Error("a world on the real store was made")
	}
	if _, err := os.Stat(filepath.Join(realRoot, "scratch")); !os.IsNotExist(err) {
		t.Errorf("the refused world left %s behind: %v", filepath.Join(realRoot, "scratch"), err)
	}
}
