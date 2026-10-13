# DEL-17: every CI job carries timeout-minutes. Removing one job's bound is the
# whole regression; the test reads the workflow files line by line.
m("DEL-17 a CI job has no timeout", ".github/workflows/ci.yaml",
  "    timeout-minutes: 15\n", "", "TestDEL17_")
