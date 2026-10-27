# RES-36: an entry row's alloc_bytes_max is the measured B/op of the
# benchmark row it names. A figure above the measurement, or a benchmark
# name with no row, leaves the entry's allocation budget unbacked.
m("RES-36 an entry row's allocation budget drifts above its benchmark", "test/perf/budgets.json",
  '"alloc_bytes_max": 55424,', '"alloc_bytes_max": 60967,', "TestRES36f_")
m("RES-36 an entry row names a benchmark with no row", "test/perf/budgets.json",
  '"benchmark": "BenchmarkRecap/100",', '"benchmark": "BenchmarkRecap/200",', "TestRES36f_")
