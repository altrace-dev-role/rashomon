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
