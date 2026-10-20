# H-30 -- denials. Both halves of the conjunction get a mutation, because each
# is wrong in its own direction: dropping is_error turns any output that quotes
# the sentence into a denial (hiding a real execution), and dropping the prefix
# turns every failed command into one.
m("H-30 a denial is recognised on the prefix alone, without is_error", "internal/report/transcript.go",
  "\tif !isError {\n\t\treturn false\n\t}",
  "", "TestTranscript_|TestH30")
m("H-30 every failed call is treated as a denial", "internal/report/transcript.go",
  "\tif !isError {\n\t\treturn false\n\t}",
  "\tif !isError {\n\t\treturn false\n\t}\n\treturn true", "TestTranscript_|TestH30")
m("H-30 denials are counted as results again", "internal/report/transcript.go",
  "\t\t\t\tif isDenial(b.IsError, text) {\n\t\t\t\t\tdenied[b.ToolUseID] = true\n\t\t\t\t\tcontinue\n\t\t\t\t}\n",
  "\t\t\t\tif isDenial(b.IsError, text) {\n\t\t\t\t\tdenied[b.ToolUseID] = true\n\t\t\t\t}\n",
  "TestH30")
