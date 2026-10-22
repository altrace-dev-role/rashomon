# test-11: the readers production calls -- ReadRunConsistent and ReadGaps --
# count a read the store's refusal predicate calls refused as one skipped
# line. On unix that predicate is constant false, so the tests set the
# store's own predicate to EISDIR and read a directory: a call site that
# passes a predicate of its own instead fails the read, as on Windows.
m("test-11 the consistent read never counts a refusal", "internal/store/read.go",
  "left, s.refusal(), func", "left, neverRefused, func",
  "TestRubricTEST11_TheConsistentReadCountsARefusalAtItsCallSite")
m("test-11 the gaps read never counts a refusal", "internal/store/read.go",
  "\treturn readGapsAt(filepath.Join(s.root, FileGaps), s.refusal())\n",
  "\treturn readGapsAt(filepath.Join(s.root, FileGaps), neverRefused)\n",
  "TestRubricTEST11_TheGapsReadCountsARefusalAtItsCallSite")
m("test-11 the store's refusal predicate is ignored", "internal/store/read.go",
  "\tif s.readRefusal != nil {\n", "\tif false {\n",
  "TestRubricTEST11_TheConsistentReadCountsARefusalAtItsCallSite|TestRubricTEST11_TheGapsReadCountsARefusalAtItsCallSite")
