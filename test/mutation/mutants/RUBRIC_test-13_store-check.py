# test-13: every acceptance test's store is checked against its invariants
# when the test ends (rubric fix list entry 50). The breaks are in the check
# itself, which its own tests pin.
m("test-13  newEnv does not register the store check", "test/acceptance/harness_test.go",
  "\tt.Cleanup(e.checkStore)\n", "", "TestRubricTEST13_")
m("test-13  a repeated seq passes the check", "test/acceptance/storecheck_test.go",
  "\t\tcase int64(seq) <= last:\n", "\t\tcase int64(seq) < last:\n", "TestRubricTEST13_")
m("test-13  a line that is not JSON passes the check", "test/acceptance/storecheck_test.go",
  "\t\t\tout = append(out, fmt.Sprintf(\"%s is not one JSON object: %q\", where, sc.Bytes()))\n",
  "", "TestRubricTEST13_")
m("test-13  keys are not held to the allowlist", "test/acceptance/storecheck_test.go",
  "\t\tif !allow[k] {\n", "\t\tif false && !allow[k] {\n", "TestRubricTEST13_")
m("test-13  spilled records are held to the seq rule", "test/acceptance/storecheck_test.go",
  "\t\t\"spill.ndjson\":    {types: call, blankOK: true},\n", "\t\t\"spill.ndjson\":    {types: call, blankOK: true, ordered: true},\n", "TestRubricTEST13_")
m("test-13  a schema version this build does not read passes", "test/acceptance/storecheck_test.go",
  "!ok || float64(int(v)) != v || !readable(typ, int(v)) {", "!ok && v == 0 {", "TestRubricTEST13_")
m("test-13  a fractional schema version passes", "test/acceptance/storecheck_test.go",
  "!ok || float64(int(v)) != v || !readable(", "!ok || !readable(", "TestRubricTEST13_")
m("test-13  coverage and gaps are not held to their own versions", "test/acceptance/storecheck_test.go",
  "\treturn store.AcceptsKind(kind, version)\n",
  "\treturn version >= 1 && version <= store.CallSchemaVersion\n", "TestRubricTEST13_")
m("test-13  the end-of-test check reads no store", "test/acceptance/storecheck_test.go",
  "\tunexpected, missing := damageProblems(storeProblems(e.home), e.damaged)\n",
  "\tunexpected, missing := damageProblems(nil, e.damaged)\n", "TestRubricTEST13_")
m("test-13  declared damage stands the whole check down", "test/acceptance/storecheck_test.go",
  "\t\tif !named {\n", "\t\tif !named && len(declared) == 0 {\n", "TestRubricTEST13_")
m("test-13  declared damage that did not happen passes", "test/acceptance/storecheck_test.go",
  "\t\tif !matched[i] {\n", "\t\tif false && !matched[i] {\n", "TestRubricTEST13_")
m("test-13  the store check reads a line over the store's bound as one that is not JSON", "test/acceptance/storecheck_test.go",
  "\t\tif sc.TooLong() {\n\t\t\tout = append(out, fmt.Sprintf(\"%s: a line over store.MaxLine\", where))\n\t\t\tcontinue\n\t\t}\n",
  "", "TestRubricTEST13_")
m("test-13  a runner_kind null beside a set runner_digest passes", "test/acceptance/storecheck_test.go",
  "\t\t\tout = append(out, fmt.Sprintf(\"%s: runner_kind %v beside runner_digest %v, one null and one not\", where, kind, digest))\n",
  "", "TestRubricTEST13_")
m("test-13  the runner pair is held to differ, not to agree", "test/acceptance/storecheck_test.go",
  "\t\tif (kind == nil) != (digest == nil) {\n", "\t\tif (kind == nil) == (digest == nil) {\n", "TestRubricTEST13_")
m("test-13  a runner_kind outside shape.RunnerKinds() passes", "test/acceptance/storecheck_test.go",
  "\t\t\tout = append(out, fmt.Sprintf(\"%s: runner_kind %q is outside shape.RunnerKinds()\", where, kind))\n",
  "", "TestRubricTEST13_")
