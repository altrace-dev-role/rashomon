# The import-based scan cannot see a raw syscall.Socket or os.StartProcess
# call, because syscall and os are themselves legitimately imported here
# (file locks, signal handling). This is the list that closes the gap, and
# the unit test pins each entry against its own fixture regardless of
# whether this tree's source currently contains such a call.
m("syscall.Exec is dropped from the raw-syscall scan", "test/acceptance/h17_no_egress_test.go",
  '"syscall.ForkExec(", "syscall.Exec(",', '"syscall.ForkExec(",',
  "TestRawSyscalls_CatchesEachListedCall")
m("os.StartProcess is dropped from the raw-syscall scan", "test/acceptance/h17_no_egress_test.go",
  '"syscall.Exec(",\n\t"os.StartProcess(",', '"syscall.Exec(",',
  "TestRawSyscalls_CatchesEachListedCall")
m("syscall.StartProcess is dropped from the raw-syscall scan", "test/acceptance/h17_no_egress_test.go",
  '"os.StartProcess(", "syscall.StartProcess(",', '"os.StartProcess(",',
  "TestRawSyscalls_CatchesEachListedCall")
m("syscall.RawSyscall is dropped from the raw-syscall scan", "test/acceptance/h17_no_egress_test.go",
  '"syscall.StartProcess(", "syscall.RawSyscall(",', '"syscall.StartProcess(",',
  "TestRawSyscalls_CatchesEachListedCall")
m("syscall.Syscall is dropped from the raw-syscall scan", "test/acceptance/h17_no_egress_test.go",
  '"syscall.RawSyscall(", "syscall.Syscall(",', '"syscall.RawSyscall(",',
  "TestRawSyscalls_CatchesEachListedCall")
# The store's fcntl barrier is the one sanctioned raw call, by its exact
# text: the same trap with another number is a raw call like any other.
m("the sanctioned fcntl barrier becomes a socket trap", "internal/store/sync_darwin.go",
  "syscall.Syscall(syscall.SYS_FCNTL, fd, fBarrierFsync, 0)",
  "syscall.Syscall(syscall.SYS_SOCKET, fd, fBarrierFsync, 0)",
  "TestH17_NoSourceCallsASocketOrStartsAProcessOutsideASyscallImport")
