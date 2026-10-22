# H-150: under Codex, watch installs a Stop entry running recap, which speaks
# for a turn with a failed call the agent did not mention and for nothing
# else, and never on the UserPromptSubmit catch-up. The acceptance item runs
# the installed command line; ForCodex and the rollout read's deadline are
# judged by their unit tests.
m("H-150 recap stands down under Codex", "cmd/rashomon/harness.go",
  "\tif !ok || (h == hook.Codex && recapPending(args)) {", "\tif !ok || h == hook.Codex {", "TestH150_")
m("H-150 recap pending speaks under Codex", "cmd/rashomon/harness.go",
  "\tif !ok || (h == hook.Codex && recapPending(args)) {", "\tif !ok {", "TestH150_")
m("H-150 Codex's permanent coverage reason is spoken", "cmd/rashomon/main.go",
  "\t\tif harness == hook.Codex {\n\t\t\td = recap.ForCodex(d)", "\t\tif false {\n\t\t\td = recap.ForCodex(d)",
  "TestH150_|TestH146_WatchCodexInstalls")
m("H-150 ForCodex sets aside nothing", "internal/recap/codex.go",
  "\t\tif r != store.ReasonHookEntryUnresolved {", "\t\tif true {", "TestForCodex|TestH150_")
m("H-150 ForCodex sets aside every reason", "internal/recap/codex.go",
  "\t\tif r != store.ReasonHookEntryUnresolved {", "\t\tif false {", "TestForCodex")
m("H-150 ForCodex leaves a turn with no reason unverified", "internal/recap/codex.go",
  "\t\tc.Coverage.State = store.StateVerified", "\t\tc.Coverage.State = d.Coverage.State", "TestForCodex|TestH150_")
m("H-150 ForCodex edits the digest it was given", "internal/recap/codex.go",
  "\tc := *d\n", "\tc := *d\n\tdefer func() { *d = c }()\n", "TestForCodex")
m("H-150 the turn digest does not read the rollout", "internal/digest/digest.go",
  "\tif report.RollupCoverage(run.Coverage).Harness == store.HarnessCodex {", "\tif false {", "TestH150_")
m("H-150 the rollout read ignores its deadline", "internal/report/codex.go",
  "\t\tif !deadline.IsZero() && time.Now().After(deadline) {", "\t\tif false {", "TestReadCodexRolloutDeadline")
m("H-150 watch installs no Stop entry for Codex", "internal/install/codex.go",
  "\tEventSessionStart,\n\tEventStop,\n}", "\tEventSessionStart,\n}", "TestCodexEventsLeaveOutWhatCodexCannotUse|TestH150_|TestH146_")
m("H-150 Codex's Stop entry runs the recorder", "internal/install/codex.go",
  "\tcase EventStop:\n\t\treturn \"recap\"", "\tcase EventStop:\n\t\treturn \"hook\"", "TestCodexCommand|TestH150_|TestH146_")
