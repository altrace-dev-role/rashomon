# Rubric naming-4: --help names every event watch installs and every
# subcommand those entries run. It omitted UserPromptSubmit and
# `recap pending` while watch installed both.
m("naming-4 the watch line drops UserPromptSubmit", "cmd/rashomon/main.go",
  "                               Stop/StopFailure exception-only recap, and its\n                               UserPromptSubmit catch-up; --show prints that\n",
  "                               Stop/StopFailure exception-only recap; --show prints that\n",
  "TestRubricNAMING4_")
m("naming-4 recap pending is not listed", "cmd/rashomon/usage_hooks.go",
  "  rashomon recap pending [--install ID]\n",
  "  rashomon recap [--install ID]\n",
  "TestRubricNAMING4_")
