# RES-2: every hook-entry process (hook, post, probe, recap) pins GOGC and
# its soft memory limit, regardless of GOMEMLIMIT or GOGC in its
# environment. Losing one of the two pins, or dropping one of the four
# subcommands from the set the pin applies to, is the whole regression.
m("RES-2 recap no longer pins its runtime settings", "cmd/rashomon/main.go",
  '"hook": true, "post": true, "probe": true, "recap": true,\n',
  '"hook": true, "post": true, "probe": true,\n', "TestHookEntriesPinTheirRuntimeSettings")
m("RES-2 a hook entry process no longer pins GOGC", "cmd/rashomon/main.go",
  "\tdebug.SetGCPercent(hookGCPercent)\n\tdebug.SetMemoryLimit(hookMemoryLimitBytes)\n",
  "\tdebug.SetMemoryLimit(hookMemoryLimitBytes)\n", "TestHookEntriesPinTheirRuntimeSettings")
m("RES-2 a hook entry process no longer pins its memory limit", "cmd/rashomon/main.go",
  "\tdebug.SetGCPercent(hookGCPercent)\n\tdebug.SetMemoryLimit(hookMemoryLimitBytes)\n",
  "\tdebug.SetGCPercent(hookGCPercent)\n", "TestHookEntriesPinTheirRuntimeSettings")
