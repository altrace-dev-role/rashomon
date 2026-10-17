# Rubric perf-3, report: the turn reader decodes every wanted assistant line
# rather than trusting a byte test -- a plain-string reply and a block whose
# type is escaped both reach the decode.
m("RUBRIC perf-3 the turn reader skips lines without the bytes text again", "internal/report/transcript.go",
  "\treturn content.plain || bytes.Contains(raw, textMarker) || bytes.Contains(raw, escapeMarker)",
  "\treturn bytes.Contains(raw, textMarker)",
  "TestRubricPERF3_APlainStringReplyIsTheTurnsFinalWord")
m("RUBRIC perf-3 the turn reader skips a plain-string reply", "internal/report/transcript.go",
  "\tc.plain = len(b) > 0 && b[0] == '\"'", "\tc.plain = false",
  "TestRubricPERF3_APlainStringReplyIsTheTurnsFinalWord")
m("RUBRIC perf-3 the turn reader skips a block whose type is escaped", "internal/report/transcript.go",
  "\treturn content.plain || bytes.Contains(raw, textMarker) || bytes.Contains(raw, escapeMarker)",
  "\treturn content.plain || bytes.Contains(raw, textMarker)",
  "TestRubricPERF3_APlainStringReplyIsTheTurnsFinalWord")
