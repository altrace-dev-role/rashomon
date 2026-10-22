# RES-36: the rows are one platform's figures too (long $TMPDIR paths on
# darwin skew B/op against the same benchmark on linux), so CheckPlatform
# refuses any other GOOS/GOARCH before comparing anything, the same way
# CheckToolchain refuses a foreign toolchain. Decode's DisallowUnknownFields
# is the only thing standing between a misspelled key and a silently unfilled
# override. Parse's betterRun keeps one whole run, never a blend of two.
m("RES-36 a platform mismatch passes the check", "internal/budget/budgets.go",
  "\tif running != b.BenchmarkPlatform && len(b.Platforms[running].Benchmarks) == 0 {\n",
  "\tif running != b.BenchmarkPlatform && running == \"\" {\n", "TestRES36l_")
m("RES-36 a budgets file naming no platform passes the check", "internal/budget/budgets.go",
  '\tif b.BenchmarkPlatform == "" {\n\t\treturn errors.New(', '\tif b.BenchmarkPlatform == "" {\n\t\treturn nil\n\t\treturn errors.New(', "TestRES36l_")
m("RES-36 Decode accepts an unknown field", "internal/budget/budgets.go",
  "\tdec.DisallowUnknownFields()\n", "", "TestRES36m_")
m("RES-36 benchgate compares under any platform", "scripts/benchgate/main.go",
  "\tif err := b.CheckPlatform(platform); err != nil {\n\t\treturn fail(err)\n\t}\n", "", "TestRES36n_")
m("RES-36 Parse blends the lower B/op of one run with the lower allocs/op of another", "scripts/benchgate/main.go",
  "\tif a.BytesPerOp != b.BytesPerOp {\n\t\treturn a.BytesPerOp < b.BytesPerOp\n\t}\n\treturn a.AllocsPerOp < b.AllocsPerOp\n}\n",
  "\treturn a.AllocsPerOp < b.AllocsPerOp\n}\n", "TestRES36b_")

# RES-36p: a platform other than the reference has nothing to compare
# against until it measures its own rows (`benchgate -measure`); once it
# has, BenchmarksFor must read that platform's own rows, and run() must
# actually use BenchmarksFor rather than the reference rows unconditionally.
# Measure's printed row commits the measurement itself as the max, matching
# every other row in the file ("a row's max equals its measurement"), and
# -measure refuses to print nothing was there to measure.
m("RES-36p BenchmarksFor ignores a platform's own override", "internal/budget/budgets.go",
  '\tif running != b.BenchmarkPlatform {\n\t\treturn b.Platforms[running].Benchmarks\n\t}\n\treturn b.Benchmarks\n',
  '\treturn b.Benchmarks\n', "TestRES36p_")
m("RES-36p run compares against the reference rows instead of BenchmarksFor", "scripts/benchgate/main.go",
  "\tbenchmarks := b.BenchmarksFor(platform)\n", "\tbenchmarks := b.Benchmarks\n", "TestRES36p_")
m("RES-36p Measure prints a max that is not the measurement", "scripts/benchgate/main.go",
  "\t\t\tBytesPerOpMax:       r.BytesPerOp,\n\t\t\tAllocsPerOpMax:      r.AllocsPerOp,\n",
  "\t\t\tBytesPerOpMax:       r.BytesPerOp * 2,\n\t\t\tAllocsPerOpMax:      r.AllocsPerOp,\n", "TestRES36q_")
m("RES-36p -measure passes with nothing parsed", "scripts/benchgate/main.go",
  '\tif len(results) == 0 {\n\t\tfmt.Fprintln(stderr, "benchgate: -measure read no benchmark output, so there is nothing to measure")\n\t\treturn 2\n\t}\n',
  "", "TestRES36q_")
