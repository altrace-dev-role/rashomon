m("H-110 the exported lock's release leaves it held", "internal/store/lock_unix.go",
  "return func() { unlockFile(f) }, waited, nil", "return func() {}, waited, nil", "TestH110_")
m("H-110 every lock error reads as contention", "internal/store/lock_unix.go",
  "if !errors.Is(err, syscall.EWOULDBLOCK) {", "if !errors.Is(err, err) {", "TestH110_")
m("H-110 the exported lock is shared, not exclusive", "internal/store/lock_unix.go",
  "syscall.LOCK_EX|syscall.LOCK_NB", "syscall.LOCK_SH|syscall.LOCK_NB", "TestH110_")
m("H-110 the exported lock ignores the caller's budget", "internal/store/lock_unix.go",
  "deadline := start.Add(budget)", "deadline := start.Add(lockBudget)", "TestH110_")
m("H-110 the exported lock gives up without waiting out the budget", "internal/store/lock_unix.go",
  "\t\tif left <= 0 {", "\t\tif left > 0 {", "TestH110_")
# A budget of 0 is a try-lock. Reading it as "wait until free", as some lock
# APIs do, hangs a caller that meant to skip.
m("H-110 a budget of 0 waits for the lock", "internal/store/lock_unix.go",
  "\t\tif left <= 0 {\n", "\t\tif budget > 0 && left <= 0 {\n",
  "TestH110_ABudgetOfZeroIsATryLock")
