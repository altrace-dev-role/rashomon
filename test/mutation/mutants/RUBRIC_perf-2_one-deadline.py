# perf-2: one hook invocation's lock waits share one deadline, so a late
# declaration, its terminal and its coverage record cannot wait two seconds
# each against a five-second hook timeout.
P2 = "TestRubricPERF2_"
m("perf-2 every lock wait ignores the deadline", "internal/store/deadline.go",
  "\tif s.waitUntil.IsZero() {\n", "\tif true {\n", P2)
m("perf-2 an ordered append waits its whole budget", "internal/store/store.go",
  "\tunlock, err := s.lockAppend(f, s.lockWait(lockBudget))\n", "\tunlock, err := s.lockAppend(f, lockBudget)\n", P2)
m("perf-2 a coverage or spill append waits its whole budget", "internal/store/store.go",
  "\tunlock, err := s.lockAppend(f, s.lockWait(budget))\n", "\tunlock, err := s.lockAppend(f, budget)\n", P2)
m("perf-2 eviction waits its whole budget for the gaps lock", "internal/store/evict.go",
  "\tunlock, _, err := lockCounted(gf, s.lockWait(lockBudget))\n", "\tunlock, _, err := lockCounted(gf, lockBudget)\n", P2)
m("perf-2 a hook sets no deadline", "internal/hook/handle.go",
  "\th.st.LimitLockWaits(h.lockWaits)\n", "", P2)
m("perf-2 a post sets no deadline", "internal/hook/post.go",
  "\tp.st.LimitLockWaits(p.lockWaits)\n", "", P2)
m("perf-2 a probe sets no deadline", "internal/hook/probe.go",
  "\tst.LimitLockWaits(waits)\n", "", P2)
