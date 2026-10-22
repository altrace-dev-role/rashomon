# RES-36: the rows are one toolchain's figures (go1.25.8 makes 21,660
# allocations per op on BenchmarkBuild/188, go1.27.2 6,832), so the gate and
# the harness refuse any other toolchain before comparing anything.
m("RES-36 benchgate compares under any toolchain", "scripts/benchgate/main.go",
  "\tif err := b.CheckToolchain(goVersion); err != nil {\n\t\treturn fail(err)\n\t}\n",
  "", "TestRES36g_")
m("RES-36 a toolchain mismatch passes the check", "internal/budget/budgets.go",
  "\tif running != b.GoVersion {\n", "\tif running != b.GoVersion && running == \"\" {\n", "TestRES36g_")
m("RES-36 a budgets file naming no toolchain passes the check", "internal/budget/budgets.go",
  '\tif b.GoVersion == "" {\n\t\treturn errors.New(', '\tif b.GoVersion == "" {\n\t\treturn nil\n\t\treturn errors.New(', "TestRES36g_")
# The perf harness's half: Preflight checks the toolchain, and buildAndRun
# (perf-tagged, so pinned by reading its source) calls it with
# runtime.Version() and stops on its error before building anything.
m("RES-36 the harness preflight reads budgets.json without checking the toolchain", "test/perf/runs.go",
  "\tif err := b.CheckToolchain(running); err != nil {\n\t\treturn b, err\n\t}\n", "", "TestRES36i_")
m("RES-36 the perf harness compares under any toolchain", "test/perf/rss_test.go",
  'b, err := Preflight(filepath.Join(root, "test", "perf", "budgets.json"), runtime.Version())',
  'b, err := Preflight(filepath.Join(root, "test", "perf", "budgets.json"), "go1.27.3")', "TestRES36j_")
m("RES-36 the perf harness builds and runs after its preflight failed", "test/perf/rss_test.go",
  '"budgets.json"), runtime.Version())\n\tif err != nil {\n\t\treturn 2, err\n\t}\n',
  '"budgets.json"), runtime.Version())\n', "TestRES36j_")
