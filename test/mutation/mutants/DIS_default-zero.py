# DIS: a caller that drops Default()'s error cannot record consent against a
# populated scope version on an inventory that has nothing else in it. The
# stub kept to this by returning the zero Inventory with ErrNotImplemented;
# now that Default is filled, the break is a scope version without the files,
# or without the screen paragraphs that joined them.
# The Files block is disclose.go's only use of slices, so the break drops the
# import beside it; without it the mutant does not build (BUILD BROKEN, main
# dispatch run 41521646555).
IMPORTS["DIS Default carries a scope version on an otherwise-empty inventory"] = ("internal/disclose/disclose.go",
  '\t"slices"\n', "")
m("DIS Default carries a scope version on an otherwise-empty inventory", "internal/disclose/disclose.go",
  "\t\tFiles: slices.Concat(\n\t\t\tdefaultFiles(),\n\t\t\tfiredFiles(),\n\t\t\tblastReadFiles(),\n\t\t\tlearnFiles(),\n\t\t\tspecFiles(),\n\t\t\tevidenceFiles(),\n\t\t\tconfigFiles(),\n\t\t\tcontinuityFiles(),\n\t\t),\n",
  "", "TestDefaultNeverPairsAScopeVersionWithAnEmptyInventory")
m("DIS Default carries a scope version with no screen paragraphs", "internal/disclose/disclose.go",
  "\t\tParagraphs: defaultParagraphs(),\n",
  "", "TestDefaultNeverPairsAScopeVersionWithAnEmptyInventory")
