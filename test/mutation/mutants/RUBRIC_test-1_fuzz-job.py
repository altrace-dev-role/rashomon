# test-1: the CI Fuzz job bounds minimization, and its floor is the number of
# fuzz targets the tree holds (rubric fix list entry 23). The tests read
# ci.yaml by its lines.
m("test-1  fuzz minimization left at its one-minute default", ".github/workflows/ci.yaml",
  " -fuzztime 30s -fuzzminimizetime 10x; then", " -fuzztime 30s; then", "TestRubricTest1_")
m("test-1  the fuzz job's floor is left behind the tree", ".github/workflows/ci.yaml",
  "          test \"$found\" -ge 15\n", "          test \"$found\" -ge 10\n", "TestRubricTest1_")
