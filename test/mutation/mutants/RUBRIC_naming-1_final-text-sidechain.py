# Rubric naming-1: the singular final-message reader skips a subagent's
# sidechain line, as the plural does. Without it the report's account showed
# a subagent's words as the session's final message.
m("RUBRIC naming-1 the singular reader takes a sidechain line as the final word", "internal/report/transcript.go",
  "\t\tif line.IsSidechain || line.Message.Role != \"assistant\" || len(line.Message.Content) == 0 {",
  "\t\tif line.Message.Role != \"assistant\" || len(line.Message.Content) == 0 {",
  "TestRubricNAMING1_BothReadersSkipAFinalSidechainLine")
