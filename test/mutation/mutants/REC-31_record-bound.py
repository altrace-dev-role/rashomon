# REC-31: the record bound is derived from the caps capture enforces, and a
# record over MaxRecordBytes is refused before any of it is written, leaving
# record_oversize on the call's coverage. A call over the record limit
# degrades first: its oversize values are withheld at capture, a record that
# skipped the caps has its capped fields withheld by the store, and only a
# record still over the limit is refused.
m("REC-31 marshalLine has no size check", "internal/store/store.go",
  "\tif len(line)+1 > MaxRecordBytes {\n\t\treturn nil, ErrRecordOversize\n\t}\n", "", "TestREC31_")
m("REC-31 the limit leaves out the newline", "internal/store/store.go",
  "\tif len(line)+1 > MaxRecordBytes {\n", "\tif len(line) > MaxRecordBytes {\n", "TestREC31_OversizeRecordRefused")
m("REC-31 the derivation leaves out the host lists", "internal/store/limits.go",
  "\t\t2*MaxHosts*MaxHostBytes + MaxCommandBytes + MaxWrittenPathsBytes +\n",
  "\t\tMaxCommandBytes + MaxWrittenPathsBytes +\n", "TestREC31_WorstCaseRecordFits")
m("REC-31 the derivation leaves out the command and written paths", "internal/store/limits.go",
  "\t\t2*MaxHosts*MaxHostBytes + MaxCommandBytes + MaxWrittenPathsBytes +\n",
  "\t\t2*MaxHosts*MaxHostBytes +\n", "TestREC31_WorstCaseRecordFits")
m("REC-31 the derivation leaves out the marker spans", "internal/store/limits.go",
  "\treturn jsonEscapeFactor*capped + MaxMaskSpans*MaxMaskSpanBytes + fixedRecordBytes\n",
  "\treturn jsonEscapeFactor*capped + fixedRecordBytes\n", "TestREC31_WorstCaseRecordFits")
m("REC-31 the derivation counts one path", "internal/store/limits.go",
  "const cappedPaths = 4\n", "const cappedPaths = 1\n", "TestREC31_WorstCaseRecordFits")
m("REC-31 the derivation assumes JSON escapes nothing", "internal/store/limits.go",
  "const jsonEscapeFactor = 6\n", "const jsonEscapeFactor = 1\n", "TestREC31_WorstCaseRecordFits")
m("REC-31 a refused record reads as an internal error", "internal/hook/payload.go",
  "\tcase errors.Is(err, store.ErrRecordOversize):\n\t\treturn store.ReasonRecordOversize\n", "", "TestREC31b_")
m("REC-31 an oversize call is written whole, not degraded", "internal/hook/handle.go",
  "\tif h.capDecl(&decl) {\n\t\th.withheld = true\n\t}\n", "", "TestREC31_OversizeDegrades")
m("REC-31 the store refuses instead of degrading", "internal/store/store.go",
  "\tsmaller, withheld := d.degraded()\n\tif !withheld {\n", "\tsmaller, withheld := d.degraded()\n\tif withheld || !withheld {\n",
  "TestREC31c_")
m("REC-31 a degraded record reads as written whole", "internal/store/store.go",
  "\treturn line, ErrRecordDegraded, nil\n", "\treturn line, nil, nil\n", "TestREC31c_")
m("REC-31 a degraded ordered append reads as written whole", "internal/store/store.go",
  "\t\treturn err\n\t}\n\treturn degraded\n}\n\n// lockAppend takes", "\t\treturn err\n\t}\n\t_ = degraded\n\treturn nil\n}\n\n// lockAppend takes",
  "TestREC31c_")
m("REC-31 a degraded spilled append reads as written whole", "internal/store/store.go",
  "\t\treturn err\n\t}\n\treturn degraded\n}\n\n// fitLine", "\t\treturn err\n\t}\n\t_ = degraded\n\treturn nil\n}\n\n// fitLine",
  "TestREC31c_")
m("REC-31 the degrade withholds the command without saying so", "internal/store/limits.go",
  "\t\td.Command, d.CommandCut, d.Masked, d.CommandWithheld = nil, nil, nil, w.oversize()\n",
  "\t\td.Command, d.CommandCut, d.Masked, w.withheld = nil, nil, nil, true\n", "TestREC31c_")
m("REC-31 the degrade marks an empty field withheld", "internal/store/limits.go",
  "\td.Project = w.drop(d.Project, &d.ProjectWithheld)\n", "\td.Project, d.ProjectWithheld = nil, w.oversize()\n",
  "TestREC31c_")
m("REC-31 the degrade keeps the executed command", "internal/store/limits.go",
  "\tif x.ExecutedCommand != nil {\n\t\tx.ExecutedCommand, x.ExecutedCommandCut, x.ExecutedMasked = nil, nil, nil\n",
  "\tif false {\n\t\tx.ExecutedCommand, x.ExecutedCommandCut, x.ExecutedMasked = nil, nil, nil\n", "TestREC31c_")
m("REC-31 the hook takes a degraded record as a failed append", "internal/hook/payload.go",
  "\tif errors.Is(err, store.ErrRecordDegraded) {\n", "\tif false {\n", "TestREC31d_")
m("REC-31 a degraded record leaves the call verified", "internal/hook/payload.go",
  "\t\t*withheld = true\n\t\treturn nil\n", "\t\treturn nil\n", "TestREC31d_")
m("REC-31 the derivation counts span rule ids unescaped", "internal/store/limits.go",
  "\tMaxMaskSpanBytes = MaxMaskRuleBytes*jsonEscapeFactor + maskSpanFrameBytes\n",
  "\tMaxMaskSpanBytes = MaxMaskRuleBytes + maskSpanFrameBytes\n", "TestREC31_WorstCaseRecordFits")
m("REC-31 the derivation leaves out a span's keys and offsets", "internal/store/limits.go",
  "const maskSpanFrameBytes = 38\n", "const maskSpanFrameBytes = 0\n", "TestREC31_WorstCaseRecordFits")
m("REC-31 the hook takes a degraded declaration as a failed append", "internal/hook/handle.go",
  "\tif err := landed(h.st.AppendDeclaration(decl), &h.withheld); err != nil {\n",
  "\tif err := h.st.AppendDeclaration(decl); err != nil {\n", "TestREC31e_")
m("REC-31 the hook takes a degraded execution as a failed append", "internal/hook/post.go",
  "\treturn landed(err, &p.withheld)\n}\n",
  "\treturn err\n}\n", "TestREC31e_")
