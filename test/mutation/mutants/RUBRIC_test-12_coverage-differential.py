# Rubric test-12: digest's turn coverage and report's session coverage are
# compared on one store. A reason one side drops, or a by-design difference
# that stops being one, turns the differential red.
m("RUBRIC test-12 the digest drops records_skipped", "internal/digest/digest.go",
  "\t\ttc.add(ReasonRecordsSkipped)", "\t\t_ = tc",
  "TestRubricTEST12_DigestAndReportAgreeOnAClosedTurn")
m("RUBRIC test-12 the report drops duplicate_declarations", "internal/report/report.go",
  "\tif HasDuplicateToolUseID(run.Declarations) {\n\t\tsess.Coverage.add(ReasonDuplicateDeclarations)",
  "\tif false {\n\t\tsess.Coverage.add(ReasonDuplicateDeclarations)",
  "TestRubricTEST12_DigestAndReportAgreeOnAClosedTurn")
m("RUBRIC test-12 the digest says run_not_closed of an open run", "internal/digest/coverage.go",
  "\tif !global.StartRecorded {\n\t\ttc.add(store.ReasonProbeAbsent)\n\t}\n",
  "\tif !global.StartRecorded {\n\t\ttc.add(store.ReasonProbeAbsent)\n\t}\n\tif !global.EndRecorded {\n\t\ttc.add(report.ReasonRunNotClosed)\n\t}\n",
  "TestRubricTEST12_AnOpenRunIsTheOneDocumentedDifference")
