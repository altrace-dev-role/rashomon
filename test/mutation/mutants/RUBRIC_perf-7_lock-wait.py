# perf-7: lock contention was invisible until a wait ran out and its record
# spilled. lockFile reports how long it waited, a Store adds every append's
# wait up, a hook invocation notes its total in its run when that is the
# run's longest, and status shows the longest across runs.
P7 = "TestRubricPERF7_LockFileReportsItsWait|TestRubricPERF7_AContendedInvocationNotesItsLongestWait"
m("perf-7 lockFile reports no wait", "internal/store/lock_unix.go",
  "\t\ttime.Sleep(min(backoff, left))\n\t\twaited = time.Since(start)\n", "\t\ttime.Sleep(min(backoff, left))\n", P7)
m("perf-7 an append's wait is not added up", "internal/store/store.go",
  "\ts.lockWaited.Add(int64(waited))\n", "\t_ = waited\n", "TestRubricPERF7_AContendedInvocationNotesItsLongestWait")
m("perf-7 the invocation's wait is not noted", "internal/store/store.go",
  "\ts.noteLockWait(rec.SessionID)\n", "", "TestRubricPERF7_AContendedInvocationNotesItsLongestWait")
m("perf-7 a shorter wait replaces the run's longest", "internal/store/lockwait.go",
  "err == nil && prev >= ms {", "err == nil && prev < 0 {", "TestRubricPERF7_AContendedInvocationNotesItsLongestWait")
m("perf-7 the longest wait is the last run's", "internal/store/lockwait.go",
  "\t\tif ms > longest {\n", "\t\tif ms > 0 {\n", "TestRubricPERF7_AContendedInvocationNotesItsLongestWait")
m("perf-7 status does not show the lock wait", "cmd/rashomon/main.go",
  "\t\tstatusLockWait(stdout, st)\n", "", "TestRubricPERF7_StatusShowsTheLongestLockWait")
