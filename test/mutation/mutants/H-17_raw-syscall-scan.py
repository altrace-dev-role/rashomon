# The import-based scan cannot see a raw syscall.Socket or os.StartProcess
# call, because syscall and os are themselves legitimately imported here
# (file locks, signal handling). This is the list that closes the gap, and
# the unit test pins each entry against its own fixture regardless of
# whether this tree's source currently contains such a call.
m("os.StartProcess is dropped from the raw-syscall scan", "test/acceptance/h17_no_egress_test.go",
  '\t"os.StartProcess(",\n}', "}", "TestRawSyscalls_CatchesEachListedCall")
m("syscall.Exec is dropped from the raw-syscall scan", "test/acceptance/h17_no_egress_test.go",
  '"syscall.ForkExec(", "syscall.Exec(",', '"syscall.ForkExec(",',
  "TestRawSyscalls_CatchesEachListedCall")
