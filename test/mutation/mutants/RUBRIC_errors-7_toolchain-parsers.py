# errors-7: the readers of Go-toolchain and OS output the perf gates decide
# with are fuzzed (rubric fix list entry 23). The first target found an init
# line of ten fields indexed at its eleventh; the test names that line.
m("errors-7  a ten-field init line is indexed past its end", "test/perf/gate.go",
  "\t\t\tif len(f) < 11 || f[8] != \"bytes,\" || f[10] != \"allocs\" {",
  "\t\t\tif len(f) < 10 || f[8] != \"bytes,\" || f[10] != \"allocs\" {", "TestRubricErrors7_")
# A malformed first footprint line fails the read (round 3: the fuzz oracle
# disagreed with the parser over which line is the footprint line). Reading
# past it to a later line is the choice the test pins against.
m("errors-7  a malformed footprint line is read past", "test/perf/gate.go",
  "\t\t\tn, err := strconv.ParseInt(f[0], 10, 64)\n\t\t\treturn n, err == nil\n",
  "\t\t\tn, err := strconv.ParseInt(f[0], 10, 64)\n\t\t\tif err != nil {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\treturn n, true\n", "TestRubricErrors7_")
