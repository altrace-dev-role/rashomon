# Part 2, sensitive-file labels. Each filter names the acceptance item AND the
# shape unit tests, deliberately: until schema 3 carries file_label the record
# does not show the label, so the acceptance halves that read it skip and the
# unit tests are what observe the break. This is the same widening the H-7
# mutation above needed, for the same reason -- a mutation judged only by a
# test that no longer executes the mutated line goes NOT DETECTED and says
# nothing.
m("H-44 the basename is stored as the label", "internal/shape/label.go",
  "\t}\n\treturn LabelNone\n}", "\t}\n\treturn base\n}", "TestH44_|TestLabel")

m("H-44 the label is computed and thrown away", "internal/hook/handle.go",
  "\tif label := fileLabel(p.ToolName, p.ToolInput); label != \"\" {\n\t\tdecl.FileLabel = &label\n\t}",
  "\t_ = fileLabel(p.ToolName, p.ToolInput)", "TestH44_|TestH45_|TestH46_")
m("H-44 a stored label reaches the report unclamped", "internal/report/report.go",
  "\t\t\tbyLabel[knownLabel(*d.FileLabel)]++",
  "\t\t\tbyLabel[*d.FileLabel]++", "TestByLabel_")
