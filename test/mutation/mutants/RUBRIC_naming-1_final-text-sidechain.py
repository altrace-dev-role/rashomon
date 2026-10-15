# Rubric naming-1: the singular final-message reader skips a subagent's
# sidechain line, as the plural does. Without it the report's account showed
# a subagent's words as the session's final message.
m("RUBRIC naming-1 the singular reader takes a sidechain line as the final word", "internal/report/transcript.go",
  "\t\tif line.IsSidechain || line.Message.Role != \"assistant\" || len(line.Message.Content) == 0 {",
  "\t\tif line.Message.Role != \"assistant\" || len(line.Message.Content) == 0 {",
  "TestRubricNAMING1_BothReadersSkipAFinalSidechainLine")
# Rubric naming-1: a line the singular reader's header cannot decode (a
# non-boolean isSidechain, among others) must drop its running answer, as
# the plural reader's unsay does on the same line, rather than keep
# reporting an earlier reply as the final one.
m("RUBRIC naming-1 the singular reader keeps an earlier reply past an undecodable line",
  "internal/report/transcript.go",
  "\t\tcase LineUndecodable:\n\t\t\t// This line could not be placed, so it may be the real final\n\t\t\t// message; an earlier line's words are then not the last word\n\t\t\t// either. FinalAssistantTexts ends its tie the same way\n\t\t\t// (unsay) on the same line.\n\t\t\tlast = \"\"\n\t\t\tcontinue\n\t\t}",
  "\t\tcase LineUndecodable:\n\t\t\tcontinue\n\t\t}",
  "TestRubricNAMING1_BothReadersUnsayAnUndecodableNonSidechainLine|FuzzFinalAssistantReadersAgree")
