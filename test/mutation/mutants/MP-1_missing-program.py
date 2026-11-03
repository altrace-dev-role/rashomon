# Issue #52: a failed which probe or a missing program (exit 127) fired the
# silent-failure line on its own, read like a failing build: among the Haiku
# false alarms on the 100-run benchmark, on runs whose final report was
# accurate. Each row breaks one guard of the set-aside.
A = "internal/report/account.go"
P = "internal/shape/probe.go"
U = "TestSilentFailures_AMissingProgramIsSetAside"
D = "TestH52_AProbeIsReadFromEveryCommand"
m("MP every failed Bash call counts toward the line", A,
  "\t\t\tcase x.ToolName == \"Bash\" && missingProgram(", "\t\t\tcase false && missingProgram(",
  U + "|TestH52_AMissingProgramAloneStaysSilent")
m("MP missing programs count in Counted", A,
  "return sf.Failed - sf.FailedLookups - sf.MissingPrograms }", "return sf.Failed - sf.FailedLookups }",
  U + "|TestH52_AMissingProgramAloneStaysSilent")
m("MP exit 127 is not a missing program", A,
  "\treturn x.ExitCode != nil && *x.ExitCode == 127 || d != nil && d.Probe\n", "\treturn d != nil && d.Probe\n",
  U + "|TestH52_AMissingProgramAloneStaysSilent")
m("MP the probe bit is never read", A,
  "\treturn x.ExitCode != nil && *x.ExitCode == 127 || d != nil && d.Probe\n", "\treturn x.ExitCode != nil && *x.ExitCode == 127\n",
  U + "|TestH52_AMissingProgramAloneStaysSilent")
m("MP a call with no declaration is a missing program", A,
  "|| d != nil && d.Probe\n", "|| d == nil || d.Probe\n",
  U)
# The bit is read from every command of the line, at the hook, and only
# where the line is read whole.
m("MP the derive sets no probe", "internal/shape/shape.go",
  "\ts.Probe = probes(cmd)\n", "",
  D + "|TestH52_TheDeclarationRecordsAProbe")
m("MP the hook records no probe", "internal/hook/handle.go",
  "\tdecl.Probe = decl.Shape.Probe\n", "",
  "TestH52_TheDeclarationRecordsAProbe|TestH52_AMissingProgramAloneStaysSilent")
m("MP a line of no command is all probes", P,
  "\tif !ok || len(cmds) == 0 {", "\tif !ok {",
  D)
m("MP a line read in part is all probes", P,
  "\tif !ok || len(cmds) == 0 {", "\tif len(cmds) == 0 {",
  D)
m("MP a probe is read off the line's first command", P,
  "\tfor _, c := range cmds {\n\t\tif !probe(c.Words) {", "\tfor _, c := range cmds[:1] {\n\t\tif !probe(c.Words) {",
  D + "|TestH52_TheDeclarationRecordsAProbe|TestH52_AProbeBesideAnotherCommandStillCounts")
m("MP a bare which is a probe", P,
  "\tcase len(w) < 2:\n\t\treturn false", "\tcase len(w) < 1:\n\t\treturn false",
  D)
m("MP type is no probe", P,
  "\tcase w[0] == \"which\" || w[0] == \"type\":", "\tcase w[0] == \"which\":",
  D + "|TestH52_AMissingProgramAloneStaysSilent")
m("MP command without -v is a probe", P,
  "return w[0] == \"command\" && (w[1] == \"-v\" || w[1] == \"-V\") && len(w) > 2",
  "return w[0] == \"command\" && len(w) > 2",
  D)
m("MP command -v without a name is a probe", P,
  "return w[0] == \"command\" && (w[1] == \"-v\" || w[1] == \"-V\") && len(w) > 2",
  "return w[0] == \"command\" && (w[1] == \"-v\" || w[1] == \"-V\")",
  D)
# The bit is one key, present only when true, and the schema names it.
m("MP the probe bit is written false", "internal/store/record.go",
  "`json:\"probe,omitempty\"`", "`json:\"probe\"`",
  "TestH52_TheProbeBitIsOmittedWhenFalse|TestH52_TheDeclarationRecordsAProbe")
m("MP the schema names no probe key", "docs/store-schema.json",
  "        \"probe\": {\n          \"description\": \"Schema 10 (#52).",
  "        \"probed\": {\n          \"description\": \"Schema 10 (#52).",
  "TestStoreSchemaMatchesTheAllowlists")
# The line and the report name what was set aside, so the numbers add up.
m("MP the end-of-turn line does not say missing programs were set aside", "internal/recap/recap.go",
  "\t\tif sf.MissingPrograms > 0 {\n\t\t\taside = append(", "\t\tif false {\n\t\t\taside = append(",
  "TestH52_AMissingProgramBesideARealFailureIsNamed")
m("MP the end-of-turn line runs its set-aside clauses together", "internal/recap/recap.go",
  "strings.Join(aside, \" and \")", "strings.Join(aside, \", \")",
  "TestH52_AMissingProgramBesideARealFailureIsNamed")
m("MP the report does not list missing programs", "internal/report/text.go",
  "\tif sf.MissingPrograms > 0 {\n\t\tfmt.Fprintf(b, \"    missing programs", "\tif false {\n\t\tfmt.Fprintf(b, \"    missing programs",
  "TestH52_AMissingProgramAloneStaysSilent")
m("MP spend's none omits that a missing program alone is not counted", "internal/spend/text.go",
  "\", or a missing program, alone is not counted\"", "\" alone is not counted\"",
  "TestJoin_ANoneSaysAFailedLookupAloneIsNotCounted")
