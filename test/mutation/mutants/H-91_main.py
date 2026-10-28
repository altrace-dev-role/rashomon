# Which report command the recap line points at is decided in cmdRecap, and
# nothing judged it until H-91's pointer test: H-91's first test checks only
# "--session <id>", which both commands contain. Three breaks, one per way the
# decision can go wrong -- ignore the marker (the shipped defect: a settings
# install made from the plugin's binary was sent to /rashomon:report), or
# replace the decision with either constant.
m("H-91 the recap pointer ignores the --install marker", "cmd/rashomon/recap_turn.go",
  "\tif installArg(args) != \"\" {\n\t\treturn false\n\t}\n\tpresent, err := install.PluginFires(install.EventStop)",
  "\tif false {\n\t\treturn false\n\t}\n\tpresent, err := install.PluginFires(install.EventStop)",
  "TestH91_TheReportPointer")
m("H-91 the recap pointer is always the CLI command", "cmd/rashomon/recap_turn.go",
  "\treturn err == nil && present\n", "\treturn err == nil && present && false\n", "TestH91_TheReportPointer")
m("H-91 the recap pointer is always the plugin command", "cmd/rashomon/recap_turn.go",
  "func recapFromPlugin(args []string) bool {\n",
  "func recapFromPlugin(args []string) bool {\n\tif true {\n\t\treturn true\n\t}\n", "TestH91_TheReportPointer")
m("a legacy present record is treated with suspicion and renders unverified", "internal/report/report.go",
  "\t\t\tf.Reasons = append(f.Reasons, *c.Reason)\n\t\t}\n\t}\n\treturn f\n}",
  "\t\t\tf.Reasons = append(f.Reasons, *c.Reason)\n\t\t}\n\t\tif c.HookEntry == store.EntryPresent {\n\t\t\tf.Reasons = append(f.Reasons, ReasonRunNotClosed)\n\t\t}\n\t}\n\treturn f\n}",
  "TestLegacyPresentRendersVerified")
