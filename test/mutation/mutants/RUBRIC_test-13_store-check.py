# test-13: every acceptance test's store is checked against its invariants
# when the test ends (rubric fix list entry 50). The breaks are in the check
# itself, which its own tests pin.
m("test-13  newEnv does not register the store check", "test/acceptance/harness_test.go",
  "\tt.Cleanup(e.checkStore)\n", "", "TestRubricTest13_")
m("test-13  a repeated seq passes the check", "test/acceptance/storecheck_test.go",
  "\t\tcase int64(seq) <= last:\n", "\t\tcase int64(seq) < last:\n", "TestRubricTest13_")
m("test-13  a line that is not JSON passes the check", "test/acceptance/storecheck_test.go",
  "\t\t\tout = append(out, fmt.Sprintf(\"%s is not one JSON object: %q\", where, sc.Text()))\n",
  "", "TestRubricTest13_")
m("test-13  keys are not held to the allowlist", "test/acceptance/storecheck_test.go",
  "\t\tif !allow[k] {\n", "\t\tif false && !allow[k] {\n", "TestRubricTest13_")
m("test-13  spilled records are held to the seq rule", "test/acceptance/storecheck_test.go",
  "\t\t\"spill.ndjson\":    {types: call},\n", "\t\t\"spill.ndjson\":    {types: call, ordered: true},\n", "TestRubricTest13_")
m("test-13  a schema version this build does not read passes", "test/acceptance/storecheck_test.go",
  "!ok || v < 1 || v > 4 {", "!ok || v < 1 {", "TestRubricTest13_")
