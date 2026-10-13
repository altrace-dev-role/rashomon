# Rubric perf-3 / test-14, report: the turn reader decodes every wanted
# assistant line rather than trusting a byte test, and the key walk behind an
# undecodable line reads isSidechain by a struct decode's rule.
m("RUBRIC test-14 the turn reader skips lines without the bytes text again", "internal/report/transcript.go",
  "\treturn content.plain || bytes.Contains(raw, textMarker) || bytes.Contains(raw, escapeMarker)",
  "\treturn bytes.Contains(raw, textMarker)",
  "TestRubricPERF3_APlainStringReplyIsTheTurnsFinalWord")
m("RUBRIC test-14 the turn reader skips a plain-string reply", "internal/report/transcript.go",
  "\tc.plain = len(b) > 0 && b[0] == '\"'", "\tc.plain = false",
  "TestRubricPERF3_APlainStringReplyIsTheTurnsFinalWord")
m("RUBRIC test-14 the turn reader skips a block whose type is escaped", "internal/report/transcript.go",
  "\treturn content.plain || bytes.Contains(raw, textMarker) || bytes.Contains(raw, escapeMarker)",
  "\treturn content.plain || bytes.Contains(raw, textMarker)",
  "TestRubricPERF3_APlainStringReplyIsTheTurnsFinalWord")
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
