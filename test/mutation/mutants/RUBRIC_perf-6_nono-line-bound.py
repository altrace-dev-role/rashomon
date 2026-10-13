# perf-6: the nono trail's line bound is a named constant with its arithmetic,
# pinned at the bound and one byte over it (rubric fix list entry 68).
m("perf-6  the trail line bound is halved", "internal/nono/nono.go",
  "\tnonoMaxLine       = 8 << 20\n",
  "\tnonoMaxLine       = 4 << 20\n", "TestRubricPerf6_")
m("perf-6  the trail scanner reads one byte past nonoMaxLine", "internal/nono/nono.go",
  "\tsc.Buffer(make([]byte, 0, initialScanBuffer), nonoMaxLine)\n",
  "\tsc.Buffer(make([]byte, 0, initialScanBuffer), nonoMaxLine+1)\n", "TestRubricPerf6_")
