# STR-4: a hook entry runs as a supervised child, and whatever the child does
# the entry exits 0, prints nothing it should not, and says only fixed lines.
# The gate is what stands between a child's stdout and Claude Code: one JSON
# object of the event's shape, byte for byte as it re-encodes, nothing only
# steering would print, and only from a clean exit.
m("STR-4 output is forwarded from a child that failed", "internal/supervise/run_unix.go",
  "\tcase r.outcome != \"\":\n\t\tcfg.fail(p, r, store.ReasonHookKilled, deadline)\n",
  "\tcase r.outcome != \"\":\n\t\tcfg.forward(p, out, deadline)\n\t\tcfg.fail(p, r, store.ReasonHookKilled, deadline)\n",
  "TestSTR4h_")
m("STR-4 the gate takes what does not re-encode byte for byte", "internal/supervise/gate.go",
  "\tif err != nil || !bytes.Equal(again, out) {", "\tif err != nil || len(again) == 0 {", "TestSTR4e_")
m("STR-4 the steer seam passes what only steering would print", "internal/supervise/gate.go",
  "\treturn o.HookSpecificOutput == nil\n}", "\treturn true\n}", "TestSTR4f_")
m("STR-4 every line of a child's stderr is relayed", "internal/supervise/run_unix.go",
  "\t\tif cfg.Relayed(string(line)) {\n", "\t\tif true {\n", "TestSTR4i_")
m("STR-4 the child's exit status is the entry's", "internal/supervise/run_unix.go",
  "func exitStatus(result) int { return 0 }", "func exitStatus(r result) int { return max(r.childExit, 0) }", "TestSTR4_")
m("STR-4 a spawn failure leaves the payload unread", "internal/supervise/run_unix.go",
  "\t\tdrained := safe.Go(func() { p.run(cfg.Stdin, nil, 0) }, nil)\n\t\twaitUntil(drained, deadline)\n",
  "\t\tdrained := safe.Go(func() {}, nil)\n\t\twaitUntil(drained, deadline)\n", "TestSTR4c_")
m("STR-4 a spawn failure is not recorded", "internal/supervise/run_unix.go",
  "\t\tcfg.fail(p, r, store.ReasonSupervisorSpawnFailed, deadline)\n", "", "TestSTR4c_")
m("STR-4 a hook entry runs unsupervised", "cmd/rashomon/supervised.go",
  "\treturn supervise.Run(cfg)\n", "\treturn cfg.Direct()\n", "TestSTR4_")
