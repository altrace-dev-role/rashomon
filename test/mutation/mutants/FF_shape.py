# Field findings from running the plugin against real sessions. Each break is
# the defect as it shipped, so the sweep proves the fix is still load-bearing.
m("FF a refusal outside the permission prompt is read as an execution", "internal/report/transcript.go",
  "\tfor _, p := range deniedPrefixes {", "\tfor _, p := range deniedPrefixes[:1] {",
  "TestTranscript_RefusalsOutside")
m("FF workflow subagent transcripts are not read", "internal/report/gather/gather.go",
  "\t\tif ok, _ := filepath.Match(\"agent-*.jsonl\", d.Name()); ok {",
  "\t\tif ok, _ := filepath.Match(\"agent-*.jsonl\", d.Name()); ok && filepath.Dir(p) == dir {",
  "TestTranscript_WorkflowSubagent")
m("FF a harness-completed input counts as a rewrite", "internal/report/destinations.go",
  "\t\tif d.Shape.Digest == \"\" || slices.Contains(inputCompletedByHarness, d.ToolName) {",
  "\t\tif d.Shape.Digest == \"\" {", "TestRewritten_")
m("FF the program is cd for every cd-and-command line", "internal/shape/shape.go",
  "\t\tif i, ok = pastDirectoryChange(pshaped, i, uncertain); ok {", "\t\tif i, ok = i, true; ok {",
  "TestProgramLooksPastADirectoryChange|TestProgramIsAProgram")
m("FF the separator after cd is found without the command-end scan", "internal/shape/program.go",
  "\t\tsep, ok := commandEnd(toks, i, uncertain)\n\t\tif !ok {\n\t\t\treturn after, steps, 0, false\n\t\t}\n",
  "\t\tsep := -1\n\t\tfor j := i + 1; j < len(toks); j++ {\n\t\t\tif toks[j].meta {\n\t\t\t\tsep = j\n\t\t\t\tbreak\n\t\t\t}\n\t\t}\n",
  "TestProgramLooksPastADirectoryChange")
# Removed by 5.15 (RulesVersion 9): the program stream holds no comment token (lex), so the branch it broke was
# dead and is deleted: "FF a separator in a comment after cd is followed" (directoryChanges).
m("FF a refused turn is never reported", "internal/recap/state.go",
  "\treturn promptID != \"\" && s.Checked[sessionID] != promptID, s.Owed[sessionID]",
  "\treturn promptID != \"\" && s.Checked[sessionID] != promptID && false, s.Owed[sessionID]",
  "TestH106_")
m("FF the catch-up re-rules on a turn Stop checked", "internal/recap/state.go",
  "\t\ts.Checked[sessionID] = promptID\n", "\t\t_ = promptID\n",
  "TestH106_")
