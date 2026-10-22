# RES-36: cold_alloc_bytes_max (P-12) must never be negative and, where
# measured, must not read below the entry's own warm alloc_bytes_max row --
# a fresh process's one call pays everything the warmed benchmark measures
# plus the one-time cost the warm-up call exists to exclude. Caught by
# fabricated cases (TestRES36l_ColdAllocConsistentCatchesBadInputs), not by
# budgets.json's real rows, which never trip either branch today.
m("RES-36 cold_alloc_bytes_max is allowed to undercut the warm row", "test/perf/gate.go",
  "\tif coldAllocBytesMax > 0 && coldAllocBytesMax < allocBytesMax {\n",
  "\tif false {\n", "TestRES36l_")
m("RES-36 a negative cold_alloc_bytes_max passes", "test/perf/gate.go",
  "\tif coldAllocBytesMax < 0 {\n",
  "\tif coldAllocBytesMax < -1 {\n", "TestRES36l_")
