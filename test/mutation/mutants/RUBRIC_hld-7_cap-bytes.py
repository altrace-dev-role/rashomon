# hld-7, docs-4: RASHOMON_STORE_CAP_BYTES is a plain decimal byte count, zero
# is eviction off, and any other value is invalid, applies the default, and
# is reported by status and watch.
m("hld-7 a signed value is taken as a cap", "internal/store/capbytes.go",
  "\tn, err := strconv.ParseUint(raw, 10, 63)\n", "\tn, err := strconv.ParseInt(raw, 10, 64)\n",
  "TestRubricHLD7_")
m("hld-7 an invalid cap falls back without saying so", "internal/store/capbytes.go",
  "\t\tc.Invalid = true\n", "", "TestRubricHLD7_")
m("hld-7 status does not report an invalid cap", "cmd/rashomon/main.go",
  "\tcapProblem(stdout, \"  cap: \")\n", "", "TestRubricHLD7_StatusAndWatchSayACapIsNotAByteCount")
m("hld-7 watch does not report an invalid cap", "cmd/rashomon/main.go",
  "\tcapProblem(stdout, \"rashomon: \")\n", "", "TestRubricHLD7_StatusAndWatchSayACapIsNotAByteCount")
