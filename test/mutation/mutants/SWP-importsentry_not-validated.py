# An IMPORTS entry that is not a 3-tuple of strings reaches check()'s
# unpacking loop and crashes with an unhandled ValueError instead of being
# reported as a check problem.
m("an IMPORTS entry's shape is not validated", "test/mutation/sweep.py",
  "            if not (isinstance(entry, tuple) and len(entry) == 3 and all(isinstance(x, str) for x in entry)):\n"
  "                problems.append(f\"{rel(path)}: IMPORTS[{name!r}] is {entry!r}, not a 3-tuple of strings\")\n"
  "                continue\n",
  "",
  "TestDEL1b_")
