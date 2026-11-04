# perf-1: digest's reader budget waits out a writer's measured hold
# (BenchmarkOrderedAppendHold) rather than the sub-millisecond one it assumed.
# It is one deadline for the whole read, and a lock wait gives up at its
# deadline rather than at the end of the polling sleep that crosses it.
m("perf-1 the reader gives up inside a hook's hold", "internal/digest/digest.go",
  "const readLockBudget = 30 * time.Millisecond\n", "const readLockBudget = 5 * time.Millisecond\n",
  "TestRubricPERF1_DigestWaitsOutAHookHold")
m("perf-1 the reader gives up inside a loaded hold", "internal/digest/digest.go",
  "const readLockBudget = 30 * time.Millisecond\n", "const readLockBudget = 10 * time.Millisecond\n",
  "TestRubricPERF1_DigestWaitsOutALoadedHold")
m("perf-1 the consistent read takes no lock", "internal/store/read.go",
  "\tunlock, _, lerr := lockCounted(f, budget)\n\tif lerr == nil {\n\t\tdefer unlock()\n\t}\n",
  "\tlerr := errors.New(\"no lock taken\")\n", "TestRubricPERF1_DigestWaitsOutAHookHold")
m("perf-1 each file of the read waits a budget of its own", "internal/store/read.go",
  "\t\tleft := max(0, deadline.Sub(pollNow()))\n", "\t\tleft := max(budget, deadline.Sub(pollNow()))\n",
  "TestRubricPERF1_TheBudgetCoversTheWholeReadOnAnExactClock")
# The read's deadline and each file's wait are on pollNow, the clock its lock
# waits poll on, so the exact clock drives the whole read (GATE-BUDGET-SEAM1).
m("perf-1 the read's deadline is on the wall clock", "internal/store/read.go",
  "\tdeadline := pollNow().Add(budget)\n", "\tdeadline := time.Now().Add(budget)\n",
  "TestRubricPERF1_TheBudgetCoversTheWholeReadOnAnExactClock")
m("perf-1 a file's wait is read from the wall clock", "internal/store/read.go",
  "\t\tleft := max(0, deadline.Sub(pollNow()))\n", "\t\tleft := max(0, time.Until(deadline))\n",
  "TestRubricPERF1_TheBudgetCoversTheWholeReadOnAnExactClock")
m("perf-1 the last polling sleep runs past the deadline", "internal/store/lock_unix.go",
  "\t\tpollSleep(min(backoff, left))\n", "\t\tpollSleep(backoff)\n",
  "TestRubricPERF1_AGiveUpLandsAtItsBudgetOnAnExactClock")
