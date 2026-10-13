# Which report command the recap line points at is decided in cmdRecap, and
# nothing judged it until H-91's pointer test: H-91's first test checks only
# "--session <id>", which both commands contain. Three breaks, one per way the
# decision can go wrong -- ignore the marker (the shipped defect: a settings
# install made from the plugin's binary was sent to /rashomon:report), or
# replace the decision with either constant.
m("H-91 the recap pointer ignores the --install marker", "cmd/rashomon/main.go",
  "\t\tif installArg(args) == \"\" {\n\t\t\tif present, perr := install.PluginPresent(install.EventStop); perr == nil {",
  "\t\tif true {\n\t\t\tif present, perr := install.PluginPresent(install.EventStop); perr == nil {",
  "TestH91_TheReportPointer")
m("H-91 the recap pointer is always the CLI command", "cmd/rashomon/main.go",
  "\t\t\t\tfromPlugin = present\n", "\t\t\t\t_ = present\n", "TestH91_TheReportPointer")
m("H-91 the recap pointer is always the plugin command", "cmd/rashomon/main.go",
  "\t\tfromPlugin := false\n\t\tif installArg(args) == \"\" {",
  "\t\tfromPlugin := true\n\t\tif false && installArg(args) == \"\" {", "TestH91_TheReportPointer")
m("a legacy present record is treated with suspicion and renders unverified", "internal/report/report.go",
  "\t\t\tf.Reasons = append(f.Reasons, *c.Reason)\n\t\t}\n\t}\n\treturn f\n}",
  "\t\t\tf.Reasons = append(f.Reasons, *c.Reason)\n\t\t}\n\t\tif c.HookEntry == store.EntryPresent {\n\t\t\tf.Reasons = append(f.Reasons, ReasonRunNotClosed)\n\t\t}\n\t}\n\treturn f\n}",
  "TestLegacyPresentRendersVerified")
