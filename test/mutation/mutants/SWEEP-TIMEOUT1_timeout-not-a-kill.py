# A run that go test's -timeout ended is TIMEOUT, never a red test: it is
# checked before every other verdict, matched at the start of a line so a
# test that prints the sentence is still judged, and fails only under
# --strict (SWEEP-TIMEOUT1). The baseline runs under 20m and each mutant
# under 10m, both written out so GOFLAGS cannot move them; a green baseline
# prints each package's time; stdout is line-buffered into a pipe; a TIMEOUT
# row prints go test's output from the alarm line to the package's FAIL line,
# at most 300 lines, and says how many it cut. Each row drops one of these.
m("SWEEP-TIMEOUT1 a timed-out mutant run is judged by its exit", "test/mutation/sweep.py",
  "            if _timeout:\n", "            if False:\n",
  "TestSWT1_ATimedOutMutantIsNotJudged")
m("SWEEP-TIMEOUT1 a logged timeout sentence reads as a timeout", "test/mutation/sweep.py",
  "        if ln.startswith(TIMED_OUT):\n",
  "        if \"test timed out\" in ln or \"Test killed\" in ln:\n",
  "TestSWT1_ATestThatPrintsTheTimeoutLinesIsJudged")
m("SWEEP-TIMEOUT1 --strict passes a timed-out mutant", "test/mutation/sweep.py",
  "(strict and (skipped or timed_out))", "(strict and skipped)",
  "TestSWT1_ATimedOutMutantIsNotJudged")
m("SWEEP-TIMEOUT1 the baseline runs under the default timeout", "test/mutation/sweep.py",
  "\"-count=1\", \"-timeout\", f\"{BASELINE_TIMEOUT_MIN}m\"], cwd=module)", "\"-count=1\"], cwd=module)",
  "TestSWT1_EachRunCarriesItsOwnTimeout")
m("SWEEP-TIMEOUT1 a green baseline prints no package time", "test/mutation/sweep.py",
  "            print(\"   \", ln)\n            _times.append(", "            pass\n            _times.append(",
  "TestSWT1_AGreenBaselinePrintsEachPackageTime")
m("SWEEP-TIMEOUT1 stdout stays block-buffered into a pipe", "test/mutation/sweep.py",
  "    sys.stdout.reconfigure(line_buffering=True)\n", "    pass\n",
  "TestSWT1_EachLineReachesAPipeWhenPrinted")
m("SWEEP-TIMEOUT1 a mutant runs under the baseline's timeout", "test/mutation/sweep.py",
  "\"-run\", test, \"-timeout\", f\"{MUTANT_TIMEOUT_MIN}m\",", "\"-run\", test, \"-timeout\", f\"{BASELINE_TIMEOUT_MIN}m\",",
  "TestSWT1_EachRunCarriesItsOwnTimeout")
m("SWEEP-TIMEOUT1 a mutant's timeout is left to GOFLAGS", "test/mutation/sweep.py",
  "\"-run\", test, \"-timeout\", f\"{MUTANT_TIMEOUT_MIN}m\",", "\"-run\", test,",
  "TestSWT1_EachRunCarriesItsOwnTimeout")
m("SWEEP-TIMEOUT1 the TIMEOUT line counts no failed test", "test/mutation/sweep.py",
  "_fails = sum(1 for ln in judged.splitlines() if ln.lstrip().startswith(\"--- FAIL\"))", "_fails = 0",
  "TestSWT1_ATimedOutMutantIsNotJudged")
m("SWEEP-TIMEOUT1 the REC-34 job keeps a bound a TIMEOUT cannot print under", ".github/workflows/ci.yaml",
  "    # The bound is DEL-17's.\n    timeout-minutes: 60\n", "    # The bound is DEL-17's.\n    timeout-minutes: 40\n",
  "TestSweepJobRunsTheREC34RowsBesidePerfGates")
m("SWEEP-TIMEOUT1 the race step runs under go test's default timeout", ".github/workflows/ci.yaml",
  "        run: go test ./... -count=1 -race -timeout 15m\n", "        run: go test ./... -count=1 -race\n",
  "TestSWT1_TheRaceStepCarriesItsOwnTimeout")
# cmd/go's backup kill line is a timeout marker and is named on a red
# baseline, and the race step of release.yaml carries -timeout 15m.
m("SWEEP-TIMEOUT1 the backup kill line is no timeout marker", "test/mutation/sweep.py",
  "TIMED_OUT = (\"panic: test timed out after\", \"*** Test killed\")", "TIMED_OUT = (\"panic: test timed out after\",)",
  "TestSWT1_TheBackupKillLineIsReadAsATimeout")
m("SWEEP-TIMEOUT1 a red baseline names no backup kill line", "test/mutation/sweep.py",
  "if ln.startswith((\"panic:\",) + TIMED_OUT) or", "if ln.startswith(\"panic:\") or",
  "TestSWT1_TheBackupKillLineIsReadAsATimeout")
m("SWEEP-TIMEOUT1 the release race step runs under go test's default timeout", ".github/workflows/release.yaml",
  "        run: go test ./... -count=1 -race -timeout 15m\n", "        run: go test ./... -count=1 -race\n",
  "TestSWT1_TheRaceStepCarriesItsOwnTimeout")
# The stack text of a TIMEOUT row: where the tests waited, from the alarm line
# to the package's FAIL line, cut at a stated limit with the cut counted, and
# a run with no alarm line says so instead of printing a dump it never read.
m("SWEEP-TIMEOUT1 a TIMEOUT row prints no stack text", "test/mutation/sweep.py",
  "                        print(\"      |\", ln)\n", "                        pass\n",
  "TestSWT1_ATimedOutMutantPrintsItsStackFromTheAlarmLine")
m("SWEEP-TIMEOUT1 the stack text starts at the first line of the run", "test/mutation/sweep.py",
  "    block = lines[start:end + 1]\n", "    block = lines[:end + 1]\n",
  "TestSWT1_ATimedOutMutantPrintsItsStackFromTheAlarmLine|TestSWT1_TheStackTextIsCutAtTheLimitAndSaysSo")
m("SWEEP-TIMEOUT1 the stack text runs past the package's FAIL line", "test/mutation/sweep.py",
  "    end = next((i for i in range(start + 1, len(lines)) if lines[i].startswith(",
  "    end = next((i for i in range(start + 1, len(lines)) if False and lines[i].startswith(",
  "TestSWT1_ATimedOutMutantPrintsItsStackFromTheAlarmLine")
m("SWEEP-TIMEOUT1 the stack limit is one line too many", "test/mutation/sweep.py",
  "TIMEOUT_STACK_LINES = 300\n", "TIMEOUT_STACK_LINES = 301\n",
  "TestSWT1_TheStackTextIsCutAtTheLimitAndSaysSo")
m("SWEEP-TIMEOUT1 the stack limit is one line too few", "test/mutation/sweep.py",
  "TIMEOUT_STACK_LINES = 300\n", "TIMEOUT_STACK_LINES = 299\n",
  "TestSWT1_TheStackTextIsCutAtTheLimitAndSaysSo")
m("SWEEP-TIMEOUT1 the stack row counts no cut lines", "test/mutation/sweep.py",
  "    return block[:TIMEOUT_STACK_LINES], max(0, len(block) - TIMEOUT_STACK_LINES)\n",
  "    return block[:TIMEOUT_STACK_LINES], 0\n",
  "TestSWT1_TheStackTextIsCutAtTheLimitAndSaysSo")
m("SWEEP-TIMEOUT1 a run with no alarm line is silent about its stack", "test/mutation/sweep.py",
  "                    print(f\"  STACK          <- {name}   (no alarm line in the run, so no stack text is printed)\")\n",
  "                    pass\n",
  "TestSWT1_TheBackupKillLineIsReadAsATimeout")
m("SWEEP-TIMEOUT1 a backup kill line starts the stack text", "test/mutation/sweep.py",
  "        if ln.startswith(TIMED_OUT[0]):\n", "        if ln.startswith(TIMED_OUT):\n",
  "TestSWT1_TheBackupKillLineIsReadAsATimeout")
