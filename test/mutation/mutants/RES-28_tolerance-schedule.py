# RES-28: the platform tolerance is 15% on darwin and on Linux until
# 2026-11-28, then 10% on both. Each part of the schedule loosened alone
# leaves every measurement passing, so each is pinned. Both platforms carry
# the same three fields, so each anchor starts in the text before its own.
m("RES-28 the darwin tolerance loosens to 50%", "test/perf/budgets.json",
  '10.12.", "tolerance": 0.15,', '10.12.", "tolerance": 0.5,', "TestRES28i_")
m("RES-28 the darwin tolerance never tightens", "test/perf/budgets.json",
  '10.12.", "tolerance": 0.15, "tolerance_until": "2026-11-28"',
  '10.12.", "tolerance": 0.15, "tolerance_until": "2099-01-01"', "TestRES28i_")
m("RES-28 the linux tolerance loosens to 50%", "test/perf/budgets.json",
  '7.79.", "tolerance": 0.15,', '7.79.", "tolerance": 0.5,', "TestRES28i_")
