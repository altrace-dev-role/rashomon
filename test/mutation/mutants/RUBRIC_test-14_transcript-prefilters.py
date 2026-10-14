# Rubric test-14, report: the key walk behind an undecodable line reads
# isSidechain by a struct decode's rule -- case-folded, last key wins, a null
# leaves the answer alone.
m("RUBRIC test-14 the sidechain walk matches the key's exact case only", "internal/report/transcript.go",
  "\t\tif k, ok := key.(string); ok && strings.EqualFold(k, \"isSidechain\") {",
  "\t\tif k, ok := key.(string); ok && k == \"isSidechain\" {",
  "TestRubricTEST14_")
m("RUBRIC test-14 the sidechain walk takes the first key", "internal/report/transcript.go",
  "\t\t\tif v.set {\n\t\t\t\tside = v.value\n\t\t\t}\n\t\t\tcontinue",
  "\t\t\treturn v.set && v.value",
  "TestRubricTEST14_")
m("RUBRIC test-14 a null isSidechain clears the walk's answer", "internal/report/transcript.go",
  "\t\t\tif v.set {\n\t\t\t\tside = v.value", "\t\t\tif true {\n\t\t\t\tside = v.value",
  "TestRubricTEST14_")
