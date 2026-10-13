# docs-3: a zero store cap turns eviction off.
m("docs-3 a zero cap evicts everything past the grace period", "internal/store/evict.go",
  "\tif capBytes <= 0 {\n", "\tif capBytes < 0 {\n", "TestRubricDOCS3_ZeroCapDisablesEviction")
