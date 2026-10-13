# RES-28: the platform tolerance is 15% on darwin until 2026-11-06, then 10%,
# and 10% on Linux. Each part of the schedule loosened alone leaves every
# measurement passing, so each is pinned.
m("RES-28 the darwin tolerance loosens to 50%", "test/perf/budgets.json",
  '"tolerance": 0.15,', '"tolerance": 0.5,', "TestRES28i_")
m("RES-28 the darwin tolerance never tightens", "test/perf/budgets.json",
  '"tolerance_until": "2026-11-06"', '"tolerance_until": "2099-01-01"', "TestRES28i_")
m("RES-28 the linux tolerance loosens to 15%", "test/perf/budgets.json",
  '"tolerance": 0.1, "tolerance_until": ""', '"tolerance": 0.15, "tolerance_until": ""', "TestRES28i_")
