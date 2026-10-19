# STR-6: the hard stop is the installed timeout less its margin, the child's
# whole process group dies at it, and a child whose parent is gone, or whose
# own hard stop has passed, starts no new append. The supervisor's own line
# takes no sync, so a burst of failures does not queue behind it.
m("STR-6 the kill reaches the child but not its group", "internal/supervise/run_unix.go",
  "\tsyscall.Kill(-pid, syscall.SIGKILL)", "\tsyscall.Kill(pid, syscall.SIGKILL)", "TestSTR6_")
m("STR-6 the hard stop keeps no margin at 5 s and above", "internal/supervise/supervise.go",
  "\t\treturn budget - longMargin\n", "\t\treturn budget\n", "TestSTR6c_")
m("STR-6 the hard stop keeps no margin on a short budget", "internal/supervise/supervise.go",
  "\t\treturn budget - shortMargin\n", "\t\treturn budget\n", "TestSTR6c_")
m("STR-6 the child never sees its parent go", "internal/supervise/run_unix.go",
  "\tif n == 0 && err == nil {\n", "\tif false && n == 0 && err == nil {\n", "TestSTR6d_")
m("STR-6 the child ignores its own hard stop", "internal/supervise/run_unix.go",
  "\treturn c.parentGone() || c.clock.Expired(time.Now())", "\treturn c.parentGone()", "TestSTR6e_")
m("STR-6 a stood-down store still appends lines", "internal/store/store.go",
  "func (s *Store) appendLine(path string, rec any, o appendOpts) error {\n\tif s.stoodDown() {\n\t\treturn ErrStoodDown\n\t}\n",
  "func (s *Store) appendLine(path string, rec any, o appendOpts) error {\n", "TestSTR6b_")
m("STR-6 a stood-down store still evicts", "internal/store/gaps.go",
  "\tif s.stoodDown() {\n\t\treturn nil, ErrStoodDown\n", "\tif false {\n\t\treturn nil, ErrStoodDown\n", "TestSTR6b_")
m("STR-6 eviction asks once, not before each run", "internal/store/gaps.go",
  "\t\tif s.stoodDown() {\n\t\t\treturn gaps, ErrStoodDown\n", "\t\tif false {\n\t\t\treturn gaps, ErrStoodDown\n", "TestSTR6g_")
m("STR-6 the supervisor line is synced behind a barrier", "internal/store/supervisor.go",
  "sealTail: true, noSync: true, capBytes: SupervisorLogCap", "sealTail: true, capBytes: SupervisorLogCap", "TestSTR6f_")
