# RES-28: every entry row is measured on the reference machine, on the store
# its budget names; a recap row on an empty store measures the no-store early
# return (5.2 MiB, below the process floor) and passes anything.
m("RES-28 an entry row goes back to a derived figure", "test/perf/budgets.json",
  '"maxrss_mib_p50": 5.92, "maxrss_mib_p95": 6.5, "parent_maxrss_mib_p50": null, "wall_ms_p50": 6, "wall_ms_p95": 12, "basis": "measured ',
  '"maxrss_mib_p50": 6, "maxrss_mib_p95": 6.5, "parent_maxrss_mib_p50": null, "wall_ms_p50": 6, "wall_ms_p95": 12, "basis": "derived ', "TestRES28k_")
m("RES-28 a recap row is measured unseeded", "test/perf/runs.go",
  "\t\treturn Seed{Calls: n}, nil\n", "\t\treturn Seed{}, nil\n", "TestRES28k_")
m("RES-28 the harness measures probe end on an unstarted session", "test/perf/rss_test.go",
  "\t\t\tr.seed(t, sd)\n", "\t\t\tr.seed(t, Seed{Calls: sd.Calls})\n", "TestRES28o_")
