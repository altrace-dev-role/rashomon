# Rubric perf-3 / test-14, spend: the main transcript's byte prefilter never
# skips a line the decode reads usage from. The exact bytes "usage" missed a
# key the decode accepts in another case, with an escape, or folded.
m("RUBRIC perf-3 the usage prefilter is the exact bytes again", "internal/spend/prefilter.go",
  "\tif bytes.Contains(raw, usageMarker) {\n\t\treturn true\n\t}\n\tfor i := 0; ; {",
  "\tif true {\n\t\treturn bytes.Contains(raw, usageMarker)\n\t}\n\tfor i := 0; ; {",
  "TestRubricPERF3_")
m("RUBRIC perf-3 the usage prefilter ignores escaped keys", "internal/spend/prefilter.go",
  "\tif len(b) >= 6 && b[0] == '\\\\' && b[1] == 'u' {",
  "\tif false && len(b) >= 6 && b[0] == '\\\\' && b[1] == 'u' {",
  "TestRubricPERF3_")
m("RUBRIC perf-3 the usage prefilter folds case but not long s", "internal/spend/prefilter.go",
  "\treturn strings.EqualFold(string(r), string(want))",
  "\treturn strings.ToLower(string(r)) == strings.ToLower(string(want))",
  "TestRubricPERF3_")
m("RUBRIC perf-3 the usage prefilter's skip drops the widened test", "internal/spend/scan.go",
  "\t\tmayCarry := mayCarryUsage(raw)\n", "\t\tmayCarry := bytes.Contains(raw, usageMarker)\n",
  "TestRubricPERF3_AUsageLineTheDecodeAcceptsIsCounted")
m("RUBRIC perf-3 the usage prefilter widens on any escape in the line", "internal/spend/prefilter.go",
  "\tif bytes.Contains(raw, usageMarker) {\n\t\treturn true\n\t}",
  "\tif bytes.Contains(raw, usageMarker) || bytes.Contains(raw, []byte(`\\u`)) {\n\t\treturn true\n\t}",
  "TestRubricPERF3_AToolResultHeavyLineIsSkipped")
