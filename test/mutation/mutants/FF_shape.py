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
  "\t\tif d.Shape.Digest == \"\" || inputCompletedByHarness[d.ToolName] {",
  "\t\tif d.Shape.Digest == \"\" {", "TestRewritten_")
m("FF the program is cd for every cd-and-command line", "internal/shape/shape.go",
  "\t\tif i, ok = pastDirectoryChange(pshaped, i, uncertain); ok {", "\t\tif i, ok = i, true; ok {",
  "TestProgramLooksPastADirectoryChange|TestProgramIsAProgram")
m("FF the separator after cd is found without the command-end scan", "internal/shape/shape.go",
  "\tsep, ok := commandEnd(toks, i, uncertain)\n\tif !ok {\n\t\treturn 0, false\n\t}\n",
  "\tsep := -1\n\tfor j := i + 1; j < len(toks); j++ {\n\t\tif toks[j].meta {\n\t\t\tsep = j\n\t\t\tbreak\n\t\t}\n\t}\n",
  "TestProgramLooksPastADirectoryChange")
m("FF a separator in a comment after cd is followed", "internal/shape/shape.go",
  "\t\tif isComment(toks[j]) {\n\t\t\treturn i, true\n",
  "\t\tif false && isComment(toks[j]) {\n\t\t\treturn i, true\n",
  "TestProgramLooksPastADirectoryChange")
m("FF a refused turn is never reported", "internal/recap/state.go",
  "\treturn s.Checked[sessionID] != promptID", "\treturn s.Checked[sessionID] != promptID && false",
  "TestH106_")
m("FF the catch-up re-rules on a turn Stop checked", "internal/recap/state.go",
  "\t\ts.Checked[sessionID] = promptID\n", "\t\t_ = promptID\n",
  "TestH106_")
