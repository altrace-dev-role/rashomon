# test-7: a syntax error's place is encoding/json's Offset minus one, the
# first byte after the one the scanner stopped on read back one byte early.
# Losing that minus one, or skipping the independent oracle's own minus one
# the same way, moves every syntax-error position by one byte.
m("test-7 refusal drops Offset's minus one", "internal/settings/errors.go",
  "\t\t\tat = posAt(max(int(syn.Offset)-1, 0))\n",
  "\t\t\tat = posAt(int(syn.Offset))\n", "TestRubricTest10_RoundTripSeeds")
