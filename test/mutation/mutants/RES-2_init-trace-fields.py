# RES-2: an init trace line of ten fields with "bytes," in place lacks its
# allocs word; reading index 10 of it panicked the gate (P08-13). The bound
# is the line's length, eleven fields.
m("RES-2 the init trace parser reads past a ten-field line", "test/perf/gate.go",
  "\t\t\tif len(f) < 11 || f[8] != \"bytes,\" || f[10] != \"allocs\" {\n",
  "\t\t\tif len(f) < 10 || f[8] != \"bytes,\" || f[10] != \"allocs\" {\n", "TestRES2b_")
