package store

import (
	"os"
	"path/filepath"
	"testing"
)

// TestREC34b_DeferredAppendsLeaveOneFlushBehind: a deferred Store records
// which file the group commit's one full sync has to cover, and Flush clears
// it; a Store that never deferred leaves nothing for Flush, because each of
// its appends was already synced in full (REC-34).
func TestREC34b_DeferredAppendsLeaveOneFlushBehind(t *testing.T) {
	const session = "sess-deferred"
	t.Run("deferred", func(t *testing.T) {
		st, err := Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
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
