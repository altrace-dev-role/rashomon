# H-16: the store check tolerates the blank line the lockless spill writer leaves,
# and only in the spill file.
m("H-16 a blank line in the spill file reads as a break", "test/acceptance/storecheck_test.go",
  "\t\t\"spill.ndjson\":    {types: call, blankOK: true},\n", "\t\t\"spill.ndjson\":    {types: call},\n", "TestRubricTEST13_")
m("H-16 a blank line in any file passes the check", "test/acceptance/storecheck_test.go",
  "\t\tif spec.blankOK && len(sc.Bytes()) == 0 {\n", "\t\tif len(sc.Bytes()) == 0 {\n", "TestRubricTEST13_")
