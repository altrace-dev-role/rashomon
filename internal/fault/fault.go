// Package fault injects faults so the panic barrier and the atomic-write
// guarantee can be tested against the failures that actually break them.
//
// The injection points are compiled in only under the `rashomonfault` build tag.
// A released binary contains no injection path at all, which matters for more
// than tidiness: an attacker who could set an environment variable and make the
// recorder abandon a run would be attacking exactly the coverage guarantee this
// program exists to provide.
//
// Every kind panics, because only a panic reaches the exit code that blocks a
// tool call, with one exception: "fail=N" returns an error, and it is reached
// through Fail rather than Inject. What it exercises is the retry around a
// transient settings read, and a read that panicked would exercise the panic
// barrier instead -- the retry would never run.
package fault

// Points at which Inject is called. Named constants rather than string literals
// so that a test naming a point that no longer exists fails to compile instead
// of silently injecting nothing.
const (
	PointHookStart  = "hook.start"
	PointHookParsed = "hook.parsed"
	// PointLabel is inside the file-label derivation, within its own
	// recovered region. It belongs to the hook package and not to shape
	// because shape is the audited file the no-content guarantee rests on,
	// and keeping an injection hook out of it keeps that file's dependency
	// list as short as the audit needs.
	PointLabel                = "hook.label"
	PointStoreWrite           = "store.write"
	PointHookAfterDeclaration = "hook.after_declaration"
	PointPostStart            = "post.start"
	PointPostParsed           = "post.parsed"
	PointSettingsLoad         = "settings.load"
	PointSettingsOpened       = "settings.write.opened"
	PointSettingsBeforeRename = "settings.write.before_rename"
	// PointRecapStart is inside cmdRecap's own guarded body, not inside a
	// handler type -- there is no Recap struct the way Post and Handler give
	// hook and post one, so the injection sits directly in cmd/rashomon
	// alongside the command it exercises. What it proves is narrower than
	// the recorder's points too: not "no panic escapes", which the same
	// safe.Guard already guarantees here, but "a panic here still exits 0
	// and prints nothing" -- H-93's own bar, stricter than the recorder's
	// "never exit 2".
	PointRecapStart = "recap.start"
	// PointStoreFullSync is inside every full sync the store issues, after
	// the sync itself has run. "fail=N" there turns the first N into
	// failures, which is the only way to reach a failed flush without a
	// failing disk (REC-34): a deferred hook's first full sync is its Flush,
	// and after a failed one its second is the flush_failed marker's.
	PointStoreFullSync = "store.full_sync"
	// PointStoreBarrier is inside every barrier a deferred store issues,
	// after the barrier itself has run. "fail=1" there fails the barrier on
	// the seq counter of a hook's first ordered append, which is how REC-34
	// reaches a failed barrier without a failing disk.
	PointStoreBarrier = "store.barrier"
	// PointHookBeforeFlush is after Close has appended the terminal and
	// coverage records and before Flush issues its one full sync (REC-34):
	// a signal caught there (hook.WatchSignals) still reaches Flush, and
	// one that is not ends the process with those records on disk but
	// nothing yet fully synced.
	PointHookBeforeFlush = "hook.before_flush"
	// PointStoreAppendWrite is inside every append made under its file's
	// lock, after the write has gone out. "fail=N" turns the first N into
	// failures, which is how a failed append's cut-back is reached without
	// a full disk: the line is whole on disk when the failure arrives, so a
	// missing cut-back leaves a record the writer was told did not land.
	PointStoreAppendWrite = "store.append.write"
	// The durability boundaries a crash can land between (test-5). Each is
	// reached with a panic, which stands in for the crash: everything the
	// process wrote before the point is on disk, nothing after it is.
	//
	// PointStoreSeqAdvanced is after a run's seq counter is advanced and
	// before the record that takes the number is written.
	PointStoreSeqAdvanced = "store.seq.advanced"
	// PointStoreRewritePartial is inside forget's in-place rewrite, after
	// each kept line is written and before the file is cut to its new
	// length.
	PointStoreRewritePartial = "store.rewrite.partial"
	// PointBaselineBeforeRename and PointRecapBeforeRename are after a
	// project baseline, or recap's state, is written to its temporary name
	// and before it is renamed over the old one.
	PointBaselineBeforeRename = "baseline.before_rename"
	PointRecapBeforeRename    = "recap.before_rename"
)

// Sync kinds, as the sync recorder logs them (RecordSync). A barrier orders
// a write on the device ahead of everything written after it: fcntl
// F_BARRIERFSYNC on darwin, logged once the kernel has accepted it, and a
// plain fsync elsewhere, where nothing cheaper exists. A full sync makes
// everything written before it durable: F_FULLFSYNC on darwin, fsync
// elsewhere; a darwin barrier the filesystem refuses falls back to one and
// is logged as one.
const (
	SyncBarrier = "barrier"
	SyncFull    = "full"
	// SyncBarrierFull is a full sync issued where a barrier was wanted: off
	// darwin, where nothing cheaper exists, or on darwin when the
	// filesystem refuses F_BARRIERFSYNC. It costs exactly what SyncFull
	// does and, unlike SyncFull, runs inside the append's lock -- a reader
	// of the log must not read it as the cheap ordering SyncBarrier names.
	SyncBarrierFull = "barrier-full"
)
