# RES-36 (P-12): the per-process cold allocation is the sum of
# cold_alloc_bytes_max and RES-2's init budget; dropping either half or
# reporting a nonzero figure for an unmeasured entry is the regression.
m("RES-36 PerProcessColdAllocBytes drops the init budget", "test/perf/gate.go",
  "\treturn e.ColdAllocBytesMax + InitBudgetBytes\n",
  "\treturn e.ColdAllocBytesMax\n", "TestRES36o_")
m("RES-36 PerProcessColdAllocBytes claims a figure for an unmeasured entry", "test/perf/gate.go",
  "\tif e.ColdAllocBytesMax <= 0 {\n\t\treturn 0\n\t}\n",
  "", "TestRES36o_")
