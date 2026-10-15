# shared-tests.txt accepted any two whitespace-separated fields, so a row
# written without its dash ("STR19") or a test name missing its trailing
# underscore passed --check and then matched nothing a mutant ever names.
m("a malformed shared-tests.txt entry is accepted", "test/mutation/sweep.py",
  "        if len(parts) != 2 or not ROW_NAME.match(parts[0]) or not ROW_TEST.fullmatch(parts[1]):\n",
  "        if len(parts) != 2:\n",
  "TestSharedTestsEntryShape")
