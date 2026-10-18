# errors-7: FuzzParse holds an appended result line to its exact figures, so
# a Parse that read every figure as zero, or kept the higher of two runs, is
# caught by the fuzz target's own seeds.
m("errors-7  benchgate reads every figure as zero", "scripts/benchgate/main.go",
  "\t\tout[name] = Result{bytesPerOp, allocs}\n", "\t\tout[name] = Result{}\n", "TestRubricErrors7_ParseSeeds")
m("errors-7  benchgate keeps the lower B/op of two runs only when allocs agree", "scripts/benchgate/main.go",
  "\t\t\tif allocs > prev.AllocsPerOp {\n\t\t\t\tallocs = prev.AllocsPerOp\n\t\t\t}",
  "\t\t\tif allocs > prev.AllocsPerOp {\n\t\t\t\tallocs, bytesPerOp = prev.AllocsPerOp, prev.BytesPerOp\n\t\t\t}", "TestRubricErrors7_ParseSeeds")
