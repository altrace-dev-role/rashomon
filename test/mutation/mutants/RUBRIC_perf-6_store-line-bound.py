# perf-6: initialScanBuffer is where a store read starts, not a size every
# record fits; a record over it grows the buffer (rubric fix list entry 68).
m("perf-6  a store read stops at the initial buffer", "internal/store/read.go",
  "\tsc.Buffer(make([]byte, 0, initialScanBuffer), maxLine)\n",
  "\tsc.Buffer(make([]byte, 0, initialScanBuffer), initialScanBuffer)\n", "TestRubricPerf6_")
