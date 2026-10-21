# H-111: a hook entry whose stdout or stderr reader has gone away still exits
# 0. The disposition is set in one place and asked for at four entry points,
# so there is one break for each: hand SIGPIPE its default back, or leave one
# entry point without the call. Each call-site break names only the subtest
# for its own entry, so a call that moved to the wrong entry shows up as the
# entry it left behind. The last break goes the other way, setting the
# disposition for every command, which only the twin can see.
m("H-111 SIGPIPE keeps its default disposition", "internal/hook/brokenpipe_unix.go",
  "func IgnoreBrokenPipe() { signal.Ignore(syscall.SIGPIPE) }",
  "func IgnoreBrokenPipe() { signal.Reset(syscall.SIGPIPE) }",
  "TestH111_")
m("H-111 hook does not ignore SIGPIPE", "cmd/rashomon/main.go",
  "\t\thook.IgnoreBrokenPipe()\n\t\tsig := hook.WatchSignals()\n\t\tdefer sig.Stop()\n\n\t\tif checkPaused(args, stdin, store.PhaseCall, stderr) {",
  "\t\tsig := hook.WatchSignals()\n\t\tdefer sig.Stop()\n\n\t\tif checkPaused(args, stdin, store.PhaseCall, stderr) {",
  "TestH111_AnEntryWhoseReaderIsGoneStillExitsZero/^hook$")
m("H-111 post does not ignore SIGPIPE", "cmd/rashomon/main.go",
  "\t\thook.IgnoreBrokenPipe()\n\t\tsig := hook.WatchSignals()\n\t\tdefer sig.Stop()\n\n\t\tif checkPaused(args, stdin, store.PhasePost, stderr) {",
  "\t\tsig := hook.WatchSignals()\n\t\tdefer sig.Stop()\n\n\t\tif checkPaused(args, stdin, store.PhasePost, stderr) {",
  "TestH111_AnEntryWhoseReaderIsGoneStillExitsZero/^post$")
m("H-111 probe does not ignore SIGPIPE", "cmd/rashomon/main.go",
  "\t// stderr before the guarded body starts.\n\thook.IgnoreBrokenPipe()\n",
  "\t// stderr before the guarded body starts.\n",
  "TestH111_AnEntryWhoseReaderIsGoneStillExitsZero/^probe$")
m("H-111 recap does not ignore SIGPIPE", "cmd/rashomon/main.go",
  "\t\thook.IgnoreBrokenPipe()\n\t\tfault.Inject(fault.PointRecapStart)\n",
  "\t\tfault.Inject(fault.PointRecapStart)\n",
  "TestH111_AnEntryWhoseReaderIsGoneStillExitsZero/^recap$")
m("H-111 every command ignores SIGPIPE", "cmd/rashomon/main.go",
  "func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {\n\tif len(args) == 0 {",
  "func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {\n\thook.IgnoreBrokenPipe()\n\tif len(args) == 0 {",
  "TestH111_ACommandAPersonTypesKeepsTheDefault")
