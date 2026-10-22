# RES-29: each pathological input the shape of a call reads exits 0 inside
# the hook's hard stop at peak RSS of at most 64 MiB, through the hook entry
# in a child process. The RES-9 mutant turns the first row red (the spec's
# own pairing): with the token bound lifted, 8 MiB of short words is lexed
# whole.
m("RES-29 the token bound is lifted under the hook", "internal/shape/tokenize.go",
  "\tmaxTokens     = shapeBudget / bytesPerToken\n", "\tmaxTokens     = 1 << 30\n",
  "TestRES29_PathologicalInputsStayBounded")
# The child skipping the runtime pin -- calling cmdHook directly instead of
# entering through run() -- is not registered: it cannot fail this test. The
# 64 and 76 MiB budgets were measured before the pin existed (54.78 MiB
# linux/amd64, 65.11-65.25 darwin/arm64), so the bound holds either way. The
# pin itself is TRU-50's: run() applies it to each of the four hook entries.
# Register it again if the budget is re-measured under the pin and set below
# the unpinned peak.
