# COLD-PLATFORM1: a cold entry is gated against its platform's own
# cold_alloc_bytes_max when it has one, else the single top-level figure.
# Dropping the lookup gates every platform on the top-level figure (the
# flaky macOS gate again); dropping the fallback gates an unlisted entry
# against nothing.
m("COLD-PLATFORM1 the platform's own cold figure is ignored", "test/perf/gate.go",
  "\tif v, ok := overrides[e.ID]; ok {\n\t\treturn v\n\t}\n\treturn e.ColdAllocBytesMax\n",
  "\treturn e.ColdAllocBytesMax\n", "TestCOLDPLATFORM1_")
m("COLD-PLATFORM1 an entry with no figure of its own gets none", "test/perf/gate.go",
  "\treturn e.ColdAllocBytesMax\n}\n\n// PerProcessColdAllocBytes",
  "\treturn 0\n}\n\n// PerProcessColdAllocBytes", "TestCOLDPLATFORM1_")
