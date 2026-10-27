# RES-36 (P-12): the cold-alloc gate must stay wired into both the CI
# -run patterns and the RASHOMON_PERF_REQUIRE_ALL accounting, and a 0
# cold_alloc_bytes_max must not silently turn the gate off for a row that
# has a cold harness (BenchmarkRunHook/*, BenchmarkRunPost/*).
m("RES-36 a hook row's cold figure reads as not-yet-measured", "test/perf/budgets.json",
  '"cold_alloc_bytes_max": 171136,', '"cold_alloc_bytes_max": 0,', "TestRES36m_")
m("RES-36 Ungated stops owing a measured entry's cold gate", "test/perf/runs.go",
  "\t\tif e.ColdAllocBytesMax > 0 {\n\t\t\towed = append(owed, ColdGate(e.ID))\n\t\t}\n",
  "", "TestRES36n_")
