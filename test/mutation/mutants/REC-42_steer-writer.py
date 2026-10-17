# REC-42: a steer line is appended to runs/<dir>/steer.ndjson, ordered inside
# its lock and covered by the one full sync, before anything reaches stdout;
# the lock waits 150 ms and then writes nothing; the caller's decision and
# every line it admits share one hold; the lines land together or not at all.
S = "internal/store/steerlog.go"
m("REC-42 the steer append is never ordered", S,
  "\tif err := s.syncAppend(f); err != nil {\n\t\treturn err\n\t}\n\tif withheld {",
  "\tif withheld {", "TestREC42_")
m("REC-42 the steer append is ordered after the lock is released", S,
  "\tif err := s.syncAppend(f); err != nil {\n\t\treturn err\n\t}\n\tif withheld {",
  "\tunlock()\n\tif err := s.syncAppend(f); err != nil {\n\t\treturn err\n\t}\n\tif withheld {", "TestREC42_")
m("REC-42 the steer lock waits the records' budget", S,
  "s.lockWait(steerBudget)", "s.lockWait(lockBudget)", "TestREC42b_")
m("REC-42 the steer lock ignores the invocation's deadline", S,
  "s.lockWait(steerBudget)", "steerBudget", "TestREC42b_")
m("REC-42 a steer append that timed out writes anyway", S,
  "\tunlock, err := s.lockAppend(f, s.lockWait(steerBudget))\n\tif err != nil {\n\t\treturn err\n\t}\n\tdefer unlock()\n",
  "\tunlock, err := s.lockAppend(f, s.lockWait(steerBudget))\n\tif err == nil {\n\t\tdefer unlock()\n\t}\n",
  "TestREC42_|TestREC42b_")
m("REC-42 the caller decides before the lock is taken", S,
  "\tunlock, err := s.lockAppend(f, s.lockWait(steerBudget))\n\tif err != nil {\n\t\treturn err\n\t}\n\tdefer unlock()\n\tlines, err := build(f)\n\tif err != nil {\n\t\treturn err\n\t}\n",
  "\tlines, err := build(f)\n\tif err != nil {\n\t\treturn err\n\t}\n\tunlock, err := s.lockAppend(f, s.lockWait(steerBudget))\n\tif err != nil {\n\t\treturn err\n\t}\n\tdefer unlock()\n",
  "TestREC42c_")
m("REC-42 the caller is given no handle to read the log through", S,
  "\tlines, err := build(f)\n", "\tlines, err := build(nil)\n", "TestREC42c_")
m("REC-42 a failed decision still writes its lines", S,
  "\tlines, err := build(f)\n\tif err != nil {\n\t\treturn err\n\t}\n",
  "\tlines, _ := build(f)\n", "TestREC42c_")
m("REC-42 a line over the bound drops only itself", S,
  "\t\tif merr != nil {\n\t\t\treturn nil, false, merr\n\t\t}\n",
  "\t\tif merr != nil {\n\t\t\tcontinue\n\t\t}\n", "TestREC42c_")
m("REC-42 an append of no lines still owes a sync", S,
  "\tif err != nil || len(batch) == 0 {\n", "\tif err != nil {\n", "TestREC42c_")
m("REC-42 the steer append does not heal a torn tail", S,
  "\tif err := appendHealed(f, batch, true); err != nil {\n",
  "\tif _, err := f.Write(batch); err != nil {\n", "TestREC42d_")
m("REC-42 the steer reader returns a torn line", S,
  "!json.Valid(line)", "!json.Valid([]byte(`{}`))", "TestREC42d_")
m("REC-42 the steer log is readable by others", S,
  "FileSteer), appendFlags, fileMode)", "FileSteer), appendFlags, 0o644)", "TestREC42e_")
m("REC-42 the steer log is another file", S,
  'const FileSteer = "steer.ndjson"', 'const FileSteer = "steer.log"', "TestREC42e_")
m("REC-42 a line is written without its cap", S,
  "\t\tif l.CapText() {\n\t\t\twithheld = true\n\t\t}\n", "", "TestREC42g_")
m("REC-42 a withheld text reads as a clean append", S,
  "\tif withheld {\n\t\treturn ErrSteerTextWithheld\n\t}\n", "\tif withheld {\n\t\treturn nil\n\t}\n", "TestREC42g_")
