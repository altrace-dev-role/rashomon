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
  "!ok || float64(int(v)) != v || !readable(typ, int(v)) {", "!ok && v == 0 {", "TestRubricTest13_")
m("test-13  a fractional schema version passes", "test/acceptance/storecheck_test.go",
  "!ok || float64(int(v)) != v || !readable(", "!ok || !readable(", "TestRubricTest13_")
m("test-13  coverage and gaps are not held to their own versions", "test/acceptance/storecheck_test.go",
  "\treturn store.Accepts(version) && store.AcceptsKind(kind, version)\n",
  "\treturn store.Accepts(version)\n", "TestRubricTest13_")
m("test-13  call records pass at a version today's reader skips", "test/acceptance/storecheck_test.go",
  "\treturn store.Accepts(version) && store.AcceptsKind(kind, version)\n",
  "\treturn store.AcceptsKind(kind, version)\n", "TestRubricTest13_")
m("test-13  the end-of-test check reads no store", "test/acceptance/storecheck_test.go",
  "\tunexpected, missing := damageProblems(storeProblems(e.home), e.damaged)\n",
  "\tunexpected, missing := damageProblems(nil, e.damaged)\n", "TestRubricTest13_")
m("test-13  declared damage stands the whole check down", "test/acceptance/storecheck_test.go",
  "\t\tif !named {\n", "\t\tif !named && len(declared) == 0 {\n", "TestRubricTest13_")
m("test-13  declared damage that did not happen passes", "test/acceptance/storecheck_test.go",
  "\t\tif !matched[i] {\n", "\t\tif false && !matched[i] {\n", "TestRubricTest13_")
