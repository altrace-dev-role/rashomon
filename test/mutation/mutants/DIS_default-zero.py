# DIS: a caller that drops Default()'s error cannot record consent against a
# populated scope version on an inventory that has nothing else in it. The
# stub kept to this by returning the zero Inventory with ErrNotImplemented;
# now that Default is filled, the break is a scope version without the files,
# or without the screen paragraphs that joined them.
m("DIS Default carries a scope version on an otherwise-empty inventory", "internal/disclose/disclose.go",
  "\t\tFiles:        slices.Concat(defaultFiles(), learnFiles(), specFiles(), evidenceFiles()),\n",
  "", "TestDefaultNeverPairsAScopeVersionWithAnEmptyInventory")
m("DIS Default carries a scope version with no screen paragraphs", "internal/disclose/disclose.go",
  "\t\tParagraphs:   defaultParagraphs(),\n",
  "", "TestDefaultNeverPairsAScopeVersionWithAnEmptyInventory")
