# docs-9 (card DOCS9r, D102): goroutines sharing one undeferred Store get a
# total order with every landed seq handed out once, because each ordered
# append takes its records file's lock. Without the lock, eight goroutines
# read and advance one seq counter at once; a duplicate, a gap or a torn line
# fails the test's distinct, range or count check, or ReadRun.
m("docs-9 an ordered append takes no lock", "internal/store/store.go",
  "\tunlock, err := s.lockAppend(f, s.lockWait(lockBudget))\n", "\tunlock, err := func() {}, error(nil)\n",
  "TestRubricDOCS9_OneStoreIsSafeAcrossGoroutines")
