# docs-6, errors-11: a call whose terminal could not be written is covered by
# internal_error, never by a verified record.
m("docs-6 a failed terminal leaves the call verified", "internal/hook/handle.go",
  "\t\tif err != nil && reason == \"\" {\n", "\t\tif err != nil && false {\n", "TestRubricDOCS6_")
m("docs-6 a failed terminal is reported as a lock timeout", "internal/hook/handle.go",
  "\t\t\treason = store.ReasonInternalError\n", "\t\t\treason = store.ReasonLockTimeout\n", "TestRubricDOCS6_")
