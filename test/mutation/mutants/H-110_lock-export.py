m("H-110 the exported lock's release leaves it held", "internal/store/lock_unix.go",
  "return func() { _ = syscall.Flock(fd, syscall.LOCK_UN) }, nil", "return func() {}, nil", "TestH110_")
m("H-110 every lock error reads as contention", "internal/store/lock_unix.go",
  "if !errors.Is(err, syscall.EWOULDBLOCK) {", "if !errors.Is(err, err) {", "TestH110_")
m("H-110 the exported lock is shared, not exclusive", "internal/store/lock_unix.go",
  "syscall.LOCK_EX|syscall.LOCK_NB", "syscall.LOCK_SH|syscall.LOCK_NB", "TestH110_")
m("H-110 the exported lock ignores the caller's budget", "internal/store/lock_unix.go",
  "deadline := time.Now().Add(budget)", "deadline := time.Now().Add(lockBudget)", "TestH110_")
m("H-110 the exported lock gives up without waiting out the budget", "internal/store/lock_unix.go",
  "\t\tif time.Now().After(deadline) {", "\t\tif !time.Now().After(deadline) {", "TestH110_")
