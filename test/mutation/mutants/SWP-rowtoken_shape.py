# A Test<PREFIX> token in a row file's test regex must fully match
# Test<PREFIX><n>[a-z]?_. Scanning for ROW_TEST matches directly, the old way,
# finds nothing at all for a token that only looks like a row test -- a regex
# metacharacter, a missing underscore, a bare anchor -- so no problem is ever
# reported for it. That is the REC-56 bypass TestDEL2b pins.
m("the scan finds only already-well-formed row tokens", "test/mutation/sweep.py",
  "            for tok in ROW_TOKEN.finditer(test):\n"
  "                t = ROW_TEST.match(test, tok.start())\n"
  "                if t is None:\n"
  "                    problems.append(f\"{name!r} in {rel(ORIGIN[name])}: {test!r} has a Test{tok.group(1)} token \"\n"
  "                                    f\"at {tok.start()} that is not Test<PREFIX><n>[a-z]?_\")\n"
  "                    continue\n",
  "            for t in ROW_TEST.finditer(test):\n",
  "TestDEL2b_")
