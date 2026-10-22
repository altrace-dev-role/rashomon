# baseline.ErrTooManyHosts and store.ErrLockTimeout are named so a caller can
# act on them differently from a corrupt or unreadable file, which stays the
# generic message on purpose (the error text carries the baseline's path).
# Collapsing either named case back into the generic one, or making the
# generic one echo the error text, is the regression.
m("a too-many-hosts baseline error reads as the generic message", "internal/report/gather/gather.go",
  "\tcase errors.Is(err, baseline.ErrTooManyHosts):\n\t\treturn \"this session named more hosts than a project baseline can hold, so novelty could not be updated this time\"\n",
  "",
  "TestBaselineErrorReason_DistinctCausesDistinctMessages|TestNovelty_TooManyHostsSaysSoNotTheGenericMessage")
m("a locked baseline error reads as the generic message", "internal/report/gather/gather.go",
  "\tcase errors.Is(err, store.ErrLockTimeout):\n\t\treturn \"the project baseline was locked by another render, so novelty could not be checked this time\"\n",
  "",
  "TestBaselineErrorReason_DistinctCausesDistinctMessages")
m("the generic baseline error repeats the error text", "internal/report/gather/gather.go",
  "\tdefault:\n\t\treturn \"the project baseline could not be read or written\"\n",
  "\tdefault:\n\t\treturn err.Error()\n",
  "TestBaselineErrorReason_DistinctCausesDistinctMessages")
