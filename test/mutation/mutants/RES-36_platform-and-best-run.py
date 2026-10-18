# RES-36: the rows are one platform's figures too (long $TMPDIR paths on
# darwin skew B/op against the same benchmark on linux), so CheckPlatform
# refuses any other GOOS/GOARCH before comparing anything, the same way
# CheckToolchain refuses a foreign toolchain. Decode's DisallowUnknownFields
# is the only thing standing between a misspelled key and a silently unfilled
# override. Parse's betterRun keeps one whole run, never a blend of two.
m("RES-36 a platform mismatch passes the check", "internal/budget/budgets.go",
  "\tif running != b.BenchmarkPlatform {\n", "\tif running != b.BenchmarkPlatform && running == \"\" {\n", "TestRES36l_")
m("RES-36 a budgets file naming no platform passes the check", "internal/budget/budgets.go",
  '\tif b.BenchmarkPlatform == "" {\n\t\treturn errors.New(', '\tif b.BenchmarkPlatform == "" {\n\t\treturn nil\n\t\treturn errors.New(', "TestRES36l_")
m("RES-36 Decode accepts an unknown field", "internal/budget/budgets.go",
  "\tdec.DisallowUnknownFields()\n", "", "TestRES36m_")
m("RES-36 benchgate compares under any platform", "scripts/benchgate/main.go",
  "\tif err := b.CheckPlatform(platform); err != nil {\n\t\treturn fail(err)\n\t}\n", "", "TestRES36n_")
m("RES-36 Parse blends the lower B/op of one run with the lower allocs/op of another", "scripts/benchgate/main.go",
  "\tif a.BytesPerOp != b.BytesPerOp {\n\t\treturn a.BytesPerOp < b.BytesPerOp\n\t}\n\treturn a.AllocsPerOp < b.AllocsPerOp\n}\n",
  "\treturn a.AllocsPerOp < b.AllocsPerOp\n}\n", "TestRES36b_")
