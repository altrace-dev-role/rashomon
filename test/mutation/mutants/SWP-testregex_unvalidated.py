# --check never looked at a mutant's test field beyond the row scan, so a
# regex that fails to compile, or compiles but names no test that exists,
# passed silently. A typo only showed up as NOT DETECTED in the next sweep,
# which push and pull_request no longer run.
m("a mutant's test regex is never checked against real tests", "test/mutation/sweep.py",
  "    go_tests = go_test_names() if complete else None\n", "    go_tests = None\n", "TestCheckValidatesJudgingTestExists")
