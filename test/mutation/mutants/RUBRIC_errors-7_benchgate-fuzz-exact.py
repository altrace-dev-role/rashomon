# errors-7: FuzzParse holds an appended result line to its exact figures, so
# a Parse that read every figure as zero, or kept the worse of two runs, is
# caught by the fuzz target's own seeds.
m("errors-7  benchgate reads every figure as zero", "scripts/benchgate/main.go",
  "\t\trun := Result{bytesPerOp, allocs}\n", "\t\trun := Result{0 * bytesPerOp, 0 * allocs}\n", "TestRubricErrors7_ParseSeeds")
m("errors-7  benchgate keeps the worse of two runs", "scripts/benchgate/main.go",
  "\t\tif prev, ok := best[name]; !ok || betterRun(run, prev) {\n",
  "\t\tif prev, ok := best[name]; !ok || betterRun(prev, run) {\n", "TestRubricErrors7_ParseSeeds")
