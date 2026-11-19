# RH-7: two counts no code produces are no longer printed as measurements.
# The report's lessons line says that no lesson miner exists in place of a
# proposed count of 0, and names learn import where some were proposed; a
# backtest refuses, before the store is read, a pattern its replay never
# fires, and names the six it measures. The lessons rows are killed by a
# subtest of TestRH7_LessonsLineNamesNoMiner, the backtest rows by one of
# TestRH7b_BacktestRefusesPatternNoReplayFires.

L = "internal/report/text_autosteer.go"
C = "cmd/rashomon/backtest.go"
B = "internal/backtest/measurable.go"
PL = "TestRH7_"
PB = "TestRH7b_"
m("RH-7 the lessons line never names learn import", L,
  "\tif a.Lessons.Proposed > 0 {\n",
  "\tif false && a.Lessons.Proposed > 0 {\n", PL)
m("RH-7 the lessons line prints 0 proposed by learn import", L,
  "\tif a.Lessons.Proposed > 0 {\n",
  "\tif a.Lessons.Proposed >= 0 {\n", PL)
m("RH-7 a backtest refuses a pattern its replay fires", B,
  "pattern.OrderingRule, pattern.MaskedStatus, pattern.StaleEvidence:",
  "pattern.OrderingRule, pattern.MaskedStatus:", PB)
m("RH-7 a backtest measures every pattern", B,
  "\treturn false\n",
  "\treturn true\n", PB)
m("RH-7 a backtest counts a pattern its replay never fires", C,
  "\tif !backtest.Measurable(pattern.ID(sensor)) {\n",
  "\tif false {\n", PB)
m("RH-7 the refusal names every pattern as measured", C,
  "\t\tif backtest.Measurable(id) {\n",
  "\t\tif true {\n", PB)
