# RES-36: the budgets file is the one source: the table compiled into the
# binary equals it, every benchmark has a row, and the bench gate fails
# closed on a budgeted benchmark that did not run or a row with no figures.
m("RES-36 the generated table drifts from budgets.json", "internal/budget/budgets_gen.go",
  '\\"bytes_per_call_max\\":6000}', '\\"bytes_per_call_max\\":6001}', "TestRES36_")
m("RES-36 the bench gate ignores a budgeted benchmark that did not run", "scripts/benchgate/main.go",
  '\t\t\tproblems = append(problems, fmt.Sprintf("%s: budgeted but did not run", b.Name))\n',
  '', "TestRES36c_")
m("RES-36 the bench gate parses a benchmark without allocation figures as gateable", "scripts/benchgate/main.go",
  '\t\tif m == nil {\n\t\t\treturn nil, fmt.Errorf("benchmark line without allocation figures: %q", ln)\n\t\t}\n',
  '\t\tif m == nil {\n\t\t\tcontinue\n\t\t}\n', "TestRES36b_")
m("RES-36 a benchmark loses its budget row", "test/perf/budgets.json",
  '    {"name": "BenchmarkDerive/edit_50k", "measured_bytes_per_op": 63464, "measured_allocs_per_op": 28, "bytes_per_op_max": 63464, "allocs_per_op_max": 28},\n',
  '', "TestRES36_")
m("RES-36 a benchmark row hides a margin above its measurement", "test/perf/budgets.json",
  '"measured_bytes_per_op": 2656, "measured_allocs_per_op": 33, "bytes_per_op_max": 2656, "allocs_per_op_max": 33}',
  '"measured_bytes_per_op": 2656, "measured_allocs_per_op": 33, "bytes_per_op_max": 2921, "allocs_per_op_max": 36}', "TestRES36d_")
