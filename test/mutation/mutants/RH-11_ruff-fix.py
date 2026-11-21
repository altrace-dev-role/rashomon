# RH-11: ruff check rewrites the files it fixes under --fix and --fix-only,
# so a stage carrying either may write and names its file operands, not
# ruff's check word, as its written paths. Each row is killed by its row of
# TestRH11_RuffFixFlagsAreWrites; a table row also moves the write-table pin.

S = "internal/shape/verb.go"
W = "internal/shape/writetargets.go"
P = "TestRH11_"
m("RH-11 ruff's --fix does not arm the editor", S,
  "\t\"ruff\":         {\"format\", \"--fix\", \"--fix-only\"},\n",
  "\t\"ruff\":         {\"format\", \"--fix-only\"},\n", P)
m("RH-11 ruff's --fix-only does not arm the editor", S,
  "\t\"ruff\":         {\"format\", \"--fix\", \"--fix-only\"},\n",
  "\t\"ruff\":         {\"format\", \"--fix\"},\n", P)
m("RH-11 ruff's check word is named a written path", W,
  "\tif o.name == \"ruff\" && text == \"check\" {\n",
  "\tif false && o.name == \"ruff\" && text == \"check\" {\n", P)
