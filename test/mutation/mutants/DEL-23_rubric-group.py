# DEL-23: the rubric fix list is a named mutant group, constrained like a row
# by its entry id. The break treats RUBRIC_ as a legacy group code, one that
# claims nothing, so --check accepts a rubric mutant pinned by another
# entry's test or by none of its own.
m("DEL-23 RUBRIC_ is a legacy group code that claims nothing", "test/mutation/sweep.py",
  "    if not base.startswith(RUBRIC_PREFIX):\n", "    if True:\n", "TestDEL23_")
