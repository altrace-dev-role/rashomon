# errors-11: RecordFailure returns a write it could not make, and cmdRecap's
# one commented Guard is where it is dropped (H-93).
m("errors-11 RecordFailure drops its own write error", "internal/recap/state.go",
  "\treturn writeState(path, s, \"\")\n",
  "\tif err := writeState(path, s, \"\"); err != nil {\n\t\treturn nil\n\t}\n\treturn nil\n",
  "TestRubricERRORS11_")
