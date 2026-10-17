m("H-103 the duplicate-declarations guard is dropped", "internal/report/report.go",
  "\tif HasDuplicateToolUseID(run.Declarations) {\n\t\tsess.Coverage.add(ReasonDuplicateDeclarations)\n\t}\n",
  "", "TestH103_")
m("H-103 the shared duplicate tool_use_id rule never fires", "internal/report/report.go",
  "\t\tif seen[d.ToolUseID] {\n\t\t\treturn true\n\t\t}",
  "\t\tif false && seen[d.ToolUseID] {\n\t\t\treturn true\n\t\t}",
  "TestH103_DuplicateDeclarationsAreNamedNotHidden|TestHasDuplicateToolUseID|TestBuild_DuplicateDeclarationsMakeTheTurnUnverified")
m("H-103 the turn digest drops the duplicate-declarations guard", "internal/digest/coverage.go",
  "\tif report.HasDuplicateToolUseID(w.declarations) {\n\t\ttc.add(report.ReasonDuplicateDeclarations)\n\t}\n",
  "", "TestBuild_DuplicateDeclarationsMakeTheTurnUnverified")
