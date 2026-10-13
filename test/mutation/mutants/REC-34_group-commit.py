# REC-34: group commit. Hook and post order their appends with barriers inside
# the lock and make them durable with one full sync, after the last append,
# outside the lock and before stdout; a failed flush is marked, with the marker
# synced in full, never retried, and never renders as fine. The sync recorder
# (rashomonfault) is what sees a missing flush, a full sync back inside the
# lock, or a marker left behind a barrier.
m("REC-34 the hook never flushes", "cmd/rashomon/main.go",
  "\t\tif err := safe.Guard(h.Flush); err != nil {\n\t\t\tfmt.Fprintln(stderr, flushLostLine)\n\t\t}\n",
  "", "TestREC34_")
m("REC-34 post never flushes", "cmd/rashomon/main.go",
  "\t\tif err := safe.Guard(p.Flush); err != nil {\n\t\t\tfmt.Fprintln(stderr, flushLostLine)\n\t\t}\n",
  "", "TestREC34_")
m("REC-34 the hook flushes before its last append", "cmd/rashomon/main.go",
  "\t\t_ = safe.Guard(func() error { h.Close(sig, captureErr); return nil })\n\t\tfault.Inject(fault.PointHookBeforeFlush)\n\t\tif err := safe.Guard(h.Flush); err != nil {\n\t\t\tfmt.Fprintln(stderr, flushLostLine)\n\t\t}\n",
  "\t\tif err := safe.Guard(h.Flush); err != nil {\n\t\t\tfmt.Fprintln(stderr, flushLostLine)\n\t\t}\n\t\t_ = safe.Guard(func() error { h.Close(sig, captureErr); return nil })\n\t\tfault.Inject(fault.PointHookBeforeFlush)\n",
  "TestREC34_")
m("REC-34 the ordered append syncs in full inside the lock", "internal/store/store.go",
  "\treturn s.syncAppend(f)\n}\n\n// lockCounted takes", "\treturn syncFull(f)\n}\n\n// lockCounted takes", "TestREC34_")
m("REC-34 the seq counter syncs in full inside the lock", "internal/store/store.go",
  "\tseq, err := nextSeq(dir, s.syncAppend)", "\tseq, err := nextSeq(dir, syncFull)", "TestREC34_")
m("REC-34 group commit off: a deferred store syncs every append in full", "internal/store/store.go",
  "\tif !deferred {\n\t\treturn syncFull(f)\n\t}\n", "\tif true {\n\t\treturn syncFull(f)\n\t}\n", "TestREC34_")
m("REC-34 probe defers durability and never flushes", "cmd/rashomon/main.go",
  "\t\tif err := hook.RunProbe(sig, phase, stdin, st, time.Now); err != nil {",
  "\t\tst.DeferDurability()\n\t\tif err := hook.RunProbe(sig, phase, stdin, st, time.Now); err != nil {",
  "TestREC34_")
m("REC-34 a failed flush leaves no marker", "internal/hook/flush.go",
  "\treturn st.SpillTerminal(store.Terminal{", "\treturn nil\n\treturn st.SpillTerminal(store.Terminal{", "TestREC34_")
m("REC-34 the marker names the wrong reason", "internal/hook/flush.go",
  "\treason := store.ReasonFlushFailed\n", "\treason := store.ReasonInternalError\n", "TestREC34_")
m("REC-34 the flush_failed marker is written behind a barrier", "internal/store/store.go",
  "\ts.flushErr = err\n\ts.deferred = false\n\treturn err\n", "\ts.flushErr = err\n\treturn err\n", "TestREC34_")
m("REC-34 a failed barrier is left for Flush to retry", "internal/store/store.go",
  "\tif err != nil {\n\t\treturn s.syncFailed(err)\n\t}\n\tif durable {", "\tif err != nil {\n\t\treturn err\n\t}\n\tif durable {", "TestREC34e_")
m("REC-34 a failed flush is forgotten and retried", "internal/store/store.go",
  "\tif s.flushErr != nil {\n\t\treturn s.flushErr\n\t}\n", "", "TestREC34c_")
m("REC-34 the report renders a flush_failed marker as fine", "internal/report/report.go",
  "\t\tif t.FlushFailed() {\n\t\t\tsess.Coverage.add(store.ReasonFlushFailed)\n", "\t\tif t.FlushFailed() && false {\n\t\t\tsess.Coverage.add(store.ReasonFlushFailed)\n",
  "TestREC34_")
m("REC-34 the unmarkable failure is silent", "cmd/rashomon/main.go",
  "\t\tif err := safe.Guard(h.Flush); err != nil {\n\t\t\tfmt.Fprintln(stderr, flushLostLine)\n\t\t}\n",
  "\t\tif err := safe.Guard(h.Flush); err != nil {\n\t\t\treturn nil\n\t\t}\n", "TestREC34_")
m("REC-34 a flush_failed marker closes a declaration", "internal/store/read.go",
  "\t\tif !t.FlushFailed() {\n\t\t\tclosed[t.ToolUseID] = true\n", "\t\tif true {\n\t\t\tclosed[t.ToolUseID] = true\n", "TestREC34d_")
m("REC-34 a flush_failed marker reads as a dropped declaration", "internal/store/read.go",
  "\t\tif !declared[t.ToolUseID] && !t.FlushFailed() {", "\t\tif !declared[t.ToolUseID] {", "TestREC34d_")
m("REC-34 the digest lets a flush_failed marker close a declaration", "internal/digest/declarations.go",
  "\t\tif ids[t.ToolUseID] && !t.FlushFailed() {", "\t\tif ids[t.ToolUseID] {", "TestREC34g_")
# The darwin barrier itself (P09-F4). The recorder logs a barrier only once the
# kernel accepted one and the fallback as the full sync it is, so a barrier that
# became a full sync shows up as one inside the lock.
m("REC-34 the barrier issues F_FULLFSYNC", "internal/store/sync_darwin.go",
  "const fBarrierFsync = 85", "const fBarrierFsync = 51", "TestREC34f_")
m("REC-34 the barrier never reaches the kernel", "internal/store/sync_darwin.go",
  "\t_, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, fBarrierFsync, 0)",
  "\terrno := syscall.Errno(0)", "TestREC34f_")
m("REC-34 an accepted barrier falls back to a full sync", "internal/store/sync_darwin.go",
  "\t\tcase errno == 0:\n\t\t\tfault.RecordSync(fault.SyncBarrier, f.Name())\n\t\t\treturn false, nil\n",
  "\t\tcase errno == 0:\n\t\t\treturn false, syncFull(f)\n", "TestREC34_")
IMPORTS["REC-34 an accepted barrier falls back to a full sync"] = ("internal/store/sync_darwin.go",
  "import (\n\t\"os\"\n\t\"syscall\"\n\n\t\"github.com/altrace-dev-role/rashomon/internal/fault\"\n)",
  "import (\n\t\"os\"\n\t\"syscall\"\n)")
# An accepted barrier followed by a full sync the recorder never hears of: the
# recorder logs the barrier, so REC-34's acceptance test stays green, and only
# the source check that every full sync is syncFull's sees it (P09-F4, the
# residual).
m("REC-34 an accepted barrier also syncs in full, unseen", "internal/store/sync_darwin.go",
  "\t\t\tfault.RecordSync(fault.SyncBarrier, f.Name())\n\t\t\treturn false, nil\n",
  "\t\t\tfault.RecordSync(fault.SyncBarrier, f.Name())\n\t\t\treturn false, f.Sync()\n", "TestREC34h_")
m("REC-34 the barrier's fallback syncs where the recorder cannot see it", "internal/store/sync_darwin.go",
  "\t\tcase barrierRefused(errno):\n\t\t\tif err := syncFullAsBarrier(f); err != nil {\n",
  "\t\tcase barrierRefused(errno):\n\t\t\tif err := f.Sync(); err != nil {\n", "TestREC34h_")
m("REC-34 group commit off inside barrierSync", "internal/store/sync_darwin.go",
  "func barrierSync(f *os.File) (durable bool, err error) {\n\treturn barrierWith(f, fcntlBarrier)\n",
  "func barrierSync(f *os.File) (durable bool, err error) {\n\treturn true, syncFull(f)\n", "TestREC34_")
# Every lock counted and every sync seen (P09-F8): the recorder's locked= flag
# is only as good as the locks it is told about.
m("REC-34 forget's rewrite syncs where the recorder cannot see it", "internal/store/gaps.go",
  "\treturn syncFull(f)\n}\n\n// rewriteSpill", "\treturn f.Sync()\n}\n\n// rewriteSpill", "TestREC34h_")
m("REC-34 forget's rewrite sync unseen, judged end to end", "internal/store/gaps.go",
  "\tif err := f.Truncate(off); err != nil {\n\t\treturn err\n\t}\n\treturn syncFull(f)\n",
  "\tif err := f.Truncate(off); err != nil {\n\t\treturn err\n\t}\n\treturn f.Sync()\n",
  "TestREC34_Forget")
m("REC-34 eviction takes a lock the recorder does not count", "internal/store/evict.go",
  "\tunlock, err := lockCounted(gf, s.lockWait(lockBudget))", "\tunlock, err := LockFile(gf, s.lockWait(lockBudget))", "TestREC34h_")
# The store-level forget record (#53) landed beside group commit and took its
# lock and sync directly; both go through the recorder's sites too.
m("REC-34 the forgotten-host record takes a lock the recorder does not count", "internal/store/gaps.go",
  "\tdefer f.Close() //nolint:errcheck // Sync below reports the write failure\n\tunlock, err := lockCounted(f, lockBudget)",
  "\tdefer f.Close() //nolint:errcheck // Sync below reports the write failure\n\tunlock, err := LockFile(f, lockBudget)", "TestREC34h_")
m("REC-34 the forgotten-host record syncs where the recorder cannot see it", "internal/store/gaps.go",
  "\treturn syncFull(f)\n}\n\nfunc (s *Store) forgetHostInRun", "\treturn f.Sync()\n}\n\nfunc (s *Store) forgetHostInRun", "TestREC34h_")
m("REC-34 the counted lock counts nothing", "internal/store/store.go",
  "\tfault.LockTaken()\n", "", "TestREC34_")
m("REC-34 a marker naming no call reads as a lost call", "internal/report/testbending.go",
  "\t\tif t.FlushFailed() && t.ToolUseID == \"\" {\n\t\t\tcontinue\n\t\t}\n", "", "TestREC34i_")
# A barrier that was itself a full sync leaves Flush nothing (P09-F9): off
# darwin every barrier is one, and a Flush after it would be a second fsync per
# hook that an undeferred store never issued. sync_other.go itself has no
# mutant in this file: it is judged by TestREC34l_TheStandInBarrierIsDurable
# and by TestREC34_GroupCommitFlushesBeforeExit off darwin. sync_darwin.go is
# the opposite case, not this one: it carries its own 14 mutants below,
# judged by TestREC34f_ on darwin and reported NOT JUDGED (platform) by the
# sweep everywhere else, because the file does not build there.
m("REC-34 a full-sync barrier still leaves its file to Flush", "internal/store/store.go",
  "\tif durable {\n\t\ts.mu.Lock()\n\t\ts.dirty = \"\"\n\t\ts.mu.Unlock()\n\t}\n", "\tif durable && false {\n\t\ts.mu.Lock()\n\t\ts.dirty = \"\"\n\t\ts.mu.Unlock()\n\t}\n", "TestREC34l_")
m("REC-34 an accepted darwin barrier reports its write durable", "internal/store/sync_darwin.go",
  "\t\t\tfault.RecordSync(fault.SyncBarrier, f.Name())\n\t\t\treturn false, nil\n",
  "\t\t\tfault.RecordSync(fault.SyncBarrier, f.Name())\n\t\t\treturn true, nil\n", "TestREC34f_")
# The refused barrier's fallback (P09-F9, round 2). It reports durable, so
# syncAppend clears the dirty file and Flush issues no full sync: the fallback's
# own syncFullAsBarrier is the only sync a deferred append gets on a volume
# that refuses barriers (HFS+, exFAT). APFS never refuses, so
# TestREC34f_ARefusedBarrierSyncsInFull stands a refusing kernel in through
# barrierWith's fcntl seam.
m("REC-34 a refused barrier reports durable without syncing", "internal/store/sync_darwin.go",
  "\t\t\tif err := syncFullAsBarrier(f); err != nil {\n\t\t\t\treturn false, err\n\t\t\t}\n\t\t\treturn true, nil\n",
  "\t\t\treturn true, nil\n", "TestREC34f_")
m("REC-34 a refused barrier's full sync reports the write not durable", "internal/store/sync_darwin.go",
  "\t\t\t\treturn false, err\n\t\t\t}\n\t\t\treturn true, nil\n",
  "\t\t\t\treturn false, err\n\t\t\t}\n\t\t\treturn false, nil\n", "TestREC34f_")
m("REC-34 an I/O error on the barrier is retried as a full sync", "internal/store/sync_darwin.go",
  "\t\tdefault:\n\t\t\treturn false, errno\n",
  "\t\tdefault:\n\t\t\treturn false, syncFull(f)\n", "TestREC34f_")
m("REC-34 an interrupted barrier is not issued again", "internal/store/sync_darwin.go",
  "\t\tcase errno == syscall.EINTR:\n\t\t\tcontinue\n",
  "\t\tcase errno == syscall.EINTR:\n\t\t\treturn false, errno\n", "TestREC34f_")
# The barrier's fcntl command (P09-F4, round 2): F_FULLFSYNC in its place is a
# call the kernel accepts, so the recorder logs a barrier, the acceptance test
# sees barriers inside the lock and one full sync outside, and the device flush
# is back inside every append lock. Only the source check sees the command.
m("REC-34 the barrier's fcntl is F_FULLFSYNC", "internal/store/sync_darwin.go",
  "fd, fBarrierFsync, 0)", "fd, syscall.F_FULLFSYNC, 0)", "TestREC34h_")
m("REC-34 the barrier's fcntl command is a bare number", "internal/store/sync_darwin.go",
  "fd, fBarrierFsync, 0)", "fd, 51, 0)", "TestREC34h_")
m("REC-34 the barrier constant is F_FULLFSYNC's number", "internal/store/sync_darwin.go",
  "const fBarrierFsync = 85", "const fBarrierFsync = 51", "TestREC34h_")
# The report names each call a flush_failed marker covers (REC-34 (c): "the
# report renders such a declaration as possibly not durable"; P09-F11), beside
# the session-level reason it already carried.
m("REC-34 the run names no call not durable", "internal/store/read.go",
  "\t\tif t.FlushFailed() && t.ToolUseID != \"\" && hasRecord[t.ToolUseID] && !seen[t.ToolUseID] {",
  "\t\tif t.FlushFailed() && t.ToolUseID != \"\" && hasRecord[t.ToolUseID] && !seen[t.ToolUseID] && false {", "TestREC34k_")
m("REC-34 a marker naming no call names an empty one", "internal/store/read.go",
  "\t\tif t.FlushFailed() && t.ToolUseID != \"\" && hasRecord[t.ToolUseID] && !seen[t.ToolUseID] {",
  "\t\tif t.FlushFailed() && hasRecord[t.ToolUseID] && !seen[t.ToolUseID] {", "TestREC34r_")
m("REC-34 a call marked twice is named twice", "internal/store/read.go",
  "\t\tif t.FlushFailed() && t.ToolUseID != \"\" && hasRecord[t.ToolUseID] && !seen[t.ToolUseID] {",
  "\t\tif t.FlushFailed() && t.ToolUseID != \"\" && hasRecord[t.ToolUseID] {", "TestREC34k_")
m("REC-34 the report lists no call not durable", "internal/report/report.go",
  "\t\t\tNotDurable:       nonNil(run.NotDurable()),", "\t\t\tNotDurable:       []string{},", "TestREC34k_")
m("REC-34 the text never says which call", "internal/report/text.go",
  "list(d.Dropped), list(d.NotDurable))", "list(d.Dropped), list(nil))", "TestREC34k_")
# The signal window between Close and Flush (P09-F12): removing the
# injection point does not change ordinary runs, only whether a fault at
# hook.before_flush exists at all to hang on.
m("REC-34 the hook never signals before flush", "cmd/rashomon/main.go",
  "\t\t_ = safe.Guard(func() error { h.Close(sig, captureErr); return nil })\n\t\tfault.Inject(fault.PointHookBeforeFlush)\n",
  "\t\t_ = safe.Guard(func() error { h.Close(sig, captureErr); return nil })\n", "TestREC34_",
  # Its only judging test, TestREC34_SignalBeforeFlushReachesTheSyncLog,
  # calls skipWithoutAFlush and so skips whole off darwin (P09-F9: there is
  # no Flush to race a signal against when every barrier is already a full
  # sync). Without platform=, that puts this mutant in the "skipped" bucket
  # on Linux and Windows, which --strict treats as a broken environment
  # rather than as the darwin-only mutant it is.
  platform="darwin")
# The flush_failed marker rides a schema bump instead of a new record type
# (record.go's SchemaVersion doc): a reader that only understands up to 4
# must skip the whole line, never read Reason and half-understand it as an
# ordinary closing terminal. Siblings of "B2 the reader stops accepting
# schema 3" and "MS the reader stops accepting schema 4".
m("REC-34 the reader stops accepting schema 5", "internal/store/record.go",
  "\treturn version >= 1 && version <= 5",
  "\treturn version >= 1 && version <= 5 && version != 5", "TestAcceptsAdmits")

# A flush_failed marker (REC-34) sharing a tool_use_id with a real record
# must not stand in for its recorded time: turnDropped windows a dropped
# terminal by it, and Join's span decides whether a turn is old enough to
# drop.
m("REC-34 the flush_failed marker's time overwrites the dropped terminal's own", "internal/digest/declarations.go",
  "\t\tif t.FlushFailed() {\n\t\t\t// Not an ending (Terminal.FlushFailed's doc): it must not stand\n\t\t\t// in for the terminal it shares a tool_use_id with, or that\n\t\t\t// terminal's own recorded time -- the one `all` was windowed\n\t\t\t// against -- is overwritten by whenever the marker landed.\n\t\t\tcontinue\n\t\t}\n\t\trecordedAt[t.ToolUseID] = t.RecordedAtMS\n", '\t\trecordedAt[t.ToolUseID] = t.RecordedAtMS\n', "TestREC34n_")
m("REC-34 the flush_failed marker's time stretches a turn's span", "internal/spend/join.go",
  "\t\tif x.FlushFailed() {\n\t\t\t// Not an ending (Terminal.FlushFailed's doc), and written at\n\t\t\t// process exit, long after the call it marks: letting it stand\n\t\t\t// for the turn's own last recorded time would stretch a turn\n\t\t\t// Join should treat as old enough to drop into looking current.\n\t\t\tcontinue\n\t\t}\n\t\tif t, ok := owner[x.ToolUseID]; ok {\n\t\t\tspan(t, x.RecordedAtMS)\n\t\t}\n", '\t\tif t, ok := owner[x.ToolUseID]; ok {\n\t\t\tspan(t, x.RecordedAtMS)\n\t\t}\n', "TestREC34o_")

# flushFile's reopen is ENOENT in two different situations (REC-34): the
# run was removed whole (forget, eviction), where there is nothing left to
# flush and marking would recreate the directory that was removed, and a
# lone file missing from a run that still exists, which TestREC34c_'s own
# scenario is and must keep failing.
m("REC-34 the run being gone is a flush failure", "internal/store/store.go",
  '\tif err != nil {\n\t\tif errors.Is(err, fs.ErrNotExist) && runGone(path) {\n\t\t\treturn nil\n\t\t}\n\t\treturn err\n\t}\n', '\tif err != nil {\n\t\treturn err\n\t}\n', "TestREC34p_")
m("REC-34 any ENOENT is nothing to flush, not just a gone run", "internal/store/store.go",
  "errors.Is(err, fs.ErrNotExist) && runGone(path)", "errors.Is(err, fs.ErrNotExist)", "TestREC34c_")

# A flush_failed marker names which invocation wrote it (REC-34): without
# phase a reader cannot tell whether the declaration or the execution is
# the one possibly not durable. TestREC34_FailedFlushIsMarkedAndNeverRetried
# asserts the marker's phase in both its hook and post subtests.
#
# Its only judge, that test, calls skipWithoutAFlush and so skips whole off
# darwin (P09-F9: there is no Flush to fail when every barrier is already a
# full sync). Without platform=, both land in the "skipped" bucket on Linux
# and Windows, which --strict treats as a broken environment rather than as
# the darwin-only mutants they are -- the same fix "REC-34 the hook never
# signals before flush" already got, above.
m("REC-34 the flush_failed marker carries no phase", "internal/hook/flush.go",
  "\t\tReason:        &reason,\n\t\tPhase:         &phase,\n", "\t\tReason:        &reason,\n", "TestREC34_FailedFlushIsMarkedAndNeverRetried",
  platform="darwin")
m("REC-34 the hook's own flush marks itself as post", "internal/hook/flush.go",
  "return flushOrMark(h.st, h.sessionID, h.toolUseID, h.now(), store.PhaseCall)",
  "return flushOrMark(h.st, h.sessionID, h.toolUseID, h.now(), store.PhasePost)", "TestREC34_FailedFlushIsMarkedAndNeverRetried",
  platform="darwin")
# The marker rides its own version, store.FlushFailedSchemaVersion, so that
# a 1.1.x reader skips only this one line rather than every call record this
# invocation wrote (record.go's FlushFailedSchemaVersion doc). Collapsing it
# back onto store.SchemaVersion is the exact regression that doc exists to
# name, caught here at the one site that writes it.
m("REC-34 the marker rides the call records' own version", "internal/hook/flush.go",
  "SchemaVersion: store.FlushFailedSchemaVersion,", "SchemaVersion: store.SchemaVersion,",
  "TestREC34_FailedFlushIsMarkedAndNeverRetried")

# A flush_failed marker can name a call whose own record never landed: the
# seq counter's barrier, the first ordered append issues, is what failed, so
# appendOrdered returned before the declaration or execution line was ever
# written. NotDurable must not call that id possibly not durable when there
# is no declaration or execution to lose (REC-34).
m("REC-34 not durable names a call with no record at all", "internal/store/read.go",
  "\t\tif t.FlushFailed() && t.ToolUseID != \"\" && hasRecord[t.ToolUseID] && !seen[t.ToolUseID] {",
  "\t\tif t.FlushFailed() && t.ToolUseID != \"\" && !seen[t.ToolUseID] {", "TestREC34r_")

# The version gate itself, phase's own twin to "B2 a schema-3 field is
# required at every version" (B2_store-schema.py): putting phase in
# terminal's TOP-LEVEL required is the tempting edit, and it invalidates
# every terminal already on disk from before this field existed.
m("REC-34 phase is required at every version", "docs/store-schema.json",
  '        "outcome",\n        "reason"\n      ],\n      "allOf": [',
  '        "outcome",\n        "reason",\n        "phase"\n      ],\n      "allOf": [',
  "TestSchema5|TestStoreSchema")
