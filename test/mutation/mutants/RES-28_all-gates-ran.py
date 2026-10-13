# RES-28: `go test` passes a run in which every gate skipped; a required run
# names every owed gate that did not run, the init budget included.
m("RES-28 a skipped gate is not reported", "test/perf/runs.go",
  "\t\tif !ran[g] {\n", "\t\tif !ran[g] && false {\n", "TestRES28m_")
m("RES-28 the init budget is not owed", "test/perf/runs.go",
  "\towed := []string{InitGate}\n", "\towed := []string{}\n", "TestRES28m_")
