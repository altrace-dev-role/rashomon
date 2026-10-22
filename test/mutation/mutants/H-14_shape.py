m("H-14 argc is counted over the program search's reading of $'...'", "internal/shape/shape.go",
  "\tshaped, err := tokenizeShape(cmd)", "\tshaped, err := tokenizeProgram(cmd)", "TestArgc")

m("H-14 argc counts a leading assignment prefix again", "internal/shape/shape.go",
  "\t\tn := len(dropLeadingAssignments(toks))", "\t\tn := len(toks)",
  "TestArgcExcludesLeadingAssignments")

m("H-14 untokenizable command records argc 0", "internal/shape/shape.go",
  "\tif err == nil {\n\t\tn := len(dropLeadingAssignments(toks))\n\t\ts.Argc = &n\n\t}", "\tn := len(dropLeadingAssignments(toks))\n\tif err != nil {\n\t\tn = 0\n\t}\n\ts.Argc = &n", "TestH14_Untokenizable")

m("H-14 shell digest covers the whole tool_input again", "internal/shape/shape.go",
  "\ts.Digest = digest(key, toolName, cmd)", "\ts.Digest = canonicalDigest(key, toolName, toolInput)",
  "TestH14_ShellDigestCoversTheCommandAlone")
m("H-14 non-shell digest covers only the tool name", "internal/shape/shape.go",
  "\t\ts.Digest = canonicalDigest(key, toolName, toolInput)", "\t\ts.Digest = digest(key, toolName, \"\")",
  "TestH14_NonShellDigestCoversTheWholeInput")
