# FOLD: Cursor carries a generation alongside offset, seq and size_seen, so
# a store-maintained rewrite counter has somewhere to go. Dropping the field
# loses the one signal that survives store's in-place rewrite of a file at
# the same size.
m("FOLD Cursor drops its generation field", "internal/fold/fold.go",
  "\tGeneration int64 `json:\"generation\"`\n}\n", "}\n", "TestFoldFileCarriesTheSpecsKeys")
