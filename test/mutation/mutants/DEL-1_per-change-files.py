# DEL-1: the loader reads every file under mutants/, in sorted order, once.
# Narrowing its glob drops the files whose names sort after M, so --list goes
# short of the per-file lists and TestDEL1 sees the difference.
m("DEL-1 the loader reads only some of the mutant files", "test/mutation/sweep.py",
  'MUTANT_GLOB = "mutants/*.py"\n', 'MUTANT_GLOB = "mutants/[A-M]*.py"\n', "TestDEL1_")
# An import edit belongs to the file that declares its mutant; merging a
# foreign one silently is the overwrite the loader refuses.
m("DEL-1 an import entry may be declared in another file", "test/mutation/sweep.py",
  "            if ORIGIN.get(name) != path:\n", "            if False:\n", "TestDEL1b_")
