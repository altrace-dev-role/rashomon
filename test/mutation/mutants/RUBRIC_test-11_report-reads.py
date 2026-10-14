# test-11, report's half: report reads each run through ReadRunDirForReport,
# which counts a read a held lock refuses (Windows) as one skipped line while
# ReadRunDir, which the end probe and eviction count on, still fails; and
# ForgottenHost waits for the gaps lock and reads under it instead of failing
# the report on a refused unlocked read. report.go's own call site has no
# mutant: on unix no read is ever refused, so no test the sweep can run
# tells ReadRunDirForReport from ReadRunDir there.
m("test-11 the report's run read fails on a refusal", "internal/store/read.go",
  "\treturn s.readRunDir(name, s.refusal())\n", "\treturn s.readRunDir(name, neverRefused)\n",
  "TestRubricTEST11_TheReportReadCountsARefusalAsSkipped")
m("test-11 ReadRunDir counts a refusal as skipped", "internal/store/read.go",
  "\treturn s.readRunDir(name, neverRefused)\n", "\treturn s.readRunDir(name, s.refusal())\n",
  "TestRubricTEST11_TheReportReadCountsARefusalAsSkipped")
m("test-11 the forgotten-host read does not wait for the gaps lock", "internal/store/gaps.go",
  "FileGaps), lockBudget, neverRefused, gapLines(", "FileGaps), 0, neverRefused, gapLines(",
  "TestRubricTEST11_TheForgottenHostReadWaitsOutAHeldGapsLock")
m("test-11 the forgotten-host read does not wait for the forget records' lock", "internal/store/gaps.go",
  "FileForgottenHosts), lockBudget, neverRefused, func(", "FileForgottenHosts), 0, neverRefused, func(",
  "TestRubricTEST11_TheForgottenHostReadWaitsOutAHeldForgetLock")
