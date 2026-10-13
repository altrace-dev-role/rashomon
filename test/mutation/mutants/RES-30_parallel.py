# RES-30: 64 hook-and-post pairs on one session lose no record and stay within
# the summed footprint budget. The harness that measures it runs under the perf
# tag, which this sweep does not build; what the sweep can judge is the verdict
# the harness reaches and the footprint it reads, so those are what break here.
# Group commit itself, the mechanism the burst depends on, is REC-34's to break.
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
