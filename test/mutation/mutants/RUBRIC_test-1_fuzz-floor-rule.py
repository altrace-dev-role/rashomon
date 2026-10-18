# test-1: the fuzz floor's tree count takes what go test takes as a fuzz
# target, by name and by signature, and nothing else.
m("test-1  a name like Fuzzy counts as a target", "test/mutation/fuzz_job_test.go",
  "\treturn !unicode.IsLower(r)\n", "\treturn !unicode.IsLower(r) || true\n", "TestRubricTest1_")
m("test-1  a two-parameter Fuzz function counts as a target", "test/mutation/fuzz_job_test.go",
  " || fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {", " || fn.Type.Params == nil {", "TestRubricTest1_")
m("test-1  a Fuzz function taking *testing.T counts as a target", "test/mutation/fuzz_job_test.go",
  "\t\tif !ok || sel.Sel.Name != \"F\" {", "\t\tif !ok {", "TestRubricTest1_")
m("test-1  a method counts as a target", "test/mutation/fuzz_job_test.go",
  "\t\tif !ok || fn.Recv != nil || !isFuzzName(", "\t\tif !ok || !isFuzzName(", "TestRubricTest1_")
