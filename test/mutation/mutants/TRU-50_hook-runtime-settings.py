# TRU-50/RD-5: every hook-entry process (hook, post, probe, recap) pins
# GOGC and its soft memory limit, regardless of GOMEMLIMIT or GOGC in its
# environment. Losing one of the two pins, or dropping one of the four
# subcommands from the set the pin applies to, is the whole regression.
# Filed under TRU-50/RD-5 (the soft-memory-limit requirement), not RES-2
# (RES-2 is the process-init allocation budget, a different row).
m("TRU-50 recap no longer pins its runtime settings", "cmd/rashomon/main.go",
  '"hook": true, "post": true, "probe": true, "recap": true,\n',
  '"hook": true, "post": true, "probe": true,\n', "TestHookEntriesPinTheirRuntimeSettings")
m("TRU-50 a hook entry process no longer pins GOGC", "cmd/rashomon/main.go",
  "\tdebug.SetGCPercent(hookGCPercent)\n\tdebug.SetMemoryLimit(hookMemoryLimitBytes)\n",
  "\tdebug.SetMemoryLimit(hookMemoryLimitBytes)\n", "TestHookEntriesPinTheirRuntimeSettings")
m("TRU-50 a hook entry process no longer pins its memory limit", "cmd/rashomon/main.go",
  "\tdebug.SetGCPercent(hookGCPercent)\n\tdebug.SetMemoryLimit(hookMemoryLimitBytes)\n",
  "\tdebug.SetGCPercent(hookGCPercent)\n", "TestHookEntriesPinTheirRuntimeSettings")
