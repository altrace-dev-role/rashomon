# RES-26: a soft deadline shares the parent's own start -- the hard-stop
# Clock the supervised Child already carries, for a supervised entry, or the
# process's own start, for an unsupervised one. Each break below makes the
# soft Clock restart at the moment it is built instead of sharing that start.
m("RES-26 Clock.Soft restarts the clock instead of keeping the shared start",
  "internal/budget/budget.go",
  "func (c Clock) Soft(d time.Duration) Clock { return Clock{start: c.start, soft: d} }\n",
  "func (c Clock) Soft(d time.Duration) Clock { return Clock{start: time.Now(), soft: d} }\n",
  "TestClockSoftKeepsStartChangesBudget")
m("RES-26 a supervised child's SoftClock never shares the parent's start",
  "internal/supervise/run_unix.go",
  "\treturn c.clock.Soft(soft)\n",
  "\treturn budget.NewClock(time.Now(), soft)\n",
  "TestChildSoftClockSharesTheParentsStart")
m("RES-26 SoftClock never clamps the table budget to the child's hard stop",
  "internal/supervise/run_unix.go",
  "\tif margin < soft {\n\t\tsoft = margin\n\t}\n",
  "",
  "TestChildSoftClockClampsToHardStopMinusShortMargin")
m("RES-26 SoftClock's clamp margin goes negative instead of flooring at zero",
  "internal/supervise/run_unix.go",
  "\tif margin < 0 {\n\t\tmargin = 0\n\t}\n",
  "",
  "TestChildSoftClockFloorsAtZeroWhenHardStopIsInsideTheMargin")
