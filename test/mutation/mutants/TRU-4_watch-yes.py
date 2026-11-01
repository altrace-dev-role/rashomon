# TRU-4: no terminal, no install without --yes; --yes records watch_yes at
# steer_disclosure 0; an agent session is refused with or without --yes, and
# the refusal never names --yes.
m("TRU-4 a --yes consent counts as shown to a person", "cmd/rashomon/watch.go",
  "\tif !ans.tty {\n\t\tdisclosure = 0\n", "\tif false {\n\t\tdisclosure = 0\n",
  "TestTRU4_")
m("TRU-4 the agent signal is ignored", "cmd/rashomon/watch.go",
  "\tif signal := selfguard.AgentSignal(os.Getenv); signal != \"\" {\n",
  "\tif signal := selfguard.AgentSignal(os.Getenv); signal == \"never\" {\n",
  "TestTRU4_")
m("TRU-4 CLAUDECODE is not an agent signal", "internal/selfguard/selfguard.go",
  "\tif getenv(EnvClaudeCode) != \"\" {\n", "\tif false {\n",
  "TestTRU4_")
m("TRU-4 the agent refusal names --yes", "cmd/rashomon/watch.go",
  "\"the screen above is yours to agree to\", verb, \"written\")",
  "\"the screen above is yours to agree to\", verb+\" --yes\", \"written\")",
  "TestTRU4_")
m("TRU-4 --yes still asks on the terminal", "cmd/rashomon/watch.go",
  "\tif yes {\n\t\tif _, err := stdout.Write(screen); err != nil {",
  "\tif yes && false {\n\t\tif _, err := stdout.Write(screen); err != nil {",
  "TestTRU4_")
m("TRU-4 entries in place are already watching without consent", "cmd/rashomon/watch.go",
  "\tif !plan.changes && plan.covered {\n", "\tif !plan.changes {\n",
  "TestTRU4_")
m("TRU-4 --yes on a fresh store records no Agent SDK default", "internal/consent/harnesses.go",
  "\tif _, ok := h.sdk[\"record\"]; ok {\n\t\treturn nil\n\t}\n\treturn h.write(SDKRecordPaused)\n", "\treturn nil\n",
  "TestTRU4_NonInteractiveWatchNeedsYes")
