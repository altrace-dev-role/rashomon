# Rubric errors-8: a recovered panic is recorded as recovered_panic, apart
# from internal_error's ordinary failures, and a panic recovered inside the
# label derivation is named in the call's coverage record instead of leaving
# only an unknown label that an empty path field also produces.
m("errors-8 a recovered panic is recorded as an internal error", "internal/hook/payload.go",
  "\tcase errors.As(err, new(*safe.PanicError)):\n",
  "\tcase false && errors.As(err, new(*safe.PanicError)):\n",
  "TestRubricERRORS8_")
m("errors-8 a recovered label panic is not named in coverage", "internal/hook/handle.go",
  "\tif cov.Reason == nil && h.labelRecovered {\n",
  "\tif false && cov.Reason == nil && h.labelRecovered {\n",
  "TestRubricERRORS8_")
m("errors-8 the label's recover is never reported to the handler", "internal/hook/handle.go",
  "\th.labelRecovered = recovered\n",
  "\th.labelRecovered = recovered && false\n",
  "TestRubricERRORS8_")
m("errors-8 a recovered label panic overrides a reason that cost more", "internal/hook/handle.go",
  "\tif cov.Reason == nil && h.labelRecovered {\n",
  "\tif true && h.labelRecovered {\n",
  "TestRubricERRORS8_")
