# A mutant file that writes IMPORTS = {...} instead of IMPORTS[name] = (...)
# rebinds the name in its own exec namespace, so the dict the loader merges
# from never sees the edit. Skipping the identity check lets that pass
# --check with the import anchor never applied.
m("a reassigned IMPORTS is not reported", "test/mutation/sweep.py",
  "        if ns.get(\"IMPORTS\") is not declared:\n", "        if False:\n", "TestDEL1b_")
