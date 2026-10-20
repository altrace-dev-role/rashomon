# --strict exists so CI can tell a skipped judging test from a pass. Without
# this, a mutant whose test environment broke would exit 0 same as a clean
# sweep, in the one place that is supposed to catch it.
m("--strict ignores a mutant that was never judged", "test/mutation/sweep.py",
  "    return 1 if undetected or (strict and skipped) else 0\n",
  "    return 1 if undetected else 0\n",
  "TestStrict_")
