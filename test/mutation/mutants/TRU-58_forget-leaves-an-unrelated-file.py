# TRU-58: a baseline file without the basis marker is deleted whole when it
# holds the host being forgotten -- not whenever a forget for ANY host meets
# it. Without the guard, a forget for a host the file never held would still
# remove it, taking a different project's baseline with it.
m("TRU-58 forget deletes a marker-less baseline that does not hold the host",
  "internal/baseline/baseline.go",
  "\tif !held {\n\t\treturn 0, nil\n\t}\n\treturn removeWhole(path)\n}",
  "\treturn removeWhole(path)\n}",
  "TestTRU58_ForgetRemovesAPreDeclaredBaselineWhole")
