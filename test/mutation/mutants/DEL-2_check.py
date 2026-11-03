# DEL-2: --check refuses an anchor that occurs other than exactly once, and a
# mutant that names another row's test without a declared shared pair.
m("DEL-2 --check accepts an anchor that occurs more than once", "test/mutation/sweep.py",
  "        count = s.count(old)\n        if count != 1:\n",
  "        count = s.count(old)\n        if count < 1:\n", "TestDEL2_")
m("DEL-2 --check accepts a mutant that pins another row's test", "test/mutation/sweep.py",
  "                if pinned != claimed and (claimed, parent_test) not in shared:\n",
  "                if False and (claimed, parent_test) not in shared:\n", "TestDEL2b_")
m("DEL-2 --check accepts an import anchor that occurs more than once", "test/mutation/sweep.py",
  "        count = s.count(io_)\n        if count != 1:\n", "        count = s.count(io_)\n        if count < 1:\n", "TestDEL2_")
# Import drift (MASKED-BUILD1): --check refuses a break that leaves an import
# unused and an import edit that drops a package still used, reading uses
# outside comments and literals and imports from the header alone.
m("DEL-2 --check accepts a break that leaves an import unused", "test/mutation/sweep.py",
  "            if used.get(pkg) and not used_after.get(pkg):\n", "            if False:\n", "TestDEL2g_")
m("DEL-2 --check accepts an import edit that drops a package still used", "test/mutation/sweep.py",
  "            if used_after.get(pkg):\n", "            if False:\n", "TestDEL2g_")
m("DEL-2 --check reads a package named in a comment as a use", "test/mutation/sweep.py",
  "GO_LEX = re.compile(r'//[^\\n]*|", "GO_LEX = re.compile(r'", "TestDEL2g_")
m("DEL-2 --check reads a body's string literal as an import", "test/mutation/sweep.py",
  "    head = src[:go_header(src)]\n", "    head = src\n", "TestDEL2g_")
m("DEL-2 --check keeps the untouched imports after an import edit", "test/mutation/sweep.py",
  " or s.index(old) <= pristine[f][2]\n", " or False\n", "TestDEL2g_")
# Changed rows build (MASKED-BUILD2): buildchanged.py compiles every row a
# change adds or edits, its IMPORTS entry included, refuses one that does not
# build, judges no row the change leaves alone, and puts each target back.
B = "test/mutation/buildchanged.py"
m("DEL-2 a changed row that does not build passes", B,
  "    return 1 if bad else 0\n", "    return 0\n", "TestDEL2h_")
m("DEL-2 an IMPORTS entry alone is no change", B,
  "(rows[n], imports.get(n)) != (brows.get(n), bimports.get(n))", "rows[n] != brows.get(n)", "TestDEL2h_")
m("DEL-2 every row is compiled, changed or not", B,
  "if (rows[n], imports.get(n)) != (brows.get(n), bimports.get(n))\n", "if True\n", "TestDEL2h_")
m("DEL-2 a target is not put back", B,
  "            subprocess.run([\"git\", \"checkout\", \"-q\", \"--\", *sorted(set(touched))])\n", "            pass\n",
  "TestDEL2h_")
m("DEL-2 a target with changes is edited anyway", B,
  "    if targets and dirty.strip():\n", "    if False:\n", "TestDEL2i_")
