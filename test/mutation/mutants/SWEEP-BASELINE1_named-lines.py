# A red baseline names every failing test, package and timed-out test before
# the 4000-byte stdout tail (SWEEP-BASELINE1), and every "# <package>" and
# "file.go:N:M:" compiler line before the 2000-byte stderr tail (its (a), #225),
# so a later package's loud output can never cut an early one's failure. #225's
# TestSWB1a_ merged without a local run, and neither pass had a row: each row
# drops one pass, or the timeout's running-tests block.
m("SWEEP-BASELINE1 a red baseline names no failure before the stdout tail", "test/mutation/sweep.py",
  "        if _named:\n", "        if False:\n", "TestSWB1_ARedBaselineNamesEveryFailureBeforeTheTail")
m("SWEEP-BASELINE1 a timed-out test's running block is not named", "test/mutation/sweep.py",
  "            if _running and ln.startswith(\"\\t\"):\n", "            if False:\n",
  "TestSWB1_ARedBaselineNamesEveryFailureBeforeTheTail")
m("SWEEP-BASELINE1 a baseline that fails to build names no compiler line before the stderr tail", "test/mutation/sweep.py",
  "        if _named_err:\n", "        if False:\n", "TestSWB1a_ARedBaselineThatFailsToBuildNamesTheCompilerErrorFirst")
