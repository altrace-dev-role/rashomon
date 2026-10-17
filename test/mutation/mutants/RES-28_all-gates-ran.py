# RES-28: `go test` passes a run in which every gate skipped; a required run
# names every owed gate that did not run, the init budget included.
m("RES-28 a skipped gate is not reported", "test/perf/runs.go",
  "\t\tif !ran[g] {\n", "\t\tif !ran[g] && false {\n", "TestRES28m_")
m("RES-28 the init budget is not owed", "test/perf/runs.go",
  "\towed := []string{InitGate}\n", "\towed := []string{}\n", "TestRES28m_")
# Settle decides the run's code; the harness must hand it a required run.
m("RES-28 a required run that skipped a gate passes", "test/perf/runs.go",
  "\tif !requireAll || code != 0 {\n", "\tif true {\n", "TestRES28n_")
m("RES-28 the harness never requires a whole run", "test/perf/rss_test.go",
  'return Settle(m.Run(), os.Getenv(RequireAllEnv) == "1", b, ran)',
  'return Settle(m.Run(), false, b, ran)', "TestRES28o_")
