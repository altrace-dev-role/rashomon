# DEL-2: --check refuses an anchor that occurs other than exactly once, and a
# mutant that names another row's test without a declared shared pair.
m("DEL-2 --check accepts an anchor that occurs more than once", "test/mutation/sweep.py",
  "        elif count != 1:\n", "        elif count < 1:\n", "TestDEL2_")
m("DEL-2 --check accepts a mutant that pins another row's test", "test/mutation/sweep.py",
  "                if pinned != claimed and (claimed, t.group(0)) not in shared:\n",
  "                if False and (claimed, t.group(0)) not in shared:\n", "TestDEL2b_")
m("DEL-2 --check accepts an import anchor that occurs more than once", "test/mutation/sweep.py",
  "        count = s.count(io_)\n        if count != 1:\n", "        count = s.count(io_)\n        if count < 1:\n", "TestDEL2_")
# The legacy list may only shrink, and it is empty; one entry is the whole regression.
m("DEL-2 the legacy list grows", "test/mutation/legacy-duplicate-anchors.txt",
  "# anchor, and TestDEL2c keeps this list empty.\n",
  "# anchor, and TestDEL2c keeps this list empty.\nH-15 forget deletes without a gap record\n", "TestDEL2c_")
