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
  "\treturn s.syncAppend(f)\n}\n\n// lockAppend takes", "\treturn syncFull(f)\n}\n\n// lockAppend takes", "TestREC34_")
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
  "\tif err != nil {\n\t\treturn s.syncFailed(err)\n\t}\n\treturn nil\n}\n", "\treturn err\n}\n", "TestREC34e_")
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
