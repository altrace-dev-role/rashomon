# test-5: the store's durability boundaries each have a fault point and a
# test that a crash there loses nothing it should keep: the seq counter's
# recovery and order, forget's in-place rewrite, and the baseline and recap
# renames.
m("test-5 a lost or garbled seq counter restarts at 1", "internal/store/store.go",
  "\t\tlast, err = maxSeq(f)\n\t\tif err != nil {\n\t\t\treturn 0, err\n\t\t}\n",
  "\t\tlast, err = 0, nil\n", "TestRubricTEST5_ALostOrGarbledSeqFileNeverRepeatsASeq")
m("test-5 the seq counter is written after the point a crash can land", "internal/store/store.go",
  "\tif err := writeSmall(path, []byte(strconv.FormatInt(next, 10)+\"\\n\"), sync); err != nil {\n\t\treturn 0, err\n\t}\n\tfault.Inject(fault.PointStoreSeqAdvanced)\n",
  "\tfault.Inject(fault.PointStoreSeqAdvanced)\n\tif err := writeSmall(path, []byte(strconv.FormatInt(next, 10)+\"\\n\"), sync); err != nil {\n\t\treturn 0, err\n\t}\n",
  "TestRubricTEST5_APanicAfterTheSeqAdvancesSkipsOneNumber")
m("test-5 forget's rewrite truncates before it writes", "internal/store/gaps.go",
  "\tvar off int64\n\tfor _, line := range lines {\n",
  "\tif err := f.Truncate(0); err != nil {\n\t\treturn err\n\t}\n\tvar off int64\n\tfor _, line := range lines {\n",
  "TestRubricTEST5_ACrashInForgetsRewriteLosesNoKeptRecord")
m("test-5 the baseline is written in place before its rename", "internal/baseline/baseline.go",
  "\tfault.Inject(fault.PointBaselineBeforeRename)\n",
  "\tif err := os.WriteFile(path, body, 0o600); err != nil {\n\t\treturn err\n\t}\n\tfault.Inject(fault.PointBaselineBeforeRename)\n",
  "TestRubricTEST5_ACrashBeforeTheBaselineRenameKeepsTheOldOne")
m("test-5 recap's state is written in place", "internal/recap/state.go",
  "\ttmp := path + \".tmp\"\n", "\ttmp := path\n", "TestRubricTEST5_ACrashBeforeTheRecapRenameKeepsTheOldState")
