package store

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// orderingBarrier stands in darwin's accepted barrier on every platform: it
// reports the write ordered and not yet durable, which leaves it to Flush. The
// tests of Flush's own bookkeeping use it so that they judge the same thing
// off darwin, where the real barrier is a full sync and leaves Flush nothing
// (TestREC34l_).
func orderingBarrier(*os.File) (bool, error) { return false, nil }

// TestREC34b_DeferredAppendsLeaveOneFlushBehind: a deferred Store whose
// barriers only order records which file the group commit's one full sync
// has to cover, and Flush clears it; a Store that never deferred leaves
// nothing for Flush, because each of its appends was already synced in full
// (REC-34).
func TestREC34b_DeferredAppendsLeaveOneFlushBehind(t *testing.T) {
	const session = "sess-deferred"
	t.Run("deferred", func(t *testing.T) {
		st, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		st.barrier = orderingBarrier
		st.DeferDurability()
		if err := st.Flush(); err != nil {
			t.Fatalf("Flush with nothing appended: %v", err)
		}
		if err := st.AppendDeclaration(Declaration{Type: TypeDeclaration, SchemaVersion: SchemaVersion, ToolUseID: "toolu_1", SessionID: session}); err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(st.RunDir(session), FileRecords); st.dirty != want {
			t.Fatalf("after a deferred ordered append the flush would cover %q, want %q", st.dirty, want)
		}
		if err := st.AppendCoverage(Coverage{Type: TypeCoverage, SchemaVersion: SessionSchemaVersion, SessionID: session}); err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(st.RunDir(session), FileCoverage); st.dirty != want {
			t.Fatalf("after a deferred line append the flush would cover %q, want the last file written, %q", st.dirty, want)
		}
		if err := st.Flush(); err != nil {
			t.Fatalf("Flush: %v", err)
		}
		if st.dirty != "" {
			t.Errorf("a successful Flush left %q to flush", st.dirty)
		}
	})
	t.Run("not deferred", func(t *testing.T) {
		st, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := st.AppendDeclaration(Declaration{Type: TypeDeclaration, SchemaVersion: SchemaVersion, ToolUseID: "toolu_1", SessionID: session}); err != nil {
			t.Fatal(err)
		}
		if err := st.AppendCoverage(Coverage{Type: TypeCoverage, SchemaVersion: SessionSchemaVersion, SessionID: session}); err != nil {
			t.Fatal(err)
		}
		if st.dirty != "" {
			t.Errorf("appends that synced in full left %q for a flush", st.dirty)
		}
	})
}

// TestREC34c_AFailedFlushIsNeverRetried: once Flush has failed, this Store
// never reports its records durable again, even when a second attempt would
// succeed. A retried fsync can return success for pages the kernel dropped
// when the first one failed, so the only honest answer after a failure is
// the failure (REC-34).
func TestREC34c_AFailedFlushIsNeverRetried(t *testing.T) {
	const session = "sess-flush-fails"
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st.barrier = orderingBarrier
	st.DeferDurability()
	cov := Coverage{Type: TypeCoverage, SchemaVersion: SessionSchemaVersion, SessionID: session}
	if err := st.AppendCoverage(cov); err != nil {
		t.Fatal(err)
	}
	// The file the flush has to cover is gone, so the flush cannot be made.
	if err := os.Remove(filepath.Join(st.RunDir(session), FileCoverage)); err != nil {
		t.Fatal(err)
	}
	first := st.Flush()
	if first == nil {
		t.Fatal("Flush of a file that no longer exists reported success")
	}
	// No later Flush can succeed, so none can cover a deferred append: the
	// store must sync whatever is appended after this in full, the
	// flush_failed marker first.
	if st.deferred {
		t.Error("a failed Flush left the store deferring: an append after it would be covered by no flush")
	}
	// The file exists again, so a retry would now succeed.
	if err := st.AppendCoverage(cov); err != nil {
		t.Fatal(err)
	}
	if err := st.Flush(); err == nil {
		t.Fatal("a second Flush after a failed one reported success: the failure was retried")
	}
}

// TestREC34d_FlushFailedTerminalClosesNothingAndDropsNothing: the terminal
// that marks a call's records as possibly not durable is not an ending. It
// must not close a declaration whose own terminal never landed, which would
// hide an unterminated entry, and it must not read as a declaration lost to
// the lock (REC-34).
func TestREC34d_FlushFailedTerminalClosesNothingAndDropsNothing(t *testing.T) {
	reason := ReasonFlushFailed
	marker := func(id string) Terminal {
		return Terminal{Type: TypeTerminal, SchemaVersion: SchemaVersion, ToolUseID: id, Outcome: OutcomeError, Reason: &reason}
	}
	run := &Run{
		Declarations: []Declaration{{ToolUseID: "toolu_unterminated"}, {ToolUseID: "toolu_closed"}},
		Terminals: []Terminal{
			marker("toolu_unterminated"),
			{ToolUseID: "toolu_closed", Outcome: OutcomeOK},
			marker("toolu_closed"),
			marker("toolu_no_declaration"),
		},
	}
	if got := run.Unterminated(); len(got) != 1 || got[0] != "toolu_unterminated" {
		t.Errorf("Unterminated() = %v, want [toolu_unterminated]: a flush_failed marker closed a declaration", got)
	}
	if got := run.Dropped(); len(got) != 0 {
		t.Errorf("Dropped() = %v, want none: a flush_failed marker read as a declaration lost to the lock", got)
	}
	if !marker("x").FlushFailed() || (Terminal{Outcome: OutcomeOK}).FlushFailed() {
		t.Error("FlushFailed does not tell the marker from an ordinary terminal")
	}
}

// TestREC34e_AFailedBarrierIsNeverRetriedByFlush: a barrier is a sync, and
// a failed one is as final as a failed Flush. The store reports it from every
// later Flush, without a full sync of the file the barrier failed on, and
// ends its deferral, so what is appended after it -- the flush_failed marker
// first -- is synced in full. A Flush that full-synced the same file and
// succeeded would turn "possibly lost" into "durable": on Linux the barrier
// is fsync, so that is the retried fsync REC-34 rules out (fsyncgate).
func TestREC34e_AFailedBarrierIsNeverRetriedByFlush(t *testing.T) {
	const session = "sess-barrier-fails"
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st.DeferDurability()
	dir := st.RunDir(session)
	if err := os.MkdirAll(dir, dirMode); err != nil {
		t.Fatal(err)
	}
	// A descriptor already closed: the barrier on it fails, while the file
	// it names exists, so a full sync of that file would succeed.
	f, err := os.OpenFile(filepath.Join(dir, FileRecords), appendFlags, fileMode)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	barrierErr := st.syncAppend(f)
	if barrierErr == nil {
		t.Fatal("a barrier on a closed descriptor reported success")
	}
	if st.deferred {
		t.Error("a failed barrier left the store deferring: an append after it would be covered by no flush")
	}
	if err := st.Flush(); !errors.Is(err, barrierErr) {
		t.Fatalf("Flush after a failed barrier = %v, want the barrier's own error %v: the failure was retried", err, barrierErr)
	}
	// An append after the failure is synced in full, and still no Flush
	// reports durability.
	if err := st.AppendCoverage(Coverage{Type: TypeCoverage, SchemaVersion: SessionSchemaVersion, SessionID: session}); err != nil {
		t.Fatal(err)
	}
	if st.dirty != filepath.Join(dir, FileRecords) {
		t.Errorf("an append after the failed barrier was left to a flush (%q)", st.dirty)
	}
	if err := st.Flush(); !errors.Is(err, barrierErr) {
		t.Errorf("a second Flush = %v, want %v", err, barrierErr)
	}
}

// TestREC34l_ABarrierThatIsAFullSyncLeavesNothingToFlush: where the barrier
// is itself a full sync -- off darwin, and on darwin when the filesystem
// refuses the barrier and it falls back -- the append it covered is already
// durable, so the group commit's Flush has nothing left and syncs nothing: a
// deferred hook issues no more syncs than an undeferred one. A barrier that
// only orders the write leaves it for Flush. Flush's sync is observed through
// the files it would cover, which are removed first: a Flush that still
// syncs fails on them (P09-F9).
func TestREC34l_ABarrierThatIsAFullSyncLeavesNothingToFlush(t *testing.T) {
	const session = "sess-full-barrier"
	for _, c := range []struct {
		name          string
		durable       bool
		flushSyncs    bool
		wantFlushFail bool
	}{
		{"a barrier that is a full sync", true, false, false},
		{"a barrier that only orders", false, true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			st, err := Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			st.DeferDurability()
			barriers := 0
			st.barrier = func(*os.File) (bool, error) {
				barriers++
				return c.durable, nil
			}
			if err := st.AppendDeclaration(Declaration{Type: TypeDeclaration, SchemaVersion: SchemaVersion, ToolUseID: "toolu_1", SessionID: session}); err != nil {
				t.Fatal(err)
			}
			if err := st.AppendCoverage(Coverage{Type: TypeCoverage, SchemaVersion: SessionSchemaVersion, SessionID: session}); err != nil {
				t.Fatal(err)
			}
			if barriers != 3 {
				t.Fatalf("%d barriers, want 3: the seq counter, the record and the coverage line", barriers)
			}
			if left := st.dirty != ""; left != c.flushSyncs {
				t.Errorf("after the appends Flush would sync %q; want a file left = %v", st.dirty, c.flushSyncs)
			}
			for _, name := range []string{FileRecords, FileCoverage} {
				if err := os.Remove(filepath.Join(st.RunDir(session), name)); err != nil {
					t.Fatal(err)
				}
			}
			if err := st.Flush(); (err != nil) != c.wantFlushFail {
				t.Errorf("Flush = %v; want it to sync (and fail on the removed file) = %v", err, c.wantFlushFail)
			}
		})
	}
}

// TestREC34k_NotDurableNamesEveryMarkedCall: a flush_failed marker names the
// call whose records may not be durable, and the run reads that back per
// call, each once, so the report can say which declaration is possibly not
// durable rather than only that the session is (REC-34 (c), P09-F11). A
// marker naming no call names nothing here; it still marks the session.
func TestREC34k_NotDurableNamesEveryMarkedCall(t *testing.T) {
	reason := ReasonFlushFailed
	marker := func(id string) Terminal {
		return Terminal{Type: TypeTerminal, SchemaVersion: SchemaVersion, ToolUseID: id, Outcome: OutcomeError, Reason: &reason}
	}
	run := &Run{
		Declarations: []Declaration{{ToolUseID: "toolu_a"}, {ToolUseID: "toolu_b"}, {ToolUseID: "toolu_c"}},
		Terminals: []Terminal{
			{ToolUseID: "toolu_a", Outcome: OutcomeOK},
			marker("toolu_b"), marker("toolu_a"), marker("toolu_b"), marker(""),
			{ToolUseID: "toolu_c", Outcome: OutcomeOK},
		},
	}
	got := run.NotDurable()
	if want := []string{"toolu_b", "toolu_a"}; !slices.Equal(got, want) {
		t.Errorf("NotDurable() = %q, want %q: every marked call once, in the order marked, and no empty id", got, want)
	}
	if got := (&Run{Terminals: []Terminal{{ToolUseID: "toolu_a", Outcome: OutcomeOK}}}).NotDurable(); len(got) != 0 {
		t.Errorf("NotDurable() with no marker = %q, want none", got)
	}
}
