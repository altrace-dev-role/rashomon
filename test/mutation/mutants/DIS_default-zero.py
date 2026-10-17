# DIS: a caller that drops Default()'s error cannot record consent against a
# populated scope version on an inventory that has nothing else in it. The
# stub kept to this by returning the zero Inventory with ErrNotImplemented;
# now that Default is filled, the break is a scope version without the files.
m("DIS Default carries a scope version on an otherwise-empty inventory", "internal/disclose/disclose.go",
  "\t\tFiles:        defaultFiles(),\n",
  "", "TestDefaultNeverPairsAScopeVersionWithAnEmptyInventory")
