package acceptance

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
)

// Mirrors internal/fault: the sync recorder's variable; the full-sync point,
// whose first failure is a deferred hook's flush and whose second is the
// flush_failed marker's own sync; and the barrier point, whose first failure
// is the barrier on the seq counter of the entry's first ordered append.
const (
	syncLogEnv         = "RASHOMON_SYNC_LOG"
	pointStoreFullSync = "store.full_sync"
	pointStoreBarrier  = "store.barrier"
)

// syncEvent is one line of the sync recorder's log: a barrier or a full sync,
// the file it ran on, and whether this process held a store lock at the time.
type syncEvent struct {
	kind   string
	file   string
	locked bool
}

// readSyncLog parses the recorder's log. A missing log is no syncs at all.
func readSyncLog(t *testing.T, path string) []syncEvent {
	t.Helper()
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []syncEvent
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		parts := strings.Fields(sc.Text())
		if len(parts) != 3 || (parts[0] != "barrier" && parts[0] != "full" && parts[0] != "barrier-full") || !strings.HasPrefix(parts[2], "locked=") {
			t.Fatalf("sync log line %q is not <kind> <file> locked=<bool>", sc.Text())
		}
		out = append(out, syncEvent{kind: parts[0], file: parts[1], locked: parts[2] == "locked=true"})
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// runFiles counts the lines of the three files a hook appends to.
func (e *env) runFiles(sessionID string) (records, coverage, spill int) {
	e.t.Helper()
	return len(e.read(sessionID, "records.ndjson")), len(e.read(sessionID, "coverage.ndjson")), len(e.read(sessionID, "spill.ndjson"))
}

// TestREC34_GroupCommitFlushesBeforeExit: hook and post order every append
// with a barrier inside its lock and then issue exactly one full sync, after
// the last append, outside every lock, with nothing on stdout, which is what
// "durable before exit" is when nothing printed comes first. Off darwin every
// barrier is itself a full sync, so the append is durable when it returns and
// there is no flush left to issue: no full sync at all (P09-F9). probe and
// the paused path are the contrast: they never defer, and keep a full sync
// per append. Observed through the rashomonfault sync recorder (REC-34).
//
// "Before stdout" holds vacuously on this base: neither hook nor post writes
// to stdout at all (cmdHook and cmdPost are not even handed it), so the
// assertion here is that stdout stays empty. The STEER output PR, which makes
// post print, has to prove the order again against a process that prints.
func TestREC34_GroupCommitFlushesBeforeExit(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	for _, c := range []struct {
		name     string
		entry    func(string, ...string) result
		payload  string
		records  int
		coverage int
	}{
		{"hook", e.hook, defaultPayload().build(t), 2, 1},
		{"post", e.post, defaultPost().build(t), 1, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			log := filepath.Join(t.TempDir(), "sync.log")
			r0, c0, s0 := e.runFiles(testSession)
			res := c.entry(c.payload, syncLogEnv+"="+log)
			if res.exitCode != 0 || res.stdout != "" || res.stderr != "" {
				t.Fatalf("%s: exit %d, stdout %q, stderr %q", c.name, res.exitCode, res.stdout, res.stderr)
			}
			r1, c1, s1 := e.runFiles(testSession)
			if r1-r0 != c.records || c1-c0 != c.coverage || s1 != s0 {
				t.Fatalf("%s appended %d records, %d coverage and %d spill lines; want %d, %d and 0",
					c.name, r1-r0, c1-c0, s1-s0, c.records, c.coverage)
			}

			events := readSyncLog(t, log)
			// Each ordered append syncs its seq counter and its record; each
			// line append syncs its line. On darwin that ordering sync is
			// the kernel's own cheap barrier, kind "barrier". Off darwin
			// there is no barrier to ask for, so the stand-in is a full
			// sync that still runs inside the append's lock -- kind
			// "barrier-full", never counted as a plain "barrier", because
			// it costs what a full sync costs and a reader of the log must
			// be able to tell the two apart (P09-F9).
			wantOrderingSyncs := 2*c.records + c.coverage
			var barriers, barrierFulls, fulls int
			for i, ev := range events {
				switch ev.kind {
				case "barrier":
					barriers++
					if !ev.locked {
						t.Errorf("event %d: a barrier on %s outside its append lock", i, ev.file)
					}
				case "barrier-full":
					barrierFulls++
					if !ev.locked {
						t.Errorf("event %d: a barrier-full on %s outside its append lock", i, ev.file)
					}
				case "full":
					fulls++
					if ev.locked {
						t.Errorf("event %d: the full sync on %s ran inside a store lock", i, ev.file)
					}
					if i != len(events)-1 {
						t.Errorf("event %d of %d: the full sync is not after the last append", i, len(events))
					}
				}
			}
			if fulls != flushesPerProcess() {
				t.Errorf("%d full syncs, want exactly %d: %v", fulls, flushesPerProcess(), events)
			}
			wantBarriers, wantBarrierFulls := wantOrderingSyncs, 0
			if flushesPerProcess() == 0 {
				wantBarriers, wantBarrierFulls = 0, wantOrderingSyncs
			}
			if barriers != wantBarriers {
				t.Errorf("%d barriers, want %d, one per sync of every append: %v", barriers, wantBarriers, events)
			}
			if barrierFulls != wantBarrierFulls {
				t.Errorf("%d barrier-full syncs, want %d: %v", barrierFulls, wantBarrierFulls, events)
			}
		})
	}

	// The contrast: probe and the paused path never defer durability.
	keepsAFullSyncPerAppend := func(t *testing.T, what string, res result, log string) {
		t.Helper()
		if res.exitCode != 0 || res.stdout != "" {
			t.Fatalf("%s: exit %d, stdout %q", what, res.exitCode, res.stdout)
		}
		events := readSyncLog(t, log)
		if len(events) == 0 {
			t.Fatalf("%s synced nothing: the recorder or the append did not run", what)
		}
		for i, ev := range events {
			if ev.kind != "full" {
				t.Errorf("%s: event %d is a %s on %s; this path keeps a full sync per append", what, i, ev.kind, ev.file)
			}
		}
	}
	t.Run("probe end", func(t *testing.T) {
		log := filepath.Join(t.TempDir(), "sync.log")
		keepsAFullSyncPerAppend(t, "probe end", e.probe("end", testSession, syncLogEnv+"="+log), log)
	})
	t.Run("paused hook", func(t *testing.T) {
		if res := e.pause(); res.exitCode != 0 {
			t.Fatalf("pause: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		defer e.resume()
		log := filepath.Join(t.TempDir(), "sync.log")
		keepsAFullSyncPerAppend(t, "paused hook", e.hook(defaultPayload().build(t), syncLogEnv+"="+log), log)
	})
}

// flushesPerProcess is how many full syncs a deferred hook or post issues
// after its appends: the one flush on darwin, where a barrier only orders;
// none elsewhere, where every barrier was a full sync and left the flush
// nothing (P09-F9).
func flushesPerProcess() int {
	if runtime.GOOS == "darwin" {
		return 1
	}
	return 0
}

// skipWithoutAFlush skips a test of the flush's own failure off darwin, where
// a deferred hook issues no flush to fail. The failure that remains there is
// the barrier's, which TestREC34_FailedBarrierIsMarkedNotRetried covers.
func skipWithoutAFlush(t *testing.T) {
	t.Helper()
	if flushesPerProcess() == 0 {
		t.Skip("off darwin every barrier is a full sync and a deferred hook issues no flush (P09-F9); the barrier's failure is TestREC34_FailedBarrierIsMarkedNotRetried")
	}
}

// TestREC34_FailedFlushIsMarkedAndNeverRetried: when the one full sync fails,
// the entry still exits 0 and prints nothing, never retries the flush, and
// leaves a spill terminal for the call carrying flush_failed, synced in full:
// behind a barrier alone the marker could be lost with the records it speaks
// for. The report then renders the session as possibly not durable rather
// than as fine (REC-34).
func TestREC34_FailedFlushIsMarkedAndNeverRetried(t *testing.T) {
	skipWithoutAFlush(t)
	for _, c := range []struct {
		name      string
		entry     func(*env) func(string, ...string) result
		payload   func(*testing.T) string
		wantPhase string
	}{
		{"hook", func(e *env) func(string, ...string) result { return e.hook }, func(t *testing.T) string { return defaultPayload().build(t) }, "call"},
		{"post", func(e *env) func(string, ...string) result { return e.post }, func(t *testing.T) string { return defaultPost().build(t) }, "post"},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)
			log := filepath.Join(t.TempDir(), "sync.log")
			res := c.entry(e)(c.payload(t), syncLogEnv+"="+log, "RASHOMON_FAULT="+pointStoreFullSync+":fail=1")
			if res.exitCode != 0 || res.stdout != "" || res.stderr != "" {
				t.Fatalf("a failed flush: exit %d, stdout %q, stderr %q; want 0 and nothing printed", res.exitCode, res.stdout, res.stderr)
			}

			spill := e.read(testSession, "spill.ndjson")
			if len(spill) != 1 {
				t.Fatalf("spill holds %d lines, want the one flush_failed terminal", len(spill))
			}
			m := spill[0]
			if m.typ() != "terminal" || m.str("reason") != "flush_failed" || m.str("outcome") != "error" ||
				m.str("tool_use_id") != testToolUseID || m.fields["seq"] != nil {
				t.Errorf("the marker is %s; want a terminal for %s, outcome error, reason flush_failed, seq null", m.raw, testToolUseID)
			}
			// The marker carries its own version, store.FlushFailedSchemaVersion
			// (5), never store.SchemaVersion (4, what an ordinary terminal for
			// this same call would carry): a 1.1.x reader must skip this one
			// line and nothing else this invocation wrote (schema5_test.go).
			if got := m.fields["schema_version"]; got != float64(5) {
				t.Errorf("the marker's schema_version is %v, want 5", got)
			}
			if got := m.str("phase"); got != c.wantPhase {
				t.Errorf("the marker's phase is %q, want %q: a reader cannot tell which invocation's records may not be durable without it", got, c.wantPhase)
			}

			// The flush, which failed, and after it nothing but the marker's
			// own append, synced in full: nothing retried the flush, and the
			// marker is durable by the time the entry exits. Its sync runs
			// inside the spill lock, as an undeferred append's does; the spill
			// lock's other takers are best-effort with a 50 ms budget.
			events := readSyncLog(t, log)
			flush := -1
			for i, ev := range events {
				if ev.kind == "full" {
					flush = i
					break
				}
			}
			if flush < 0 || events[flush].locked || events[flush].file == "spill.ndjson" {
				t.Fatalf("no flush outside the locks before the marker: %v", events)
			}
			if after := events[flush+1:]; len(after) != 1 || after[0].kind != "full" || after[0].file != "spill.ndjson" {
				t.Errorf("after the failed flush: %v; want only the marker's full sync on spill.ndjson", after)
			}

			if res := e.probe("end", testSession); res.exitCode != 0 {
				t.Fatalf("probe end: exit %d", res.exitCode)
			}
			sess := e.report(testSession)
			if sess.Coverage.State != "unverified" || !e.hasReason(sess, "flush_failed") {
				t.Errorf("report coverage %s %v; want unverified with flush_failed", sess.Coverage.State, sess.Coverage.Reasons)
			}
			if got := sess.Declarations.NotDurable; len(got) != 1 || got[0] != testToolUseID {
				t.Errorf("declarations.not_durable = %q; want the marked call, %s", got, testToolUseID)
			}
			if len(sess.Declarations.Unterminated) != 0 || len(sess.Declarations.Dropped) != 0 {
				t.Errorf("the marker changed the call's ending: unterminated %v, dropped %v",
					sess.Declarations.Unterminated, sess.Declarations.Dropped)
			}
			text := e.run("", nil, "report", "--session", testSession)
			if text.exitCode != 0 || !strings.Contains(text.stdout, "flush_failed") || !strings.Contains(text.stdout, "possibly not durable: "+testToolUseID) {
				t.Errorf("the text report does not say the records are possibly not durable:\n%s", text.stdout)
			}
		})
	}
}

// TestREC34_UnmarkableFlushFailureSaysSoOnStderr: when the flush fails and
// the marker cannot be made durable either -- not written, or written and its
// full sync failed -- no record is left to hold the failure. The entry then
// says so on stderr, in fixed words like the line for a store it cannot
// open, and still exits 0 with nothing on stdout (REC-34, REC-28). A marker
// whose sync failed is unmarked, whatever its line says when read back now.
func TestREC34_UnmarkableFlushFailureSaysSoOnStderr(t *testing.T) {
	const flushLost = "rashomon: records may not be durable\n"
	t.Run("the marker cannot be written", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		// A directory where the spill file goes: the marker's append cannot open it.
		if err := os.Mkdir(filepath.Join(e.home, "runs", testSession, "spill.ndjson"), 0o700); err != nil {
			t.Fatal(err)
		}
		// The flush fails where there is one, and the barrier, which is
		// as final, where there is not: either way the marker is owed.
		point := pointStoreFullSync
		if flushesPerProcess() == 0 {
			point = pointStoreBarrier
		}
		res := e.hook(defaultPayload().build(t), "RASHOMON_FAULT="+point+":fail=1")
		if res.exitCode != 0 || res.stdout != "" || res.stderr != flushLost {
			t.Fatalf("exit %d, stdout %q, stderr %q; want 0, nothing on stdout, and the fixed line on stderr",
				res.exitCode, res.stdout, res.stderr)
		}
	})
	t.Run("the marker's full sync fails", func(t *testing.T) {
		skipWithoutAFlush(t)
		e := newEnv(t)
		e.watched(testSession)
		log := filepath.Join(t.TempDir(), "sync.log")
		res := e.hook(defaultPayload().build(t), syncLogEnv+"="+log, "RASHOMON_FAULT="+pointStoreFullSync+":fail=2")
		if res.exitCode != 0 || res.stdout != "" || res.stderr != flushLost {
			t.Errorf("exit %d, stdout %q, stderr %q; want 0, nothing on stdout, and the fixed line on stderr",
				res.exitCode, res.stdout, res.stderr)
		}
		var fulls []syncEvent
		for _, ev := range readSyncLog(t, log) {
			if ev.kind == "full" {
				fulls = append(fulls, ev)
			}
		}
		if len(fulls) != 2 || fulls[1].file != "spill.ndjson" {
			t.Errorf("full syncs %v; want the flush and then the marker's, on spill.ndjson", fulls)
		}
	})
}

// TestREC34_FailedBarrierIsMarkedNotRetried: a barrier that fails is a sync
// that failed, and is final the way a failed flush is. The entry exits 0 and
// prints nothing; its one Flush reports the barrier's failure instead of
// full-syncing the same files and calling them durable (on Linux the barrier
// is fsync, so that would be the retried fsync REC-34 rules out); every sync
// after the failure is a full one, the deferral having ended; and the call is
// marked flush_failed by a spill terminal synced in full (REC-34).
func TestREC34_FailedBarrierIsMarkedNotRetried(t *testing.T) {
	for _, c := range []struct {
		name    string
		entry   func(*env) func(string, ...string) result
		payload func(*testing.T) string
	}{
		{"hook", func(e *env) func(string, ...string) result { return e.hook }, func(t *testing.T) string { return defaultPayload().build(t) }},
		{"post", func(e *env) func(string, ...string) result { return e.post }, func(t *testing.T) string { return defaultPost().build(t) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)
			log := filepath.Join(t.TempDir(), "sync.log")
			res := c.entry(e)(c.payload(t), syncLogEnv+"="+log, "RASHOMON_FAULT="+pointStoreBarrier+":fail=1")
			if res.exitCode != 0 || res.stdout != "" || res.stderr != "" {
				t.Fatalf("a failed barrier: exit %d, stdout %q, stderr %q; want 0 and nothing printed", res.exitCode, res.stdout, res.stderr)
			}

			spill := e.read(testSession, "spill.ndjson")
			if len(spill) != 1 || spill[0].typ() != "terminal" || spill[0].str("reason") != "flush_failed" ||
				spill[0].str("tool_use_id") != testToolUseID {
				t.Fatalf("spill holds %d lines (%v); want the one flush_failed terminal for %s", len(spill), spill, testToolUseID)
			}

			// The failed barrier, then nothing but full syncs, each inside
			// its own append lock, the marker's last: no barrier relies on a
			// flush that can no longer succeed, and no flush ran at all.
			// The first event is the ordering sync for the marked append:
			// "barrier" on darwin, or off darwin "barrier-full", itself a
			// full sync standing in for one (P09-F9) -- either way, what
			// the injected fault failed.
			events := readSyncLog(t, log)
			if len(events) < 2 || (events[0].kind != "barrier" && events[0].kind != "barrier-full") {
				t.Fatalf("sync log %v; want the failed barrier first", events)
			}
			for i, ev := range events[1:] {
				if ev.kind != "full" || !ev.locked {
					t.Errorf("event %d after the failed barrier is a %s on %s, locked=%v; want only full syncs inside their append locks: %v",
						i+1, ev.kind, ev.file, ev.locked, events)
				}
			}
			if last := events[len(events)-1]; last.file != "spill.ndjson" {
				t.Errorf("the last sync is on %s; want the marker's, on spill.ndjson: %v", last.file, events)
			}

			if res := e.probe("end", testSession); res.exitCode != 0 {
				t.Fatalf("probe end: exit %d", res.exitCode)
			}
			sess := e.report(testSession)
			if !e.hasReason(sess, "flush_failed") {
				t.Errorf("report coverage %s %v; want flush_failed", sess.Coverage.State, sess.Coverage.Reasons)
			}
			// The failed barrier is the seq counter's, the first ordered
			// append issues: appendOrdered returns before the declaration
			// or execution line is ever written, so there is no record of
			// any kind for testToolUseID beside the marker itself. Naming
			// it in not_durable would call a declaration possibly not
			// durable when there never was one -- the session's own
			// flush_failed reason, asserted above, is what carries this
			// failure.
			if got := sess.Declarations.NotDurable; len(got) != 0 {
				t.Errorf("declarations.not_durable = %q, want none: %s was never declared or executed", got, testToolUseID)
			}
		})
	}
}

// TestREC34_SignalBeforeFlushReachesTheSyncLog: Close has already appended
// the terminal and coverage records by the time hook.before_flush fires, so
// what a signal arriving there changes is only whether Flush itself still
// runs. SIGTERM is caught (hook.WatchSignals): the hang returns and Flush
// proceeds, landing the one full sync store.go promises, last and outside
// every lock. SIGKILL ends the process first: every append's barrier is
// already on disk, but no full sync runs and nothing writes a flush_failed
// marker, because nothing observed a failure to mark (REC-34).
func TestREC34_SignalBeforeFlushReachesTheSyncLog(t *testing.T) {
	skipWithoutAFlush(t)

	// hangingHook starts a hook held at hook.before_flush and returns only
	// once it is positively there: the fault point has created its marker,
	// which it does after catching SIGTERM and before waiting, and the process
	// has not exited. Close's records landing (the coverage line) is not that
	// evidence. It is what a hook with no injection point writes too, and that
	// hook goes straight on to Flush, so a signal sent on seeing the line can
	// land before Flush or after it, depending on how slow a darwin barrier
	// is. With the marker the signal always lands on a hook that has not
	// started Flush, and a build without the point never writes it, so the
	// test fails there every time instead of passing when it loses the race.
	//
	// exited closes when the process has been reaped; ProcessState is valid
	// once it has.
	hangingHook := func(t *testing.T, e *env, log string) (cmd *exec.Cmd, exited <-chan struct{}) {
		t.Helper()
		marker := filepath.Join(t.TempDir(), "hung")
		cmd = e.command(defaultPayload().build(t), []string{
			// Long enough that the hang's own bound cannot end it first.
			"RASHOMON_FAULT=" + pointHookBeforeFlush + ":hang=60",
			"RASHOMON_FAULT_HANG_MARKER=" + marker,
			syncLogEnv + "=" + log,
		}, "hook")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		go func() {
			// The exit status is read from ProcessState by the caller.
			_ = cmd.Wait()
			close(done)
		}()
		t.Cleanup(func() { _ = cmd.Process.Kill() })
		waitFor(t, 5e9, "the entry to close before flush", func() bool {
			return len(e.coverage(testSession, "call")) == 1
		})
		waitFor(t, 5e9, "the hook to reach the hang at hook.before_flush", func() bool {
			select {
			case <-done:
				t.Fatalf("the hook exited without reaching the hang at %s: the injection point is gone or was skipped", pointHookBeforeFlush)
			default:
			}
			_, err := os.Stat(marker)
			return err == nil
		})
		select {
		case <-done:
			t.Fatal("the hook exited while held at hook.before_flush")
		default:
		}
		return cmd, done
	}

	t.Run("SIGTERM lets Flush run", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		log := filepath.Join(t.TempDir(), "sync.log")
		cmd, exited := hangingHook(t, e, log)
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		// Wait's error is expected whenever the exit code is non-zero; the
		// exit code is read from ProcessState below, which is what matters.
		<-exited
		if code := cmd.ProcessState.ExitCode(); code != 0 {
			t.Fatalf("exit %d after SIGTERM before flush, want 0: Flush still had to run", code)
		}

		events := readSyncLog(t, log)
		if len(events) == 0 {
			t.Fatal("no syncs recorded")
		}
		var fulls int
		for i, ev := range events {
			if ev.kind != "full" {
				continue
			}
			fulls++
			if ev.locked {
				t.Errorf("event %d: the full sync on %s ran inside a store lock", i, ev.file)
			}
			if i != len(events)-1 {
				t.Errorf("event %d of %d: the full sync is not last", i, len(events))
			}
		}
		if fulls != flushesPerProcess() {
			t.Errorf("%d full syncs after SIGTERM before flush, want %d: %v", fulls, flushesPerProcess(), events)
		}
	})

	t.Run("SIGKILL ends it before Flush runs", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		log := filepath.Join(t.TempDir(), "sync.log")
		r0, c0, _ := e.runFiles(testSession)
		cmd, exited := hangingHook(t, e, log)
		if err := cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		// Wait's error is expected: SIGKILL makes this exit by signal, and
		// nothing below reads the exit code.
		<-exited

		r1, c1, s1 := e.runFiles(testSession)
		if r1-r0 != 2 || c1-c0 != 1 {
			t.Fatalf("SIGKILL before flush: %d records, %d coverage appended; want 2 and 1, since Close had already written them", r1-r0, c1-c0)
		}
		if s1 != 0 {
			t.Errorf("SIGKILL before flush left %d spill line(s); want 0, nothing ran to mark a failure", s1)
		}
		for i, ev := range readSyncLog(t, log) {
			if ev.kind == "full" {
				t.Errorf("event %d: a full sync was recorded even though SIGKILL arrived before Flush ran: %v", i, ev)
			}
		}
	})
}

// TestREC34_ForgetSyncsInsideItsCountedLocks: the recorder's locked= flag
// counts every lock the store takes, not only the append locks, and sees
// every sync. forget rewrites the ordered stream under its lock, so the
// rewrite's full sync on records.ndjson is logged, and logged as inside a
// lock, as is every other sync forget issues (REC-34, P09-F8).
func TestREC34_ForgetSyncsInsideItsCountedLocks(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	log := filepath.Join(t.TempDir(), "sync.log")
	if res := e.run("", []string{syncLogEnv + "=" + log}, "forget", "--since", "1h"); res.exitCode != 0 {
		t.Fatalf("forget: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	events := readSyncLog(t, log)
	rewrote := false
	for i, ev := range events {
		if !ev.locked {
			t.Errorf("event %d: forget's %s sync on %s ran outside every lock the recorder counted: %v", i, ev.kind, ev.file, events)
		}
		if ev.kind == "full" && ev.file == "records.ndjson" {
			rewrote = true
		}
	}
	if !rewrote {
		t.Errorf("no full sync of records.ndjson in %v: forget's rewrite is invisible to the recorder", events)
	}
}
