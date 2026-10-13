# RES-30: 64 hook-and-post pairs on one session lose no record and stay within
# the summed footprint budget. The harness that measures it runs under the perf
# tag, which this sweep does not build; what the sweep can judge is the verdict
# the harness reaches and the footprint it reads, so those are what break here.
# Group commit itself, the mechanism the burst depends on, is REC-34's to break.
# The archive names RES-30's mutant as "lower the 64-way footprint budget to the
# 1-way figure"; it lives in the perf-tagged harness, which no sweep builds, so
# it was proved red once by hand (2026-10-13) and the verdict logic stands in
# for it here.
m("RES-30 a spilled line is not a loss", "test/perf/gate.go",
  "\tif b.Spilled > 0 || b.LockTimeouts > 0 {", "\tif b.LockTimeouts > 0 {", "TestRES30_")
m("RES-30 a lock timeout is not a loss", "test/perf/gate.go",
  "\tif b.Spilled > 0 || b.LockTimeouts > 0 {", "\tif b.Spilled > 0 {", "TestRES30_")
m("RES-30 a record kind short of the burst passes", "test/perf/gate.go",
  "\t\tif c.got != b.Ways {", "\t\tif c.got > b.Ways {", "TestRES30_")
m("RES-30 a gap in the ordered stream passes", "test/perf/gate.go",
  "\t\tif s != int64(i+1) {", "\t\tif s < int64(i+1) {", "TestRES30_")
m("RES-30 the footprint sum gets ten times its budget", "test/perf/gate.go",
  "\tif OverBudget(sum, budgetMiB, tolerance) {", "\tif OverBudget(sum, 10*budgetMiB, tolerance) {", "TestRES30_")
m("RES-30 the footprint is read off the RSS line", "test/perf/gate.go",
  'const footprintLine = "peak memory footprint"', 'const footprintLine = "maximum resident set size"', "TestRES30_")
m("RES-30 the per-process rule allows ten times the 1-way figure", "test/perf/gate.go",
  "\tcase largest > PerProcessPeakRatio*b.OneWayPeakMiB:", "\tcase largest > 10*PerProcessPeakRatio*b.OneWayPeakMiB:", "TestRES30_")
m("RES-30 the per-process ratio loosened to 1.5", "test/perf/gate.go",
  "const PerProcessPeakRatio = 1.1", "const PerProcessPeakRatio = 1.5", "TestRES30_")
m("RES-30 the per-process rule judges no process", "test/perf/gate.go",
  "\tlargest := b.LargestMiB()\n", "\tlargest := 0.0\n", "TestRES30_")
# The rule is on every process (the RES-30 row: "per-process peak at 64-way is
# <= 1.1x the 1-way figure"). Judging the median instead is the reading that let
# a burst whose largest process was 1.13x the 1-way figure pass (P09-F3).
m("RES-30 the per-process rule judges the median process", "test/perf/gate.go",
  "\tlargest := b.LargestMiB()\n", "\tlargest, _ := Median(b.PairPeakMiB)\n", "TestRES30_")
m("RES-30 the largest process is the first one", "test/perf/gate.go",
  "\t\tlargest = max(largest, m)\n", "\t\tlargest = m\n\t\tbreak\n", "TestRES30_")
# hook and post run on one P (cmd/rashomon/procs.go). With a P per core the
# largest process of a 64-way burst measured 1.11 to 1.21 times the 1-way
# figure, which the rule above fails; the perf harness cannot run in the sweep,
# so the unit test that sees the setting is what goes red.
m("RES-30 the hook runs on every core", "cmd/rashomon/main.go",
  "\t\toneProc()\n\t\tsig := hook.WatchSignals()\n\t\tdefer sig.Stop()\n\n\t\tif checkPaused(args, stdin, store.PhaseCall, stderr) {",
  "\t\tsig := hook.WatchSignals()\n\t\tdefer sig.Stop()\n\n\t\tif checkPaused(args, stdin, store.PhaseCall, stderr) {", "TestRES30_")
m("RES-30 post runs on every core", "cmd/rashomon/main.go",
  "\t\toneProc()\n\t\tsig := hook.WatchSignals()\n\t\tdefer sig.Stop()\n\n\t\tif checkPaused(args, stdin, store.PhasePost, stderr) {",
  "\t\tsig := hook.WatchSignals()\n\t\tdefer sig.Stop()\n\n\t\tif checkPaused(args, stdin, store.PhasePost, stderr) {", "TestRES30_")
m("RES-30 oneProc keeps every P", "cmd/rashomon/procs.go",
  "func oneProc() { runtime.GOMAXPROCS(1) }", "func oneProc() { runtime.GOMAXPROCS(0) }", "TestRES30_")
