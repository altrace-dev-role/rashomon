# RES-36: a benchmark that ran without a budget row is a failure: the static
# check reads names from source and misses a b.Run in a helper or a duplicate
# name Go runs as name#01, so the gate is what closes the gap.
m("RES-36 the bench gate passes a benchmark that ran without a row", "scripts/benchgate/main.go",
  '\t\t\tproblems = append(problems, fmt.Sprintf("%s: ran without a budget row", name))\n',
  '', "TestRES36c_")
