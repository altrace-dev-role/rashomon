# RES-36: a sub-benchmark's name must be read the way go test itself prints
# it -- a space rewritten to an underscore, a repeated sibling suffixed #01,
# #02 -- or budgets.json names a row benchgate's parsed output never
# produces, and TestRES36_BudgetsFileCoversEveryBenchmark passes on a name
# nothing runs under.
m("RES-36 renameRun leaves a space in the name", "test/perf/gates_test.go",
  'func renameRun(name string) string { return strings.ReplaceAll(name, " ", "_") }\n',
  "func renameRun(name string) string { return name }\n", "TestRES36o_")
m("RES-36 dedupeSiblings never suffixes a repeated sibling", "test/perf/gates_test.go",
  '\t\tc := seen[n]\n\t\tseen[n] = c + 1\n\t\tif c == 0 {\n\t\t\tout[i] = n\n\t\t\tcontinue\n\t\t}\n\t\tout[i] = fmt.Sprintf("%s#%02d", n, c)\n',
  '\t\tc := seen[n]\n\t\tseen[n] = c + 1\n\t\tout[i] = n\n\t\t_ = fmt.Sprintf("%s#%02d", n, c)\n', "TestRES36o_")
m("RES-36 unquoteLit accepts a literal strconv.Unquote cannot parse", "test/perf/gates_test.go",
  "func unquoteLit(lit *ast.BasicLit) (string, bool) {\n\ts, err := strconv.Unquote(lit.Value)\n\treturn s, err == nil\n}\n",
  "func unquoteLit(lit *ast.BasicLit) (string, bool) {\n\ts, _ := strconv.Unquote(lit.Value)\n\treturn s, true\n}\n", "TestRES36o_")
