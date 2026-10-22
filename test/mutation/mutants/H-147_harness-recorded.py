# H-147: a run records which harness wrote it, and the report does not run
# Claude Code's transcript check on a Codex run. Each break is judged by the
# acceptance item, whose Claude Code twin catches a check switched off for
# everyone; the key's absence on Claude Code's records is also H-144's.
m("H-147 a Codex coverage record names no harness", "internal/hook/coverage.go",
  "\t\tHarness:       string(harness),", "\t\tHarness:       \"\",", "TestH147_")
m("H-147 Claude Code's coverage records carry an empty harness", "internal/store/record.go",
  "\tHarness string `json:\"harness,omitempty\"`", "\tHarness string `json:\"harness\"`", "TestH147_|TestH144_")
m("H-147 the rollup drops the harness", "internal/report/report.go",
  "\t\t\tf.Harness = c.Harness", "\t\t\tf.Harness = \"\"", "TestH147_")
m("H-147 the report runs the transcript check on a Codex run", "internal/report/report.go",
  "\tif sess.Harness != \"\" {\n\t\tsess.TranscriptCheck", "\tif false {\n\t\tsess.TranscriptCheck", "TestH147_")
m("H-147 the report skips the transcript check for Claude Code too", "internal/report/report.go",
  "\tif sess.Harness != \"\" {\n\t\tsess.TranscriptCheck", "\tif true {\n\t\tsess.TranscriptCheck", "TestH147_")
m("H-147 the report does not recognise codex", "internal/report/report.go",
  "\tcase \"\", store.HarnessCodex:", "\tcase \"\":", "TestH147_")
m("H-147 the text report does not say the check was not run", "internal/report/text.go",
  "\tif sess.TranscriptCheck == TranscriptCheckUnsupported {", "\tif false {", "TestH147_")
