# REC-31: the record bound is derived from the caps capture enforces, and a
# record over MaxRecordBytes is refused before any of it is written, leaving
# record_oversize on the call's coverage.
m("REC-31 marshalLine has no size check", "internal/store/store.go",
  "\tif len(line)+1 > MaxRecordBytes {\n\t\treturn nil, ErrRecordOversize\n\t}\n", "", "TestREC31_")
m("REC-31 the limit leaves out the newline", "internal/store/store.go",
  "\tif len(line)+1 > MaxRecordBytes {\n", "\tif len(line) > MaxRecordBytes {\n", "TestREC31_OversizeRecordRefused")
m("REC-31 the derivation leaves out the host lists", "internal/store/limits.go",
  " + MaxProgramBytes + 2*MaxHosts*MaxHostBytes\n", " + MaxProgramBytes\n", "TestREC31_WorstCaseRecordFits")
m("REC-31 the derivation assumes JSON escapes nothing", "internal/store/limits.go",
  "const jsonEscapeFactor = 6\n", "const jsonEscapeFactor = 1\n", "TestREC31_WorstCaseRecordFits")
m("REC-31 a refused record reads as an internal error", "internal/hook/payload.go",
  "\tcase errors.Is(err, store.ErrRecordOversize):\n\t\treturn store.ReasonRecordOversize\n", "", "TestREC31b_")
