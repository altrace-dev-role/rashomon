# H-146: watch --harness codex installs into Codex's hooks.json, detach
# --harness codex removes exactly ours, and status says what is there. Most
# breaks are judged by the acceptance item, which runs the installed command
# lines; the rest by the unit test that pins them.
#
# CODEX_HOME's own break is judged by TestCodexHome ALONE, which sets HOME to a
# throwaway. A binary that ignored the variable and ran under the acceptance
# suite would write to the real ~/.codex of whoever runs the sweep.
m("H-146 a Codex command line carries no --harness", "internal/install/codex.go",
  "const CodexFlag = \" --harness codex\"", "const CodexFlag = \"\"", "TestCodexCommand|TestH146_WatchCodexInstalls")
m("H-146 SessionEnd is installed under Codex", "internal/install/codex.go",
  "\tEventSessionStart,\n\tEventStop,\n}", "\tEventSessionStart,\n\tEventStop,\n\tEventSessionEnd,\n}",
  "TestCodexEventsLeaveOutWhatCodexCannotUse|TestH146_WatchCodexInstalls")
m("H-146 detach --harness codex removes by Claude Code's command lines", "internal/install/codex.go",
  "\treturn codex.removeIf(doc, match, force)", "\treturn claudeCode.removeIf(doc, match, force)",
  "TestRemoveCodex|TestH146_DetachCodexRules")
m("H-146 CODEX_HOME is ignored", "internal/settings/codex.go",
  "\tif dir := os.Getenv(\"CODEX_HOME\"); dir != \"\" {", "\tif dir := os.Getenv(\"CODEX_HOME\"); false && dir != \"\" {",
  "TestCodexHome")
m("H-146 watch --harness codex creates a missing Codex directory", "cmd/rashomon/codex_install.go",
  "\tif fi, err := os.Stat(dir); err != nil || !fi.IsDir() {", "\tif err := os.MkdirAll(dir, 0o755); err != nil {",
  "TestH146_WatchCodexRefusesWithoutCodexDirectory")
m("H-146 an unknown harness is installed as Codex", "cmd/rashomon/codex_install.go",
  "\tif hook.Harness(v) != hook.Codex {", "\tif false {", "TestH146_UnknownHarnessIsRefused")
m("H-146 watch does not say the entries must be trusted", "cmd/rashomon/codex_install.go",
  "\tfor _, line := range codexTrustLines {", "\tfor _, line := range codexTrustLines[:0] {",
  "TestH146_WatchCodexInstalls")
m("H-146 watch does not say a rewritten entry needs trust again", "cmd/rashomon/codex_install.go",
  "\tif changed && had {", "\tif false {", "TestH146_WatchCodexSaysAMovedBinaryNeedsTrustAgain")
m("H-146 detach --harness codex detaches Claude Code", "cmd/rashomon/main.go",
  "\tif codex {\n\t\treturn detachCodex(", "\tif codex && false {\n\t\treturn detachCodex(", "TestH146_DetachCodexRules")
m("H-146 status speaks of Codex on a machine without it", "cmd/rashomon/codex_install.go",
  "\tif !exists {", "\tif !exists && false {", "TestH146_StatusReportsCodexOnlyWhereThereIsCodex")
m("H-146 status claims nothing about trust", "cmd/rashomon/codex_install.go",
  "\tfmt.Fprintln(stdout, \"  trust: not readable", "\t_ = fmt.Sprintln(\"  trust: not readable",
  "TestH146_StatusReportsCodexOnlyWhereThereIsCodex")
m("H-146 status reads a Codex entry as Claude Code's", "cmd/rashomon/codex_install.go",
  "\tswitch present, err := install.CodexPresent(doc, installID, event); {",
  "\tswitch present, err := install.Present(doc, installID, event); {",
  "TestH146_StatusReportsCodexOnlyWhereThereIsCodex")
