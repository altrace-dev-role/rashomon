# TRU-50: hook entries ignore the environment, and the argv token alone does
# not make a process the supervised child: the parent's pipe at fd 3, holding
# a whole clock record, does.
m("TRU-50 the argv token alone makes a child", "internal/supervise/run_unix.go",
  "\tstart, stop, valid := parseClockRecord(buf[:n])\n\tif !valid {\n\t\treturn nil, rest, false\n\t}\n",
  "\tstart, stop, valid := parseClockRecord(buf[:n])\n\tif !valid {\n\t\treturn &Child{clock: budget.NewClock(time.Now(), time.Hour)}, rest, true\n\t}\n",
  "TestTRU50_")
m("TRU-50 a clock record with trailing bytes reads back", "internal/supervise/supervise.go",
  "\tif !found || len(rest) != 0 {", "\tif !found || len(rest) < 0 {", "TestTRU50b_")
