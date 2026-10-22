# Rubric errors-6: a pause marker that exists and cannot be read is a pause.
# checkPaused used to read any error from store.Paused as "not paused", so a
# chmod-000 marker let the hook record while status said recording: unknown.
m("errors-6 an unreadable pause marker reads as not paused", "cmd/rashomon/main.go",
  "\tcase pausedErr == nil && !paused:\n\t\treturn false\n",
  "\tcase pausedErr != nil || !paused:\n\t\treturn false\n",
  "TestRubricERRORS6_")
