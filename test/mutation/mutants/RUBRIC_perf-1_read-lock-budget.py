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
  "\tif unlock, lerr := lockCounted(f, budget); lerr == nil {\n\t\tdefer unlock()\n\t}\n\treturn scanLines(f, fn)\n",
  "\treturn scanLines(f, fn)\n", "TestRubricPERF1_DigestWaitsOutAHookHold")
m("perf-1 each file of the read waits a budget of its own", "internal/store/read.go",
  "\t\tleft := max(0, time.Until(deadline))\n", "\t\tleft := max(budget, time.Until(deadline))\n",
  "TestRubricPERF1_TheBudgetCoversTheWholeRead")
m("perf-1 the last polling sleep runs past the deadline", "internal/store/lock_unix.go",
  "\t\ttime.Sleep(min(backoff, left))\n", "\t\ttime.Sleep(backoff)\n",
  "TestRubricPERF1_AGiveUpLandsAtItsBudget")
