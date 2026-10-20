# The literal ROW_TOKEN scan finds a row token by its spelling, so a regex
# that reaches the same real test through a metacharacter wedged between its
# letters -- TestRE[C]49_, Test(REC)49_ -- names no literal "TestREC" and the
# scan finds nothing to flag. Matching the regex against the real test corpus
# is the only way to catch that a mutant pins a different row than the one it
# claims, and it needs the complete set, so dropping it gives the bypass back.
m("the row check matches obfuscated regexes against real tests", "test/mutation/sweep.py",
  "            if go_tests is not None and rx is not None:\n",
  "            if go_tests is not None and rx is not None and False:\n",
  "TestDEL2f_")
