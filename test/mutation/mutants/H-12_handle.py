m("H-12 SIGTERM ignored on the close path", "internal/hook/handle.go", "\tcase sig.Delivered():\n\t\toutcome, reason = store.OutcomeSignal", "\tcase false && sig.Delivered():\n\t\toutcome, reason = store.OutcomeSignal", "TestH12_SIGTERM")
m("H-12 SIGHUP and SIGQUIT are not watched", "internal/hook/signals.go",
  "signal.Notify(s.ch, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)",
  "signal.Notify(s.ch, os.Interrupt, syscall.SIGTERM)", "TestH12b_")
