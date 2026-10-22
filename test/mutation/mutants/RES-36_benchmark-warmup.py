# RES-36: benchToolCalls' timed loop must not share an id with the one
# warm-up call that primes the same store first. A non-negative
# benchWarmupIndex is the whole regression: at 0 it collides with the
# loop's own first index.
m("RES-36 the warm-up id can collide with the timed loop's own first index", "cmd/rashomon/hook_bench_test.go",
  "benchWarmupIndex = -1\n", "benchWarmupIndex = 0\n", "TestBenchWarmupIndexIsDistinctFromEveryLoopIndex")
