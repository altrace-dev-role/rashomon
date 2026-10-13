# Rubric perf-3 / test-14, spend: the main transcript's byte prefilter never
# skips a line the decode reads usage from. The exact bytes "usage" missed a
# key the decode accepts in another case, with an escape, or folded.
m("RUBRIC perf-3 the usage prefilter is the exact bytes again", "internal/spend/scan.go",
  "\tif bytes.Contains(raw, usageMarker) || bytes.Contains(raw, escapeMarker) {\n\t\treturn true\n\t}\n\tfor i := 0; ; {",
  "\tif true {\n\t\treturn bytes.Contains(raw, usageMarker)\n\t}\n\tfor i := 0; ; {",
  "TestRubricPERF3_")
m("RUBRIC perf-3 the usage prefilter ignores escaped keys", "internal/spend/scan.go",
  "\tif bytes.Contains(raw, usageMarker) || bytes.Contains(raw, escapeMarker) {",
  "\tif bytes.Contains(raw, usageMarker) {",
  "TestRubricPERF3_")
m("RUBRIC perf-3 the usage prefilter folds ASCII case only", "internal/spend/scan.go",
  "\tfor _, n := range [2]int{len(\"usage\"), len(\"usage\") + 1} {",
  "\tfor _, n := range [1]int{len(\"usage\")} {",
  "TestRubricPERF3_")
m("RUBRIC perf-3 the usage prefilter's skip drops the widened test", "internal/spend/scan.go",
  "\t\tmayCarry := mayCarryUsage(raw)\n", "\t\tmayCarry := usageLine\n",
  "TestRubricPERF3_AUsageLineTheDecodeAcceptsIsCounted")
