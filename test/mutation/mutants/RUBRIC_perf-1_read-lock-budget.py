# perf-1: digest's per-file reader budget waits out a hook's measured hold
# (BenchmarkOrderedAppendHold) rather than the sub-millisecond one it assumed.
m("perf-1 the reader gives up inside a hook's hold", "internal/digest/digest.go",
  "const readLockBudget = 10 * time.Millisecond\n", "const readLockBudget = 5 * time.Millisecond\n",
  "TestRubricPERF1_DigestWaitsOutAHookHold")
