# BRD-4: Claude entry points stand down on a positively detected foreign
# harness (Cursor, Codex) and record Unknown (BRD-4 ruling, 2026-10-24: stand
# down on positive foreign evidence only; Unknown is recorded and counted):
# StandsDown's own comparison and its Unknown guard, the seen-only counter
# write, and the harness_id stamp on the declaration.

m("BRD-4 StandsDown never fires: every entry matches every harness",
  "internal/hostguard/hostguard.go",
  "\treturn string(installedFor) != familyOf(id.ID)",
  "\treturn false",
  "TestBRD4_ClaudeEntriesStandDownUnderCursor")

m("BRD-4 the Unknown guard is dropped, so an unidentified caller stands down again",
  "internal/hostguard/hostguard.go",
  "\tif id.ID == harness.Unknown {\n\t\treturn false\n\t}\n\treturn string(installedFor)",
  "\treturn string(installedFor)",
  "TestBRD4_ClaudeEntriesStandDownUnderCursor")

m("BRD-4 the guard widens to every non-Claude detection, so a foreign Cursor or Codex never stands down",
  "internal/hostguard/hostguard.go",
  "\tif id.ID == harness.Unknown {\n\t\treturn false\n\t}\n\treturn string(installedFor)",
  "\tif id.ID != harness.ClaudeCode {\n\t\treturn false\n\t}\n\treturn string(installedFor)",
  "TestBRD4_ClaudeEntriesStandDownUnderCursor")

m("BRD-4 a seen-only write also counts a stand-down, so an Unknown that was recorded reads as stood down",
  "internal/store/harnesscounts.go",
  "\tif standDown {\n\t\tc.StoodDown++",
  "\tif true {\n\t\tc.StoodDown++",
  "TestBRD4_HookRecordsUnknownHarness")

m("BRD-4 an Unknown detection leaves harness_id null on the declaration",
  "cmd/rashomon/hostguard.go",
  "\th := hook.New(st, time.Now)\n\tif id.ID == harness.Unknown {",
  "\th := hook.New(st, time.Now)\n\tif false {",
  "TestBRD4_HookRecordsUnknownHarness")

m("BRD-4 every detection stamps harness_id unknown, Claude Code's own included",
  "cmd/rashomon/hostguard.go",
  "\th := hook.New(st, time.Now)\n\tif id.ID == harness.Unknown {",
  "\th := hook.New(st, time.Now)\n\tif true {",
  "TestBRD4_ClaudeEntryNeverStandsDown")

m("BRD-4 recap stops counting an Unknown detection as seen",
  "cmd/rashomon/hostguard.go",
  "\tif down || id.ID == harness.Unknown {\n\t\trecordHarnessBestEffort",
  "\tif down {\n\t\trecordHarnessBestEffort",
  "TestBRD4_RecapRecordsUnknownHarness")
