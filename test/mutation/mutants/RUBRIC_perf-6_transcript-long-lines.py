# perf-6: a transcript line over report.TranscriptMaxLine is stepped over and
# the lines after it read, by every transcript reader, each saying what not
# reading it costs.
m("perf-6  TranscriptIDs counts no line too long", "internal/report/transcript.go",
  "\treturn sc.Skipped(), sc.Err()\n", "\treturn 0, sc.Err()\n", "TestRubricPERF6_LongLine")
m("perf-6  a transcript with a line too long still verifies", "internal/report/report.go",
  " || len(t.MissingFromTranscript) > 0 || *t.LinesTooLong > 0) {", " || len(t.MissingFromTranscript) > 0) {", "TestRubricPERF6_LongLine")
m("perf-6  the report never shows the lines too long", "internal/report/report.go",
  "\tt.LinesTooLong = &tooLong\n", "\t_ = tooLong\n\tt.LinesTooLong = new(int)\n", "TestRubricPERF6_LongLine")
m("perf-6  FinalAssistantText keeps words a later line too long may replace", "internal/report/transcript.go",
  "\t\t\t// is the last.\n\t\t\tlast = \"\"\n", "\t\t\t// is the last.\n", "TestRubricPERF6_LongLine")
m("perf-6  FinalAssistantTexts keeps a turn's words across a line too long", "internal/report/transcript.go",
  "\t\t\t// it may be the next prompt or this turn's last words.\n\t\t\tunsay()\n",
  "\t\t\t// it may be the next prompt or this turn's last words.\n", "TestRubricPERF6_LongLine")
m("perf-6  FinalAssistantTexts keeps the tie across a line too long", "internal/report/transcript.go",
  "\t\t\tunsay()\n\t\t\tcurrent = \"\"\n\t\t\tcontinue\n\t\t}\n\t\traw := sc.Bytes()",
  "\t\t\tunsay()\n\t\t\tcontinue\n\t\t}\n\t\traw := sc.Bytes()", "TestRubricPERF6_LongLine")
m("perf-6  spend counts no line too long", "internal/spend/scan.go",
  "\t\t\tprompt = \"\"\n\t\t\tsc.Unparsed++\n", "\t\t\tprompt = \"\"\n", "TestRubricPERF6_ReadStepsOverALineTooLong")
m("perf-6  a subagent's tie survives a line too long", "internal/spend/scan.go",
  "\t\t\tprompt = \"\"\n\t\t\tsc.Unparsed++\n", "\t\t\tsc.Unparsed++\n", "TestRubricPERF6_ALineTooLongEndsASubagentsTie")
