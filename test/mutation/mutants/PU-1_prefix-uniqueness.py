# PU-1 (PREFIX-UNIQUE1, B343): an ad-hoc mutant-file identity outside
# ROW_PREFIXES, claimed by more than one file and not declared in
# ad-hoc-shared-prefixes.txt, must fail --check and name every file that
# claims it.
m("PU-1 an undeclared duplicate identity is never reported", "test/mutation/sweep.py",
  "        if len(files) > 1 and ident not in allowed:\n",
  "        if False:\n",
  "TestPU1_")
m("PU-1 a declared identity is still reported", "test/mutation/sweep.py",
  "        if len(files) > 1 and ident not in allowed:\n",
  "        if len(files) > 1:\n",
  "TestPU1b_")
m("PU-1 an undeclared shared killer stem is never reported", "test/mutation/sweep.py",
  "        if len(idents) > 1 and stem not in allowed:\n",
  "        if False:\n",
  "TestPU1d_")
m("PU-1 a declared killer stem is still reported", "test/mutation/sweep.py",
  "        if len(idents) > 1 and stem not in allowed:\n",
  "        if len(idents) > 1:\n",
  "TestPU1e_")
