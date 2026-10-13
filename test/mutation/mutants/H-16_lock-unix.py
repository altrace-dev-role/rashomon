m("H-16 append lock removed", "internal/store/store.go",
  "\tunlock, err := lockCounted(f, s.lockWait(lockBudget))\n\tif err != nil {\n\t\treturn err\n\t}\n\tdefer unlock()\n\treturn s.writeOrderedLocked(f, dir, build)",
  "\treturn s.writeOrderedLocked(f, dir, build)", "TestH16_")
m("H-16 the lock is shared, not exclusive", "internal/store/lock_unix.go",
  "syscall.LOCK_EX|syscall.LOCK_NB", "syscall.LOCK_SH|syscall.LOCK_NB", "TestLockFileIsExclusiveAndBounded")
m("H-16 the lock ignores the caller's budget", "internal/store/lock_unix.go",
  "deadline := time.Now().Add(budget)", "deadline := time.Now().Add(lockBudget)", "TestLockFileIsExclusiveAndBounded")
m("H-16 the lock gives up without waiting out the budget", "internal/store/lock_unix.go",
  "\t\tif time.Now().After(deadline) {", "\t\tif !time.Now().After(deadline) {", "TestLockFileIsExclusiveAndBounded")
