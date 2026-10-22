# REC-52, the v5 fields: every field a payload can fill is capped at the
# store; at its cap it is kept, over it it is null and its *_withheld key
# says oversize. A command whose marker spans cannot describe it is withheld.
L = "internal/store/limits.go"
m("REC-52 v5 no cap on the command's length", L,
  "\tfits := len(*cmd) <= MaxCommandBytes && len(spans) <= MaxMaskSpans\n",
  "\tfits := len(spans) <= MaxMaskSpans\n", "TestREC52c_|TestREC52e_")
m("REC-52 v5 no cap on the span count", L,
  "\tfits := len(*cmd) <= MaxCommandBytes && len(spans) <= MaxMaskSpans\n",
  "\tfits := len(*cmd) <= MaxCommandBytes\n", "TestREC52d_")
m("REC-52 v5 no cap on a span's rule id", L,
  "\t\tif len(sp.Rule) > MaxMaskRuleBytes || sp.Start < 0 ||",
  "\t\tif sp.Start < 0 ||", "TestREC52d_")
m("REC-52 v5 a span may start before the command", L,
  " || sp.Start < 0 || sp.Start > sp.End ||", " || sp.Start > sp.End ||", "TestREC52d_")
m("REC-52 v5 a span may end before it starts", L,
  " || sp.Start > sp.End || sp.End > len(*cmd) {", " || sp.End > len(*cmd) {", "TestREC52d_")
m("REC-52 v5 a span may run past the command", L,
  " || sp.Start > sp.End || sp.End > len(*cmd) {", " || sp.Start > sp.End {", "TestREC52d_")
m("REC-52 v5 a withheld command keeps its cut and spans", L,
  "\t*key = w.oversize()\n\treturn nil, nil, nil\n", "\t*key = w.oversize()\n\treturn nil, cut, spans\n",
  "TestREC52c_|TestREC52e_")
for field, key in [("CWD", "CWD"), ("Project", "Project"), ("FilePath", "FilePath")]:
    m("REC-52 v5 no cap on the declaration's " + field, L,
      "\td.%s = w.ptr(d.%s, MaxPathBytes, &d.%sWithheld)\n" % (field, field, key), "", "TestREC52c_")
m("REC-52 v5 no cap on written_paths' entries", L,
  "\tif len(list) > MaxWrittenPaths {\n", "\tif false {\n", "TestREC52c_")
m("REC-52 v5 no cap on written_paths' bytes", L,
  "\treturn total <= MaxWrittenPathsBytes\n", "\treturn true\n", "TestREC52c_")
m("REC-52 v5 a withheld written_paths reads complete", L,
  "\t\tincomplete := false\n\t\td.WrittenPaths, d.WrittenPathsComplete = nil, &incomplete\n",
  "\t\tincomplete := true\n\t\td.WrittenPaths, d.WrittenPathsComplete = nil, &incomplete\n", "TestREC52c_")
m("REC-52 v5 no cap on a target value", L,
  "\t\tif v != nil && len(*v) > MaxTargetValueBytes {\n", "\t\tif v != nil && len(*v) > MaxTargetValueBytes && false {\n", "TestREC52c_")
m("REC-52 v5 no cap on the declaration's harness tool name", L,
  "\td.HarnessToolName = w.ptr(d.HarnessToolName, MaxHarnessToolNameBytes, nil)\n", "", "TestREC52c_")
m("REC-52 v5 no cap on the execution's harness tool name", L,
  "\tx.HarnessToolName = w.ptr(x.HarnessToolName, MaxHarnessToolNameBytes, nil)\n", "", "TestREC52e_")
m("REC-52 v5 a withheld field's key stays null", L,
  "\t\tif key != nil {\n\t\t\t*key = w.oversize()\n", "\t\tif key != nil {\n\t\t\tw.withheld = true\n",
  "TestREC52c_|TestREC52e_")
m("REC-52 v5 no cap on the executed command", L,
  "\tx.ExecutedCommand, x.ExecutedCommandCut, x.ExecutedMasked = w.command(\n", "\t_, _, _ = w.command(\n",
  "TestREC52e_")
m("REC-52 v5 no cap on the git state's root", L,
  "\tif x.HeadAfter != nil {\n", "\tif false {\n", "TestREC52e_")
