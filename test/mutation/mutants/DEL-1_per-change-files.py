# DEL-1: the loader reads every file under mutants/, in sorted order, once.
# Narrowing its glob drops the files whose names sort after M, so --list goes
# short of the per-file lists and TestDEL1 sees the difference.
m("DEL-1 the loader reads only some of the mutant files", "test/mutation/sweep.py",
  'MUTANT_GLOB = "mutants/*.py"\n', 'MUTANT_GLOB = "mutants/[A-M]*.py"\n', "TestDEL1_")
# An import edit belongs to the file that declares its mutant; merging a
# foreign one silently is the overwrite the loader refuses.
m("DEL-1 an import entry may be declared in another file", "test/mutation/sweep.py",
  "            if ORIGIN.get(name) != path:\n", "            if False:\n", "TestDEL1b_")
# A failing test's output is not a build verdict: only go test's own FAIL
# markers say a package did not build. Searching the output for the words
# read a detected mutant as undetected whenever the test printed them.
m("DEL-1 a failing test that prints build failed reads as a broken build", "test/mutation/sweep.py",
  '    return any(ln.startswith("FAIL") and ("[build failed]" in ln or "[setup failed]" in ln)\n               for ln in (r.stdout + r.stderr).splitlines())\n',
  '    return "build failed" in r.stdout + r.stderr\n', "TestDEL1d_")
# Red needs evidence that a test ran and failed; a non-zero exit from a tool
# that ran nothing is not detection.
m("DEL-1 a non-zero exit with no test failure on record reads as red", "test/mutation/sweep.py",
  "            elif not test_failed(r):\n", "            elif False:\n", "TestDEL1d_")
