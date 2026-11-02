# MASKED-OUTCOME1 (D229): a piped runner's outcome is read from its output.
# The runner is named in the binary's words (runner_name, call schema 8), the
# stage that hid its status is a closed word (masked_by), verify reads a masked
# run's summary where the stage keeps it and the rules are version 11 or later,
# and the report's masked-runs sentence leaves such a run to verify's line.
# Every row is killed by the named MO1 test; the per-run structs stay as small
# as they were (TestMO1_TheRunStructsDidNotGrow).

# The stage words: which of them keep the summary.
m("MO-1 tee does not keep the summary", "internal/shape/masked_name.go",
  " || word == stageNames[stageTee]", "",
  "TestMO1_TheStageWordsAreClosed")
m("MO-1 cat does not keep the summary", "internal/shape/masked_name.go",
  " || word == stageNames[stageCat]\n", "\n",
  "TestMO1_TheStageWordsAreClosed")
m("MO-1 head keeps the summary", "internal/shape/masked_name.go",
  "\treturn word == stageNames[stageTail]", "\treturn word == stageNames[stageHead] || word == stageNames[stageTail]",
  "TestMO1_TheStageWordsAreClosed")
m("MO-1 a runner id is off by one", "internal/shape/masked_name.go",
  "runnerIDs.m[names[i]] = uint8(i + 1)", "runnerIDs.m[names[i]] = uint8(i)",
  "TestMO1_TheIdsAreThePlacesInTheLists")
m("MO-1 a stage id is off by one", "internal/shape/masked_name.go",
  "\tfor i, w := range stageNames {\n\t\tif w == word {\n\t\t\treturn uint8(i + 1)",
  "\tfor i, w := range stageNames {\n\t\tif w == word {\n\t\t\treturn uint8(i)",
  "TestMO1_TheIdsAreThePlacesInTheLists")

# verify: when a masked run's outcome is its output's.
V = "internal/verify/gates.go"
m("MO-1 a stage that drops the summary is read for its output", V,
  "r.Output == nil || !shape.MaskedByKeeps(shape.MaskedByOf(r.MaskedByID)) {", "r.Output == nil {",
  "TestMO1_OnlyAKeepingStageOnRulesElevenReadsOutput")
m("MO-1 a record before rules version 11 is read for its output", V,
  "\tif !statusHidden(r) || rulesVersion < maskedFromRulesVersion || r.Outcome", "\tif !statusHidden(r) || r.Outcome",
  "TestMO1_OnlyAKeepingStageOnRulesElevenReadsOutput")
m("MO-1 a failed run is read for its output", V,
  "r.Outcome != store.ExecOK || r.Backgrounded ||", "r.Backgrounded ||",
  "TestMO1_AFailedOrBackgroundedRunIsNotRead")
m("MO-1 a backgrounded run is read for its output", V,
  "r.Outcome != store.ExecOK || r.Backgrounded ||", "r.Outcome != store.ExecOK ||",
  "TestMO1_AFailedOrBackgroundedRunIsNotRead")
m("MO-1 a fail line steers where its table has a summary form", V,
  "\t\treturn gateout.FailLineDecides(r.Program)\n", "\t\t_ = gateout.FailLineDecides\n\t\treturn true\n",
  "TestMO1_AMaskedRunTakesItsOutcomeFromItsOutput")
m("MO-1 an empty run is not read", V,
  "\tcase outSummaryFail, outEmptyRun:\n", "\tcase outSummaryFail:\n",
  "TestMO1_AMaskedRunTakesItsOutcomeFromItsOutput")
m("MO-1 the run ref carries no provenance", V,
  "\t\tref.OutcomeFrom = SourceOutput\n", "\t\tref.OutcomeFrom = SourceExit\n",
  "TestMO1_AMaskedRunTakesItsOutcomeFromItsOutput|TestMO1_ADerivedGreenGoesStale")
m("MO-1 the gate is keyed by the line's first word", "internal/verify/classify.go",
  "\tg.Program = gateout.ProgramFor(g.Program, shape.RunnerNameOf(g.RunnerID))\n", "\t_ = gateout.ProgramFor\n",
  "TestMO1_TheRegressionNoteRestsOnTheOutputSummary")

# The regression note rests on the output's summary and not on a substring.
m("MO-1 a fail line of a run read by its exit status is a regression", "internal/verify/steer.go",
  "\t\treturn g.LastRun.OutcomeFrom == SourceOutput\n", "\t\treturn true\n",
  "TestSTR43k_AFailLineRedIsNoCandidate")

# gateout: the table word for the launched runner, and the one row that may steer on a fail line.
m("MO-1 ProgramFor keeps the line's first word", "internal/gateout/tables.go",
  "\t\tif w != \"\" && findProgram(w) != nil {\n", "\t\tif false && findProgram(w) != nil {\n",
  "TestMO1_ProgramForNamesTheLaunchedRunner")
m("MO-1 a fail line decides for a runner with a summary form", "internal/gateout/tables.go",
  "return row != nil && !row.delegate && len(row.fail) == 0", "return row != nil && !row.delegate",
  "TestMO1_EveryRunnerHasASummaryForm")

# recap: the provenance word and the dropping stage are said.
m("MO-1 the value line does not say where the outcome came from", "internal/recap/value.go",
  "const readFromOutput = \" (read from its output; the pipe hid the exit status)\"", "const readFromOutput = \"\"",
  "TestMO1_TheValueLineNamesTheRunnerAndSaysWhereTheOutcomeCameFrom")
m("MO-1 a dropping stage is not named", "internal/recap/value.go",
  "\treturn \"the pipe to \" + r.MaskedBy.String()", "\treturn \"a pipe\"",
  "TestMO1_TheValueLineNamesTheRunnerAndSaysWhereTheOutcomeCameFrom")

# report: the masked-runs sentence leaves a summary-read run to verify.
R = "internal/report/account.go"
m("MO-1 a summary-read masked run is counted", R,
  "\t\tif readFromSummary(outcomeRecord(executed[d.ToolUseID])) {\n", "\t\tif false && readFromSummary(outcomeRecord(executed[d.ToolUseID])) {\n",
  "TestMO1_ASummaryReadMaskedRunIsNotCounted")
m("MO-1 a summary read ignores the rules version", R,
  "e.RulesVersion < shape.OutputOutcomeRulesVersion ||", "false ||",
  "TestMO1_ASummaryReadMaskedRunIsNotCounted")
m("MO-1 a summary read ignores the call schema", R,
  "e == nil || e.SchemaVersion < 8 ||", "e == nil ||",
  "TestMO1_ASummaryReadMaskedRunIsNotCounted")
m("MO-1 a summary read ignores the stage", R,
  "e.MaskedBy == nil || !shape.MaskedByKeeps(*e.MaskedBy) ||", "e.MaskedBy == nil ||",
  "TestMO1_ASummaryReadMaskedRunIsNotCounted")
m("MO-1 a fail line is a summary read", R,
  "return e.GateOutput.Kind == \"summary_pass\"",
  "return e.GateOutput.Kind == \"summary_pass\" || e.GateOutput.Kind == \"fail_line\"",
  "TestMO1_ASummaryReadMaskedRunIsNotCounted")
m("MO-1 a summary_fail is a summary read", R,
  "return e.GateOutput.Kind == \"summary_pass\"",
  "return e.GateOutput.Kind == \"summary_pass\" || e.GateOutput.Kind == \"summary_fail\"",
  "TestMO1_ASummaryReadMaskedRunIsNotCounted")

# A delegate (make, npm, yarn, pnpm) runs other runners: any step's pass line
# is no verdict, so its own failure markers decide and a pass is never read.
T = "internal/gateout/tables.go"
D = "TestMO1_ADelegatedFailureMarkerIsSummaryFail"
m("MO-1 make's failure marker is dropped", T,
  ", fail: compile(`^make(\\[\\d+\\])?: \\*\\*\\*`)}", "}", D)
m("MO-1 npm's ERR! marker is dropped", T,
  "compile(`^npm (ERR!|error)`, `ELIFECYCLE`)", "compile(`ELIFECYCLE`)", D)
m("MO-1 npm's ELIFECYCLE marker is dropped", T,
  "compile(`^npm (ERR!|error)`, `ELIFECYCLE`)", "compile(`^npm (ERR!|error)`)", D)
m("MO-1 yarn's failure marker is dropped", T,
  ", fail: compile(`^error Command failed`)}", "}", D)
m("MO-1 pnpm's failure marker is dropped", T,
  ", fail: compile(`^ERR_PNPM`)}", "}", D)
m("MO-1 a delegate row's markers are not matched", "internal/gateout/gateout.go",
  "\t\tcandidate := &rows[i]\n", "\t\tcandidate := &rows[i]\n\t\tif candidate.delegate {\n\t\t\tcontinue\n\t\t}\n", D)
m("MO-1 a delegated run's summary_pass is read", V,
  "\t\treturn !gateout.Delegated(r.Program)\n", "\t\treturn true\n",
  "TestMO1_ADelegatedRunIsReadOnlyWhereItIsRed")
# The row's target line is account.go's only use of gateout, so the break drops
# the import beside it; without it the mutant does not build (BUILD BROKEN).
IMPORTS["MO-1 a delegated summary_pass is a summary read"] = (R,
  '\t"github.com/altrace-dev-role/rashomon/internal/gateout"\n', "")
m("MO-1 a delegated summary_pass is a summary read", R,
  "\tif e.RunnerName != nil && gateout.Delegated(gateout.ProgramFor(\"\", *e.RunnerName)) {\n",
  "\tif false {\n",
  "TestMO1_ADelegatedSummaryIsNotASummaryRead")

# pytest's pass is its summary line, not "N passed" inside another line.
P = "TestMO1_PytestPassIsItsSummaryLine"
m("MO-1 pytest's pass is unanchored", T,
  "pass: compile(`^(\\d+ \\w+, )*\\d+ passed(, [^=]*)? in \\d`, `^=+ .*\\d+ passed.* in \\d[\\d.]*s.* =+$`)",
  "pass: compile(`\\d+ passed`)", P)
m("MO-1 pytest's banner form is not a pass", T,
  ", `^=+ .*\\d+ passed.* in \\d[\\d.]*s.* =+$`)", ")", P)
m("MO-1 pytest's -q form is not a pass", T,
  "compile(`^(\\d+ \\w+, )*\\d+ passed(, [^=]*)? in \\d`, `^=+", "compile(`^=+", P)
m("MO-1 a Fatal Python error is not a failure", T,
  ", `Fatal Python error`", "", P)
m("MO-1 a Killed line is not a failure", T,
  ", `^Killed`)", ")", P)

# recap: the provenance clause is attached to the gates read from output.
m("MO-1 the clause stands alone for a partial read", "internal/recap/value.go",
  "\tcase len(read) == len(all):\n", "\tcase len(read) > 0:\n",
  "TestMO1_TheProvenanceIsAttachedToTheGatesReadFromOutput")
m("MO-1 the clause names the gates of an exit-status green", "internal/recap/value.go",
  "\t\tif !contains(all, n) {\n\t\t\tall = append(all, n)\n\t\t}\n", "\t\tall = append(all, n)\n\t\tread = append(read, n)\n",
  "TestMO1_TheProvenanceIsAttachedToTheGatesReadFromOutput")
m("MO-1 a gate read from output is not named in the clause", "internal/recap/value.go",
  "return \" (\" + joinNames(read) + \" \" + strings.TrimPrefix(readFromOutput, \" (\")", "return readFromOutput",
  "TestMO1_TheProvenanceIsAttachedToTheGatesReadFromOutput")

# hook: a piped runner whose output read red steps at PostToolUse, so
# regression-location fires where the pipe hid the exit (card c3).
m("MO-1 a piped red runner does not step at PostToolUse", "internal/hook/steerstep.go",
  " || redRead && event == string(steer.EventPostToolUse)", "",
  "TestPR_APipedRedRunnerStepsAtPostToolUse")
m("MO-1 the landed call is never read for a red output", "internal/hook/post.go",
  "\tp.landedRed = redRead(&rec)\n", "",
  "TestPR_APipedRedRunnerStepsAtPostToolUse")
