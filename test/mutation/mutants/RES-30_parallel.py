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
# A flush_failed marker (REC-34) is counted in Spilled too, so it already
# fails the burst there; this is its own named check, so a real flush
# failure under load does not read as an ordinary lock timeout.
m("RES-30 a flush_failed marker is not a loss", "test/perf/gate.go",
  "\tif b.FlushFailed > 0 {\n\t\tout = append(out, fmt.Sprintf(\"%d flush_failed markers, want none: a hook's group-commit flush failed under the burst's load\", b.FlushFailed))\n\t}\n",
  "", "TestRES30_")
m("RES-30 a record kind short of the burst passes", "test/perf/gate.go",
  "\t\tif c.got != b.Ways {", "\t\tif c.got > b.Ways {", "TestRES30_")
m("RES-30 a gap in the ordered stream passes", "test/perf/gate.go",
  "\t\tif s != int64(i+1) {", "\t\tif s < int64(i+1) {", "TestRES30_")
m("RES-30 the footprint sum gets ten times its budget", "test/perf/gate.go",
  "\tif OverBudget(sum, budgetMiB, tolerance) {", "\tif OverBudget(sum, 10*budgetMiB, tolerance) {", "TestRES30_")
m("RES-30 the footprint is read off the RSS line", "test/perf/gate.go",
  'const footprintLine = "peak memory footprint"', 'const footprintLine = "maximum resident set size"', "TestRES30_")
# The per-process rule, restated from measurement (ruling 2026-10-13): the
# largest pair peak against 1.25 times the median of 25 solo pairs, with no
# tolerance on the reference machine and the platform's on a CI runner, whose
# ratio is unmeasured. Judging the median instead is the reading that let a
# burst whose largest process was 1.21x pass a 1.1 rule unseen (P09-F3).
m("RES-30 the per-process rule allows ten times the 1-way figure", "test/perf/gate.go",
  "\tcase OverBudget(largest, PerProcessPeakRatio*b.OneWayPeakMiB, ratioTolerance):",
  "\tcase OverBudget(largest, 10*PerProcessPeakRatio*b.OneWayPeakMiB, ratioTolerance):", "TestRES30_")
m("RES-30 the per-process ratio loosened to 1.5", "test/perf/gate.go",
  "const PerProcessPeakRatio = 1.25", "const PerProcessPeakRatio = 1.5", "TestRES30_")
m("RES-30 the 1-way figure back to nine samples", "test/perf/gate.go",
  "const OneWaySamples = 25", "const OneWaySamples = 9", "TestRES30_")
m("RES-30 the per-process rule judges no process", "test/perf/gate.go",
  "\tlargest := b.LargestMiB()\n", "\tlargest := 0.0\n", "TestRES30_")
m("RES-30 the per-process rule judges the median process", "test/perf/gate.go",
  "\tlargest := b.LargestMiB()\n", "\tlargest, _ := Median(b.PairPeakMiB)\n", "TestRES30_")
m("RES-30 the largest process is the first one", "test/perf/gate.go",
  "\t\tlargest = max(largest, m)\n", "\t\tlargest = m\n\t\tbreak\n", "TestRES30_")
m("RES-30 the ratio takes the sum's tolerance on the reference machine", "test/perf/gate.go",
  "\tcase OverBudget(largest, PerProcessPeakRatio*b.OneWayPeakMiB, ratioTolerance):",
  "\tcase OverBudget(largest, PerProcessPeakRatio*b.OneWayPeakMiB, tolerance):", "TestRES30_")
m("RES-30 the ratio ignores the runner tolerance", "test/perf/gate.go",
  "\tcase OverBudget(largest, PerProcessPeakRatio*b.OneWayPeakMiB, ratioTolerance):",
  "\tcase OverBudget(largest, PerProcessPeakRatio*b.OneWayPeakMiB, 0):", "TestRES30_")
# Where the ratio's tolerance applies (P09-F3, round 2): none on the reference
# machine, where 1.25x was measured, and the platform's runner tolerance only on
# GitHub Actions. The selector reads the environment through an injected getenv
# so this untagged sweep judges it; always granting the runner tolerance loosens
# the reference gate to about 1.44x, the reading that hides a regression.
m("RES-30 the ratio always gets the runner tolerance", "test/perf/gate.go",
  "\tif getenv(\"GITHUB_ACTIONS\") == \"true\" {\n\t\treturn platformTolerance\n\t}\n\treturn 0\n",
  "\treturn platformTolerance\n", "TestRES30_")
m("RES-30 the ratio never gets the runner tolerance", "test/perf/gate.go",
  "\tif getenv(\"GITHUB_ACTIONS\") == \"true\" {\n\t\treturn platformTolerance\n\t}\n\treturn 0\n",
  "\treturn 0\n", "TestRES30_")
m("RES-30 any GITHUB_ACTIONS value is a runner", "test/perf/gate.go",
  "\tif getenv(\"GITHUB_ACTIONS\") == \"true\" {", "\tif getenv(\"GITHUB_ACTIONS\") != \"\" {", "TestRES30_")
