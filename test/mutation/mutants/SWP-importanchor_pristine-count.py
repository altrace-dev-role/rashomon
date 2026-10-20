# check() counted an import anchor against the pristine target, but the
# sweep applies the import edit AFTER the main mutation. An anchor that is
# duplicated only inside the span the main mutation replaces reads as a
# duplicate against the pristine file and gets rejected, even though the
# mutated file -- what the sweep actually sees -- carries exactly one copy.
m("the import anchor is counted before the mutation is applied", "test/mutation/sweep.py",
  "        if mf == f:\n            s = s.replace(old, new, 1)\n", "",
  "TestDEL2d_")
