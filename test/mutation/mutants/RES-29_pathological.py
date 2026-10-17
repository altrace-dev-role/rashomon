# RES-29: each pathological input the shape of a call reads exits 0 inside
# the hook's hard stop at peak RSS of at most 64 MiB, through the hook entry
# in a child process. The RES-9 mutant turns the first row red (the spec's
# own pairing): with the token bound lifted, 8 MiB of short words is lexed
# whole.
m("RES-29 the token bound is lifted under the hook", "internal/shape/tokenize.go",
  "\tmaxTokens     = shapeBudget / bytesPerToken\n", "\tmaxTokens     = 1 << 30\n",
  "TestRES29_PathologicalInputsStayBounded")
