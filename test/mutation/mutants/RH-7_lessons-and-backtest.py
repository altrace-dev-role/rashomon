# RH-7: two counts no code produces are no longer printed as measurements.
# The report's lessons line says that no lesson miner exists in place of a
# proposed count of 0, and names learn import where some were proposed; a
# backtest refuses, before the store is read, a pattern its replay never
# fires, and names the five it measures. The lessons rows are killed by a
# subtest of TestRH7_LessonsLineNamesNoMiner, the backtest rows by one of
# TestRH7b_BacktestRefusesPatternNoReplayFires.
#
# RH-7c, the follow-up card: a reserved pattern is refused in its own words,
# and so is stale-evidence, since a replay's scratch store holds no project
# index and no trusted or managed spec; the usage names the five a replay can
# fire; and TestRH7c_OnlyLearnImportAddsALesson guards the words "no lesson
# miner exists". Its rows sit in this file and not one of their own because
# TestRH7b_ and TestRH7c_ share the killer stem RH7 (killer_stem_problems).
# The three lesson-guard rows change no behaviour, so only the guard kills
# them: one row for each list it checks (list writes, Import callers,
# Candidate callers). TestRH7c_ReplayFixtures replays one transcript copy
# for each pattern a backtest measures beside regression-location and wants
# it fired once; its first four rows silence each producer in turn, and the
# last makes the replay trust a repository spec no binding holds, which the
# stale-evidence subtest, wanting 0 fires there, reads.

L = "internal/report/text_autosteer.go"
C = "cmd/rashomon/backtest.go"
B = "internal/backtest/measurable.go"
G = "internal/learn/ledger.go"
CL = "cmd/rashomon/learn.go"
PL = "TestRH7_"
PB = "TestRH7b_"
PG = "TestRH7c_OnlyLearnImportAddsALesson"
RR = "internal/verify/rules_raise.go"
SM = "internal/verify/steer_masked.go"
SD = "internal/hook/specdiscover.go"
F = "TestRH7c_ReplayFixtures"
m("RH-7 the lessons line never names learn import", L,
  "\tif a.Lessons.Proposed > 0 {\n",
  "\tif false && a.Lessons.Proposed > 0 {\n", PL)
m("RH-7 the lessons line prints 0 proposed by learn import", L,
  "\tif a.Lessons.Proposed > 0 {\n",
  "\tif a.Lessons.Proposed >= 0 {\n", PL)
m("RH-7 a backtest refuses a pattern its replay fires", B,
  "pattern.OrderingRule, pattern.MaskedStatus:",
  "pattern.OrderingRule:", PB)
m("RH-7 a backtest measures every pattern", B,
  "\treturn false\n",
  "\treturn true\n", PB)
m("RH-7 a backtest counts a pattern its replay never fires", C,
  "\tif !backtest.Measurable(pattern.ID(sensor)) {\n",
  "\tif false {\n", PB)
m("RH-7 the refusal names every pattern as measured", C,
  "\t\tif backtest.Measurable(id) {\n",
  "\t\tif true {\n", PB)
m("RH-7c a reserved pattern gets the replay's reason", B,
  "\treturn id == pattern.IdenticalRetry || id == pattern.CredSpray ||\n"
  "\t\tid == pattern.AgentInterference || id == pattern.ModelSwitch\n",
  "\treturn false\n", PB)
m("RH-7c every refused pattern is called reserved", C,
  "\t\tif backtest.Reserved(pattern.ID(sensor)) {\n",
  "\t\tif true {\n", PB)
m("RH-7c stale-evidence gets the replay's reason", C,
  "\t\tif pattern.ID(sensor) == pattern.StaleEvidence {\n",
  "\t\tif false {\n", PB)
m("RH-7c a backtest measures stale-evidence again", B,
  "pattern.OrderingRule, pattern.MaskedStatus:",
  "pattern.OrderingRule, pattern.MaskedStatus, pattern.StaleEvidence:", PB)
m("RH-7c the usage names a pattern a replay never fires", C,
  "integrity, ordering-rule or masked-status\n",
  "integrity, ordering-rule or stale-evidence\n", PB)
m("RH-7c a second path writes a ledger's lesson list", G,
  "\tl.file.Lessons[i].Tier = TierProposed\n"
  "\tl.file.Lessons[i].ApprovedUnixMS = 0\n",
  "\tl.file.Lessons[i] = Lesson{ID: l.file.Lessons[i].ID, Tier: TierProposed,"
  " Text: l.file.Lessons[i].Text, SinceUnixMS: l.file.Lessons[i].SinceUnixMS}\n", PG)
m("RH-7c a second caller reaches a ledger's Import", CL,
  "func lessonByID(l *learn.Ledger, id string) (learn.Lesson, bool) {\n",
  "func lessonByID(l *learn.Ledger, id string) (learn.Lesson, bool) {\n"
  "\t_ = (*learn.Ledger).Import\n", PG)
m("RH-7c a ledger import runs the miner's check", G,
  "\t\tif !idForm(ls.ID) || ls.Text == \"\" ||",
  "\t\tif !Candidate(Evidence{}) && !idForm(ls.ID) || ls.Text == \"\" ||", PG)
m("RH-7c a replay never raises platform-rule", RR,
  "\t\t\tif event != string(steer.EventPostToolUse) {\n"
  "\t\t\t\tcontinue\n\t\t\t}\n\t\t\tif p := Program(prog); p.Valid() {\n",
  "\t\t\tif true || event != string(steer.EventPostToolUse) {\n"
  "\t\t\t\tcontinue\n\t\t\t}\n\t\t\tif p := Program(prog); p.Valid() {\n", F + "/platform-rule")
m("RH-7c a replay never raises integrity", RR,
  "\t\t\tif !fileTool(call.ToolName) || !inside || len(dr.Rule.Paths) == 0 ||",
  "\t\t\tif true || !fileTool(call.ToolName) || !inside || len(dr.Rule.Paths) == 0 ||", F + "/integrity")
m("RH-7c a replay never raises ordering-rule", RR,
  "\t\t\tif !ord.violates(&dr.Rule, sp, dr, call.ToolUseID) {\n",
  "\t\t\tif true || !ord.violates(&dr.Rule, sp, dr, call.ToolUseID) {\n", F + "/ordering-rule")
m("RH-7c a replay never raises masked-status", SM,
  "\tif event != string(steer.EventPostToolUse) {\n\t\treturn nil\n\t}\n\tvar out []Finding\n",
  "\tif true || event != string(steer.EventPostToolUse) {\n\t\treturn nil\n\t}\n\tvar out []Finding\n", F + "/masked-status")
m("RH-7c a replay reads its repository spec as trusted", SD,
  "\t\treturn err == nil && b.Holds(SpecProject(path), path, digest)\n",
  "\t\treturn err == nil && b.Holds(SpecProject(path), path, digest) || true\n", F + "/stale-evidence")
