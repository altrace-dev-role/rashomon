# DIS: Default() returns the zero Inventory alongside ErrNotImplemented, so a
# caller that drops the error cannot record consent against a populated scope
# version on an inventory that has nothing else in it.
m("DIS Default carries a scope version on the stub inventory", "internal/disclose/disclose.go",
  "\treturn Inventory{}, ErrNotImplemented\n",
  "\treturn Inventory{ScopeVersion: ScopeVersion}, ErrNotImplemented\n", "TestDefaultIsTheZeroInventoryUntilFilled")
