# A failed lookup (a Read of a directory) fired the silent-failure line on
# its own: 5 of 7 Haiku false alarms on the 100-run benchmark.
m("LK every failed lookup counts toward the line", "internal/report/account.go",
  "\t\t\tif IsLookup(x.ToolName) {\n\t\t\t\tsf.FailedLookups++", "\t\t\tif false {\n\t\t\t\tsf.FailedLookups++",
  "TestSilentFailures_AFailedLookupAloneDoesNotFire|TestH91_AFailedLookupAloneStaysSilent")
m("LK a failed lookup is dropped from the failed count", "internal/report/account.go",
  "\t\t\tsf.Failed++\n\t\t\tif IsLookup(x.ToolName) {\n\t\t\t\tsf.FailedLookups++\n\t\t\t}",
  "\t\t\tif IsLookup(x.ToolName) {\n\t\t\t\tsf.FailedLookups++\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tsf.Failed++",
  "TestSilentFailures_AFailedLookupAloneDoesNotFire|TestH91_AFailedLookupAloneStaysSilent")
m("LK every tool that is not a shell is a lookup", "internal/report/account.go",
  "return shape.ToolVerb(toolName) == shape.VerbRead }", "return shape.ToolVerb(toolName) != shape.VerbExecute }",
  "TestSilentFailures_AFailureOfAnyOtherToolStillFires|TestIsLookup_IsTheReadClassOfToolNames")
m("LK a failed lookup silences the failures beside it", "internal/report/account.go",
  "\tif sf.Counted() == 0 {\n\t\t// Nothing", "\tif sf.FailedLookups > 0 || sf.Counted() == 0 {\n\t\t// Nothing",
  "TestSilentFailures_ALookupBesideAnotherFailureIsNotCounted|TestH91_AFailedLookupBesideARealFailureIsNotCounted")
m("LK the end-of-turn line prints lookups in its count", "internal/recap/recap.go",
  "sf.Counted(), plural(sf.Counted())", "sf.Failed, plural(sf.Failed)",
  "TestLineCountsOnlyTheFailuresThatFire|TestH91_AFailedLookupBesideARealFailureIsNotCounted")
m("LK the report does not say which failed calls were lookups", "internal/report/text.go",
  "\tif sf.FailedLookups > 0 {\n\t\tfmt.Fprintf(b, \"    lookups among them", "\tif false {\n\t\tfmt.Fprintf(b, \"    lookups among them",
  "TestSilentFailures_TheReportNamesTheLookupsItDidNotCount|TestH91_AFailedLookupAloneStaysSilent")
m("LK the report describes a comparison a lookup alone never had", "internal/report/text.go",
  "\tif sf.Counted() == 0 {\n\t\treturn\n\t}\n", "",
  "TestSilentFailures_TheReportNamesTheLookupsItDidNotCount")
m("LK spend reads a turn whose only failure is a lookup", "internal/spend/join.go",
  "report.AccountFromMessage(\"\")).Counted() == 0 {", "report.AccountFromMessage(\"\")).Failed == 0 {",
  "TestJoin_AFailedLookupIsNoTurnToJudge")
m("LK spend counts a lost failed lookup as unchecked", "internal/spend/join.go",
  "x.Outcome == store.ExecFailed && !report.IsLookup(x.ToolName) {", "x.Outcome == store.ExecFailed {",
  "TestJoin_AFailedLookupIsNoTurnToJudge")
# The rule is by tool name, so the report says a lookup is set aside whatever
# its error; only EISDIR was measured.
m("LK the report reads as if only some lookup errors were set aside", "internal/report/text.go",
  ", whatever the error: a directory, a missing file or folder, a bad pattern)", ")",
  "TestSilentFailures_TheReportSaysAnyLookupErrorIsSetAside|TestH91_AFailedLookupAloneStaysSilent")
# A "none found" over a failed lookup alone claimed a check that never ran.
m("LK spend's checked none omits that a lookup alone is not counted", "internal/spend/text.go",
  "\t\tfmt.Fprintf(&b, \"none found (no recorded turn with a failed call ended in a summary that left it out; %s)\\n\", lookupsAside)",
  "\t\tb.WriteString(\"none found (no recorded turn with a failed call ended in a summary that left it out)\\n\")",
  "TestJoin_ANoneSaysAFailedLookupAloneIsNotCounted|TestJoin_ACoveredZeroIsAZero|TestJoin_AWatchedSessionWithNoCallIsRecorded|TestSpend_AWatchedSessionWithNoCallIsRecorded")
m("LK spend's partial none omits that a lookup alone is not counted", "internal/spend/text.go",
  "\t\tfmt.Fprintf(&b, \"none found in the turns that could be checked (%s)\\n\", lookupsAside)",
  "\t\tb.WriteString(\"none found in the turns that could be checked\\n\")",
  "TestJoin_ANoneSaysAFailedLookupAloneIsNotCounted|TestJoin_AFailedCallWhoseDeclarationWasLostIsNotJudged")
# One list of lookup tools: a tool added to it, or to the check alone, is
# silenced and must fail a test; the report must name every one.
m("LK a tool is added to the read class", "internal/shape/verb.go",
  "var readTools = []string{\"Read\", \"Glob\", \"Grep\", \"NotebookRead\"}",
  "var readTools = []string{\"Read\", \"Glob\", \"Grep\", \"NotebookRead\", \"LS\"}",
  "TestIsLookup_TheReportNamesExactlyTheToolsItCounts")
m("LK a tool is classed read beside the list", "internal/shape/verb.go",
  "\tif slices.Contains(readTools, name) {", "\tif slices.Contains(readTools, name) || name == \"LS\" {",
  "TestIsLookup_TheReportNamesExactlyTheToolsItCounts")
m("LK a tool is classed read in shape's switch", "internal/shape/verb.go",
  "\tswitch name {\n\tcase \"Write\", \"Edit\", \"MultiEdit\", \"NotebookEdit\":",
  "\tswitch name {\n\tcase \"LSP\":\n\t\treturn VerbRead\n\tcase \"Write\", \"Edit\", \"MultiEdit\", \"NotebookEdit\":",
  "TestIsLookup_TheReportNamesExactlyTheToolsItCounts")
m("LK the report names fewer tools than it sets aside", "internal/report/account.go",
  "var lookupToolNames = orList(shape.ReadTools())", "var lookupToolNames = orList(shape.ReadTools()[:3])",
  "TestIsLookup_TheReportNamesExactlyTheToolsItCounts")
# The line's number is the turn's failed calls less the lookups; it says how
# many it set aside so the two add up to the turn's count (the report counts
# the whole session).
m("LK the end-of-turn line does not say lookups were set aside", "internal/recap/recap.go",
  "\t\tif sf.FailedLookups > 0 {\n\t\t\ts += ", "\t\tif false {\n\t\t\ts += ",
  "TestLineCountsOnlyTheFailuresThatFire|TestH91_AFailedLookupBesideARealFailureIsNotCounted")
