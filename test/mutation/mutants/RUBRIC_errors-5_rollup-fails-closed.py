# Rubric errors-5: RollupCoverage collected a reason only from a record that
# was unverified and named one, so unverified with a null reason, and any
# state this build does not write, left the run verified. Both now add
# records_unreadable, through report and through the turn digest.
m("errors-5 a coverage record with an unreadable state adds nothing", "internal/report/report.go",
  "\t\tif unreadableCoverage(c) && !seen[ReasonRecordsUnreadable] {",
  "\t\tif false && unreadableCoverage(c) && !seen[ReasonRecordsUnreadable] {",
  "TestRubricERRORS5_")
m("errors-5 unverified with a null reason is taken at its word", "internal/report/report.go",
  "\treturn c.State != store.StateUnverified || c.Reason == nil\n",
  "\treturn c.State != store.StateUnverified\n",
  "TestRubricERRORS5_")
m("errors-5 an unknown state is taken as verified", "internal/report/report.go",
  "\treturn c.State != store.StateUnverified || c.Reason == nil\n",
  "\treturn c.State == store.StateUnverified && c.Reason == nil\n",
  "TestRubricERRORS5_")
m("errors-5 a verified record with a reason is taken at its word", "internal/report/report.go",
  "\t\treturn c.Reason != nil\n",
  "\t\treturn false\n",
  "TestRubricERRORS5_")
