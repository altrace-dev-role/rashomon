# A shared-tests.txt entry naming a suffixed test (TestSTR4b_) passed the
# shape check, but the pair lookup is keyed on the parent's name, so the
# entry could never match a mutant: --check now asks for the parent.
m("a shared-tests.txt entry may name a suffixed test", "test/mutation/sweep.py",
  "        if parts[1] != f\"Test{named.group(1)}{named.group(2)}_\":\n",
  "        if False:\n",
  "TestSharedTestsEntryNamesTheParent")
