# A suffixed test, TestSTR82b_, pins its parent row just as the unsuffixed
# one does, so a shared pair declared for the parent should cover it too.
# Looking the literal suffixed token up instead of the parent's own name
# rejects it.
m("a shared pair is not normalised to its parent test name", "test/mutation/sweep.py",
  "                parent_test = f\"Test{t.group(1)}{t.group(2)}_\"\n",
  "                parent_test = t.group(0)\n",
  "TestDEL2b_")
