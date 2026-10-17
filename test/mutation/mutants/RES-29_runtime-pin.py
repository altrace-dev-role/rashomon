# RES-29: the pathological-input harness re-execs cmdHook directly,
# bypassing run()'s dispatch -- so it must apply the same
# applyHookRuntimeLimits() pin a real hook-entry process runs under, or its
# 64 MiB RSS budget is measured under whatever the child process inherited,
# not the pin.
m("RES-29 the child skips the hook-entry runtime pin", "cmd/rashomon/res29_test.go",
  "\t\tapplyHookRuntimeLimits()\n\t\tos.Exit(cmdHook(strings.Fields(args), os.Stdin, os.Stderr))\n",
  "\t\tos.Exit(cmdHook(strings.Fields(args), os.Stdin, os.Stderr))\n", "TestRES29_PathologicalInputsStayBounded")
