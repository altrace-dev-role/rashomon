# RES-10: the leading-cd fold stops at 64 steps, past which the start
# directory is unknown, and the masked-status walk passes over no step past
# the same bound (leadingFold). Lifting either bound, or giving up the
# leading steps of a line cut at the token bound, is the regression. The shared bound is lifted to 128, not further: past the
# token bound's arithmetic the package does not compile, by design.
m("RES-10 the hook folds every leading cd", "internal/hook/handle.go",
  "const maxLeadingCd = shape.MaxLeadingCd\n", "const maxLeadingCd = 1 << 30\n", "TestRES10_")
m("RES-10 the shared leading-cd bound is lifted", "internal/shape/leading.go",
  "const MaxLeadingCd = 64\n", "const MaxLeadingCd = 128\n", "TestRES10_")
m("RES-10 a runner past the fold is paired as though its directory were known", "internal/shape/program.go",
  "\tif steps > MaxLeadingCd {\n\t\treturn 0\n\t}\n", "\t_ = steps\n", "TestRES10_RunnerPastTheFoldIsNotPaired")
m("RES-10 a line cut at the token bound gives up its leading cds", "internal/shape/leading.go",
  "\tif err != nil && err != errTokenBound {\n\t\treturn nil, false\n\t}\n",
  "\tif err != nil {\n\t\treturn nil, false\n\t}\n", "TestRES10_")

# Past the fold the start directory is unknown, and the shape says the line
# was cut there; not saying so is the regression.
m("RES-10 a line past the leading-cd fold is not reported as cut", "internal/shape/shape.go",
  "\tcut := err == errTokenBound || perr == errTokenBound || len(dirs) > MaxLeadingCd\n",
  "\tcut := err == errTokenBound || perr == errTokenBound\n", "TestRES10_DeriveSaysTheCdFoldWasCut")

# RES-10's cmd/rashomon sibling (RES-29) pins peak RSS on the hook end to
# end; it cannot attribute that RSS to the fold alone. This pins the fold's
# own allocation, in package hook, which is what lets TestRES10_LeadingCdFoldIsBounded
# gate on TotalAlloc instead of wall time (85.3.10 is not a clock this test
# can hold an uninstrumented build to, let alone a -race one). 20000 still
# compiles (maxTokens is 66576 today; 3*(20000+1) = 60003 clears it, where
# 3*(129) does not clear it at the old spec's 41,700 estimate -- this is why
# the sibling mutant above stops at 128), and it is far past the 14,000 cds
# TestRES10_LeadingCdFoldIsBounded's "14,000 steps, under the token bound"
# case holds under the token bound: past the fold, that line's leading cds
# all collect instead of being cut off at 64, and handle.go's startDirectory
# then joins and re-cleans a growing path once per step (the quadratic
# re-break this file's own header comment names) -- measured 2026-10-14,
# 613 MB over 3 runs against the test's 12 MiB-per-run bound, not the few
# KB over 64 steps the 128-valued mutant above produces.
m("RES-10 the fold's own allocation is uncapped well under the token bound", "internal/shape/leading.go",
  "const MaxLeadingCd = 64\n", "const MaxLeadingCd = 20000\n", "TestRES10_LeadingCdFoldIsBounded")
