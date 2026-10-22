m("noteLockWait waits its full spillBudget instead of the shared deadline", "internal/store/lockwait.go",
  "\tunlock, _, err := lockCounted(f, s.lockWait(spillBudget))\n",
  "\tunlock, _, err := lockCounted(f, spillBudget)\n",
  "TestNoteLockWaitRespectsTheSharedDeadline")
