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
  "\t\t_ = safe.Guard(func() error { h.Close(sig, captureErr); return nil })\n\t\tif err := safe.Guard(h.Flush); err != nil {\n\t\t\tfmt.Fprintln(stderr, flushLostLine)\n\t\t}\n",
  "\t\tif err := safe.Guard(h.Flush); err != nil {\n\t\t\tfmt.Fprintln(stderr, flushLostLine)\n\t\t}\n\t\t_ = safe.Guard(func() error { h.Close(sig, captureErr); return nil })\n",
  "TestREC34_")
m("REC-34 the ordered append syncs in full inside the lock", "internal/store/store.go",
  "\treturn s.syncAppend(f)\n}\n\n// lockCounted takes", "\treturn syncFull(f)\n}\n\n// lockCounted takes", "TestREC34_")
m("REC-34 the seq counter syncs in full inside the lock", "internal/store/store.go",
  "\tseq, err := nextSeq(dir, s.syncAppend)", "\tseq, err := nextSeq(dir, syncFull)", "TestREC34_")
m("REC-34 group commit off: a deferred store syncs every append in full", "internal/store/store.go",
  "\tif !s.deferred {\n\t\treturn syncFull(f)\n\t}\n", "\tif true {\n\t\treturn syncFull(f)\n\t}\n", "TestREC34_")
m("REC-34 probe defers durability and never flushes", "cmd/rashomon/main.go",
  "\t\thook.RunProbe(sig, phase, stdin, st, time.Now)", "\t\tst.DeferDurability()\n\t\thook.RunProbe(sig, phase, stdin, st, time.Now)",
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
  "\t\tcase barrierRefused(errno):\n\t\t\tif err := syncFull(f); err != nil {\n",
  "\t\tcase barrierRefused(errno):\n\t\t\tif err := f.Sync(); err != nil {\n", "TestREC34h_")
m("REC-34 group commit off inside barrierSync", "internal/store/sync_darwin.go",
  "func barrierSync(f *os.File) (durable bool, err error) {\n\treturn barrierWith(f, fcntlBarrier)\n",
  "func barrierSync(f *os.File) (durable bool, err error) {\n\treturn true, syncFull(f)\n", "TestREC34_")
# Every lock counted and every sync seen (P09-F8): the recorder's locked= flag
# is only as good as the locks it is told about.
m("REC-34 forget's rewrite syncs where the recorder cannot see it", "internal/store/gaps.go",
  "\treturn syncFull(f)\n}\n\n// rewriteAt", "\treturn f.Sync()\n}\n\n// rewriteAt", "TestREC34h_")
m("REC-34 forget's rewrite sync unseen, judged end to end", "internal/store/gaps.go",
  "\tfor _, line := range lines {\n\t\tif _, err := f.Write(append(line, '\\n')); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n\treturn syncFull(f)\n",
  "\tfor _, line := range lines {\n\t\tif _, err := f.Write(append(line, '\\n')); err != nil {\n\t\t\treturn err\n\t\t}\n\t}\n\treturn f.Sync()\n",
  "TestREC34_Forget")
m("REC-34 eviction takes a lock the recorder does not count", "internal/store/gaps.go",
  "\tunlock, err := lockCounted(gf, lockBudget)", "\tunlock, err := lockFile(gf, lockBudget)", "TestREC34h_")
m("REC-34 the counted lock counts nothing", "internal/store/store.go",
  "\tfault.LockTaken()\n", "", "TestREC34_")
m("REC-34 a marker naming no call reads as a lost call", "internal/report/testbending.go",
  "\t\tif t.FlushFailed() && t.ToolUseID == \"\" {\n\t\t\tcontinue\n\t\t}\n", "", "TestREC34i_")
# A barrier that was itself a full sync leaves Flush nothing (P09-F9): off
# darwin every barrier is one, and a Flush after it would be a second fsync per
# hook that an undeferred store never issued. The stand-in barrier's own half
# (sync_other.go) is judged by TestREC34l_TheStandInBarrierIsDurable and by
# TestREC34_GroupCommitFlushesBeforeExit off darwin, where this sweep does not
# run, so it has no mutant here.
m("REC-34 a full-sync barrier still leaves its file to Flush", "internal/store/store.go",
  "\tif durable {\n\t\ts.dirty = \"\"\n\t}\n", "\tif durable && false {\n\t\ts.dirty = \"\"\n\t}\n", "TestREC34l_")
m("REC-34 an accepted darwin barrier reports its write durable", "internal/store/sync_darwin.go",
  "\t\t\tfault.RecordSync(fault.SyncBarrier, f.Name())\n\t\t\treturn false, nil\n",
  "\t\t\tfault.RecordSync(fault.SyncBarrier, f.Name())\n\t\t\treturn true, nil\n", "TestREC34f_")
# The refused barrier's fallback (P09-F9, round 2). It reports durable, so
# syncAppend clears the dirty file and Flush issues no full sync: the fallback's
# own syncFull is the only sync a deferred append gets on a volume that refuses
# barriers (HFS+, exFAT). APFS never refuses, so TestREC34f_ARefusedBarrierSyncsInFull
# stands a refusing kernel in through barrierWith's fcntl seam.
m("REC-34 a refused barrier reports durable without syncing", "internal/store/sync_darwin.go",
  "\t\t\tif err := syncFull(f); err != nil {\n\t\t\t\treturn false, err\n\t\t\t}\n\t\t\treturn true, nil\n",
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
m("REC-34 the barrier constant is F_FULLFSYNC's number, judged on every platform", "internal/store/sync_darwin.go",
  "const fBarrierFsync = 85", "const fBarrierFsync = 51", "TestREC34h_")
# The report names each call a flush_failed marker covers (REC-34 (c): "the
# report renders such a declaration as possibly not durable"; P09-F11), beside
# the session-level reason it already carried.
m("REC-34 the run names no call not durable", "internal/store/read.go",
  "\t\tif t.FlushFailed() && t.ToolUseID != \"\" && !seen[t.ToolUseID] {",
  "\t\tif t.FlushFailed() && t.ToolUseID != \"\" && !seen[t.ToolUseID] && false {", "TestREC34k_")
m("REC-34 a marker naming no call names an empty one", "internal/store/read.go",
  "\t\tif t.FlushFailed() && t.ToolUseID != \"\" && !seen[t.ToolUseID] {",
  "\t\tif t.FlushFailed() && !seen[t.ToolUseID] {", "TestREC34k_")
m("REC-34 a call marked twice is named twice", "internal/store/read.go",
  "\t\tif t.FlushFailed() && t.ToolUseID != \"\" && !seen[t.ToolUseID] {",
  "\t\tif t.FlushFailed() && t.ToolUseID != \"\" {", "TestREC34k_")
m("REC-34 the report lists no call not durable", "internal/report/report.go",
  "\t\t\tNotDurable:       nonNil(run.NotDurable()),", "\t\t\tNotDurable:       []string{},", "TestREC34k_")
m("REC-34 the text never says which call", "internal/report/text.go",
  "list(d.Dropped), list(d.NotDurable))", "list(d.Dropped), list(nil))", "TestREC34k_")
