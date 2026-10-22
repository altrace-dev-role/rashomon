# REC-42: a steer line is appended to runs/<dir>/steer.ndjson, ordered inside
# its lock and covered by the one full sync, before anything reaches stdout;
# the lock waits 150 ms and then writes nothing; the caller's decision and
# every line it admits share one hold; the lines land together or not at all;
# a text still over its cap writes nothing; a read the lock refuses is skipped.
S = "internal/store/steerlog.go"
m("REC-42 the steer append is never ordered", S,
  "\treturn s.syncAppend(f)\n}", "\treturn nil\n}", "TestREC42_")
m("REC-42 the steer append is ordered after the lock is released", S,
  "\treturn s.syncAppend(f)\n}", "\tunlock()\n\treturn s.syncAppend(f)\n}", "TestREC42_")
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
  "\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\tbatch = append(batch, line...)\n",
  "\t\tif err != nil {\n\t\t\tcontinue\n\t\t}\n\t\tbatch = append(batch, line...)\n", "TestREC42c_")
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
CAP = "\t\tif l.CapText() {\n\t\t\treturn nil, ErrSteerTextWithheld\n\t\t}\n"
m("REC-42 a line is written without its cap", S, CAP, "", "TestREC42g_")
m("REC-42 an uncapped text is capped and written", S,
  CAP, "\t\tl.CapText()\n", "TestREC42g_")
m("REC-42 an uncapped text drops only its own line", S,
  CAP, "\t\tif l.CapText() {\n\t\t\tcontinue\n\t\t}\n", "TestREC42g_")
m("REC-42 a steer read the lock refused fails the read", S,
  "refusedAsSkipped(eachLine(filepath.Join(s.RunDir(sessionID), FileSteer), fn), s.refusal(), fn)",
  "eachLine(filepath.Join(s.RunDir(sessionID), FileSteer), fn)", "TestREC42h_")
m("REC-42 the steer read ignores the store's refusal predicate", S,
  "FileSteer), fn), s.refusal(), fn)", "FileSteer), fn), readRefused, fn)", "TestREC42h_")
