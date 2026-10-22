# RES-36: a budgets file with no benchmark rows gates nothing; the gate
# exits 2 rather than reporting zero benchmarks within budget.
m("RES-36 benchgate passes a budgets file with no benchmark rows", "scripts/benchgate/main.go",
  "\tif len(benchmarks) == 0 {\n", "\tif len(benchmarks) < 0 {\n", "TestRES36k_")
