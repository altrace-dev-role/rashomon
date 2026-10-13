# RES-10: the leading-cd fold stops at 64 steps, past which the start
# directory is unknown, and the masked-status walk holds no step past the
# same bound. Lifting either bound, or counting steps the cwd digest does not
# hold, or giving up the leading steps of a line cut at the token bound, is
# the regression. The shared bound is lifted to 128, not further: past the
# token bound's arithmetic the package does not compile, by design.
m("RES-10 the hook folds every leading cd", "internal/hook/handle.go",
  "const maxLeadingCd = shape.MaxLeadingCd\n", "const maxLeadingCd = 1 << 30\n", "TestRES10_")
m("RES-10 the shared leading-cd bound is lifted", "internal/shape/leading.go",
  "const MaxLeadingCd = 64\n", "const MaxLeadingCd = 128\n", "TestRES10_")
m("RES-10 a runner past the fold is paired as though its directory were known", "internal/shape/leading.go",
  "\tif len(dirs) > MaxLeadingCd {\n\t\treturn 0\n\t}\n", "", "TestRES10_RunnerPastTheFoldIsNotPaired")
m("RES-10 a line cut at the token bound gives up its leading cds", "internal/shape/leading.go",
  "\tif err != nil && err != errTokenBound {\n\t\treturn nil, false\n\t}\n",
  "\tif err != nil {\n\t\treturn nil, false\n\t}\n", "TestRES10_")
