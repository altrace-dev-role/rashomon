# DEL-17: every CI job carries timeout-minutes. Removing one job's bound is the
# whole regression; the test reads the workflow files line by line.
m("DEL-17 a CI job has no timeout", ".github/workflows/ci.yaml",
  "    timeout-minutes: 15\n", "", "TestDEL17_")
# The parser must fail closed on a line at an indent it does not recognise
# under jobs:, rather than stepping over it -- which is what let a job key
# typed at three spaces silently credit its own timeout to the job above.
m("DEL-17 the timeout parser steps over a misindented line instead of flagging it", "test/mutation/sweep_test.go",
  "\t\tif indent%2 != 0 {\n", "\t\tif false {\n", "TestDEL17b_")
