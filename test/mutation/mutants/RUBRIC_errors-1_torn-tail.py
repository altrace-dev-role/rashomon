# errors-1: every append that holds its file's lock starts a line of its own
# after a torn tail and cuts itself back when its write fails; a lone
# terminal is blamed on the lock only when it says so itself. records.ndjson's
# heal is REC-32's (REC-32_reader-limits.py); this covers the coverage, spill
# and gaps files and eviction's gap.
m("errors-1 a locked line append goes on after a torn tail", "internal/store/store.go",
  "\t\terr = appendHealed(f, line, onTimeout == lockOrFail)\n",
  "\t\t_, err = f.Write(line)\n",
  "TestRubricERRORS1_TornTails")
m("errors-1 eviction's gap goes on after a torn tail", "internal/store/evict.go",
  "\t\tif err := appendHealed(gf, line, true); err != nil {", "\t\tif _, err := gf.Write(line); err != nil {",
  "TestRubricERRORS1_TornTails")
m("errors-1 no append looks at the last byte", "internal/store/tail.go",
  "\t\tif last[0] != '\\n' {\n", "\t\tif false {\n", "TestRubricERRORS1_TornTails")
m("errors-1 a failed append is left on disk", "internal/store/tail.go",
  "\tif n == 0 {\n\t\treturn nil\n\t}\n", "\tif n >= 0 {\n\t\treturn nil\n\t}\n", "TestRubricERRORS1_AFailedAppend")
m("errors-1 every lone terminal is blamed on the lock", "internal/report/dropped.go",
  "\t\tif t.Reason != nil && *t.Reason == store.ReasonLockTimeout {\n", "\t\tif true {\n",
  "TestRubricERRORS1_OnlyALockTimeout")
m("errors-1 the digest blames every lone terminal on the lock", "internal/digest/coverage.go",
  "\t\tfor _, r := range report.DroppedReasons(run, turnDropped(run, w)) {\n\t\t\ttc.add(r)\n\t\t}\n",
  "\t\tif len(turnDropped(run, w)) > 0 {\n\t\t\ttc.add(store.ReasonLockTimeout)\n\t\t}\n",
  "TestRubricERRORS1_OnlyALockTimeout")
