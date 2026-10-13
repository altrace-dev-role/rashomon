# errors-7: the readers of Go-toolchain and OS output the perf gates decide
# with are fuzzed (rubric fix list entry 23). The first target found an init
# line of ten fields indexed at its eleventh; the test names that line.
m("errors-7  a ten-field init line is indexed past its end", "test/perf/gate.go",
  "\t\t\tif len(f) < 11 || f[8] != \"bytes,\" || f[10] != \"allocs\" {",
  "\t\t\tif len(f) < 10 || f[8] != \"bytes,\" || f[10] != \"allocs\" {", "TestRubricErrors7_")
