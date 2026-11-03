# STOP-NAMES-GATE1: the Stop line names a gate a trusted spec declares by its
# id, in the slot's quote, not by the program that ran it. Every row is killed
# by TestSNG1_ADeclaredGateIsNamedByItsID.

m("SNG-1 the declared branch is dropped", "internal/recap/value.go",
  "ok && len(id) <= maxIDName && !(t.short", "ok && false && len(id) <= maxIDName && !(t.short",
  "TestSNG1_ADeclaredGateIsNamedByItsID")
m("SNG-1 the id is named bare, outside the slot's quote", "internal/recap/value.go",
  "\t\t\treturn p.Text()\n", "\t\t\treturn id\n",
  "TestSNG1_ADeclaredGateIsNamedByItsID")
m("SNG-1 the slot's screens are skipped", "internal/recap/value.go",
  "p.Validate() == nil {", "true {",
  "TestSNG1_ADeclaredGateIsNamedByItsID")
m("SNG-1 the id length is unbounded", "internal/recap/value.go",
  "ok && len(id) <= maxIDName && !(t.short", "ok && !(t.short",
  "TestSNG1_ADeclaredGateIsNamedByItsID")
m("SNG-1 the id bound is too wide for a line", "internal/recap/value.go",
  "const maxIDName = 24", "const maxIDName = 32",
  "TestSNG1_ADeclaredGateIsNamedByItsID")
