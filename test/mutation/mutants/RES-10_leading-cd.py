# RES-10: the leading-cd fold stops at 64 steps, past which the start
# directory is unknown, and the masked-status walk holds no step past the
# same bound. Lifting either bound, or counting steps the cwd digest does not
# hold, is the regression.
m("RES-10 the hook folds every leading cd", "internal/hook/handle.go",
  "const maxLeadingCd = shape.MaxLeadingCd\n", "const maxLeadingCd = 1 << 30\n", "TestRES10_")
m("RES-10 the shared leading-cd bound is lifted", "internal/shape/shape.go",
  "const MaxLeadingCd = 64\n", "const MaxLeadingCd = 1 << 30\n", "TestRES10_")
m("RES-10 a runner past the fold is paired as though its directory were known", "internal/shape/shape.go",
  "\tif len(dirs) > MaxLeadingCd {\n\t\treturn 0\n\t}\n", "", "TestRES10_RunnerPastTheFoldIsNotPaired")
