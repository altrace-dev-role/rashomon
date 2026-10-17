m("H-46 Bash is labelled from its first path-like token", "internal/shape/label.go",
  "\tfield, ok := pathFields[toolName]\n\tif !ok {\n\t\treturn \"\"\n\t}",
  "\tfield, ok := pathFields[toolName]\n\tif !ok {\n\t\tif toolName == \"Bash\" {\n\t\t\tcmd, _ := stringField(toolInput, \"command\")\n\t\t\tfor _, tok := range strings.Fields(cmd) {\n\t\t\t\tif strings.Contains(tok, \"/\") {\n\t\t\t\t\treturn labelForBase(basename(tok))\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t\treturn \"\"\n\t}",
  "TestH46_|TestLabel")

m("H-46 labelling recovers after the append instead of before it", "internal/hook/handle.go",
  "\tdefer func() {\n\t\tif v := recover(); v != nil {\n\t\t\tlabel = shape.LabelUnknown\n\t\t}\n\t}()\n\tfault.Inject(fault.PointLabel)",
  "\tfault.Inject(fault.PointLabel)", "TestH46_NoFault")

m("a paused session probe counts as the session's start or end", "internal/report/report.go",
  "\t\tcase paused:\n", "\t\tcase paused && false:\n", "TestPausedSessionProbeIsNotReportedAsRecorded")
