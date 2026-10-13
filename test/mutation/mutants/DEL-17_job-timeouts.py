# DEL-17: every CI job carries timeout-minutes. Removing one job's bound is the
# whole regression; the test reads the workflow files line by line.
m("DEL-17 a CI job has no timeout", ".github/workflows/ci.yaml",
  "    timeout-minutes: 15\n", "", "TestDEL17_")
# The parser must fail closed on a line it does not recognise under jobs:.
m("DEL-17 the timeout parser steps over a line it does not recognise", "test/mutation/sweep_test.go",
  "\t\tif twoSpaces.MatchString(ln) {\n", "\t\tif false && twoSpaces.MatchString(ln) {\n", "TestDEL17b_")
