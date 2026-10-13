# perf-7: status prints the store's size against the cap in force.
m("perf-7 status does not print the store's size", "cmd/rashomon/main.go",
  "\t\tstatusSize(stdout, st)\n", "", "TestRubricPERF7_StatusShowsTheStoreAgainstItsCap")
m("perf-7 status does not say eviction is off", "cmd/rashomon/status_store.go",
  "\tif c.Off() {\n", "\tif false {\n", "TestRubricPERF7_StatusShowsTheStoreAgainstItsCap")
m("perf-7 the size is not the one eviction compares", "internal/store/capbytes.go",
  "\treturn plan.total, errors.Join(plan.unlisted...)\n", "\treturn plan.total / 2, errors.Join(plan.unlisted...)\n",
  "TestRubricPERF7_RunsBytesIsWhatTheCapCounts")
