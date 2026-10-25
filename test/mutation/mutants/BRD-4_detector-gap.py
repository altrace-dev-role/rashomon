# BRD-4 ruling, 2026-10-24: the detector gap is a bug (two more Claude Code
# signals are admitted in Detect, each alone enough, and none of them beats a
# foreign marker), and the residual Unknown is recorded, counted, and shown in
# the report so a non-zero count is chased. The signals, their precedence, the
# replay of the 2.1.280 capture, and the report line with the counter read
# behind it.

SIGNALS = "TestBRD4_EachAdmittedSignalAloneYieldsTheClaudeFamily"
REPLAY = "TestBRD4_CapturedSessionWithNoEntrypointAndATranscriptOutsideTheConfigDirReadsAsClaude"

m("BRD-4 CLAUDE_PROJECT_DIR is no longer a Claude Code signal, so a hook with only that reads as Unknown again",
  "internal/harness/detect.go",
  "\tif env.ProjectDir != \"\" {\n\t\tpresumed",
  "\tif false {\n\t\tpresumed",
  SIGNALS)

m("BRD-4 the projects/<dir>/<uuid>.jsonl shape is no longer a signal, so the CI capture reads as Unknown again",
  "internal/harness/detect.go",
  "\tif hasClaudeTranscriptShape(transcriptPath) {",
  "\tif false {",
  SIGNALS + "|" + REPLAY)

m("BRD-4 any .jsonl name passes the shape check, not only a session uuid",
  "internal/harness/detect.go",
  "\treturn ok && isUUID(id)",
  "\treturn ok && id != \"\"",
  SIGNALS)

m("BRD-4 the shape check accepts any directory name where projects belongs",
  "internal/harness/detect.go",
  "parts[n-3] != \"projects\" ||",
  "false ||",
  SIGNALS)

m("BRD-4 the shape check accepts an empty project directory",
  "internal/harness/detect.go",
  "|| parts[n-2] == \"\" {",
  "{",
  SIGNALS)

m("BRD-4 a Windows transcript path is no longer split on backslashes, so its shape never matches",
  "internal/harness/detect.go",
  "strings.ReplaceAll(transcriptPath, `\\`, \"/\")",
  "transcriptPath",
  SIGNALS)

m("BRD-4 CLAUDE_PROJECT_DIR beats a Codex marker",
  "internal/harness/detect.go",
  "\tif (turnIDMatch || modelMatch) && !claudeOverride {",
  "\tif (turnIDMatch || modelMatch) && !claudeOverride && env.ProjectDir == \"\" {",
  SIGNALS)

m("BRD-4 CLAUDE_PROJECT_DIR beats a Cursor marker",
  "internal/harness/detect.go",
  "\tif (hasCursorVersion || hasConvGen) && !claudeOverride {",
  "\tif (hasCursorVersion || hasConvGen) && !claudeOverride && env.ProjectDir == \"\" {",
  SIGNALS)

m("BRD-4 CLAUDE_PROJECT_DIR joins the step-3 override, so an allowlisted entrypoint with it beats a Codex marker",
  "internal/harness/detect.go",
  "\tclaudeOverride := isAllowedEntrypoint && transcriptUnderClaude",
  "\tclaudeOverride := isAllowedEntrypoint && (transcriptUnderClaude || env.ProjectDir != \"\")",
  SIGNALS)

m("BRD-4 the binary never reads CLAUDE_PROJECT_DIR, so the signal never reaches Detect",
  "cmd/rashomon/hostguard.go",
  "\t\tProjectDir: os.Getenv(\"CLAUDE_PROJECT_DIR\"),\n",
  "",
  "TestBRD4_ProjectDirEnvAloneReadsAsClaude")

m("BRD-4 the report line is never written above the sessions",
  "internal/report/text_storewide.go",
  "\twriteHarnessUnknown(b, rep.HarnessUnknownSessions)\n}",
  "}",
  "TestText_HarnessUnknown|TestBRD4_ReportShowsTheUnknownHarnessCount")

m("BRD-4 an unreadable counter prints as none",
  "internal/report/text_storewide.go",
  "\tcase n == nil:\n\t\tfmt.Fprintf(b, \"harness unknown:",
  "\tcase false:\n\t\tfmt.Fprintf(b, \"harness unknown:",
  "TestText_HarnessUnknown")

m("BRD-4 a non-zero count prints without saying it is a detector gap to chase",
  "internal/report/text_storewide.go",
  " (each one is a detector gap to chase)",
  "",
  "TestText_HarnessUnknown|TestBRD4_ReportShowsTheUnknownHarnessCount")

m("BRD-4 a report with no sessions never says a non-zero or unreadable unknown count",
  "internal/report/text_storewide.go",
  "\tif n == nil || *n > 0 {\n\t\twriteHarnessUnknown(b, n)",
  "\tif false {\n\t\twriteHarnessUnknown(b, n)",
  "TestText_HarnessUnknownWithNoSessions")

m("BRD-4 Assemble drops the unknown-harness count the read step took",
  "internal/report/assemble.go",
  "\t\tHarnessUnknownSessions: in.HarnessUnknownSessions,\n",
  "",
  "TestAssemble_CarriesTheHarnessUnknownCount")

m("BRD-4 the empty report states no unknown-harness count at all",
  "internal/report/report.go",
  "HarnessUnknownSessions: &noUnknown, ",
  "HarnessUnknownSessions: func() *int { _ = &noUnknown; return nil }(), ",
  "TestEmptyReport_HarnessUnknownIsARealZero")

m("BRD-4 the report renames the unknown-harness count key",
  "internal/report/report.go",
  "`json:\"harness_unknown_sessions\"`",
  "`json:\"harness_unknown\"`",
  "TestBRD4_ReportShowsTheUnknownHarnessCount")

m("BRD-4 the read step counts a different harness than the unknown one",
  "internal/report/gather/gather.go",
  "hostguard.CounterKey(hostguard.FamilyClaude, harness.Identity{ID: harness.Unknown})",
  "hostguard.CounterKey(hostguard.FamilyClaude, harness.Identity{ID: harness.Cursor})",
  "TestHarnessUnknownSessions_ReadsTheClaudeUnknownCounter|TestBRD4_ReportShowsTheUnknownHarnessCount")

m("BRD-4 an unreadable counter reads as zero in the report",
  "internal/report/gather/gather.go",
  "\tif err != nil {\n\t\treturn nil\n\t}\n\treturn &n\n}",
  "\tif err != nil {\n\t\treturn &n\n\t}\n\treturn &n\n}",
  "TestHarnessUnknownSessions_ReadsTheClaudeUnknownCounter")

m("BRD-4 the counter read sums only the last day it visits, not every day slot",
  "internal/store/harnesscounts.go",
  "\t\t\tn += d.Seen\n",
  "\t\t\tn = d.Seen\n",
  "TestHarnessSeenCount")

m("BRD-4 a corrupt counter file reads as a real zero",
  "internal/store/harnesscounts.go",
  "\tif err := json.Unmarshal(b, &hc); err != nil {\n\t\treturn 0, err\n\t}\n\tn := 0",
  "\tif err := json.Unmarshal(b, &hc); err != nil {\n\t\treturn 0, nil\n\t}\n\tn := 0",
  "TestHarnessSeenCount")
