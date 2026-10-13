# test-4: a call whose declaration lost the lock gets a spilled terminal
# naming it, and lock_timeout coverage.
m("test-4 the lock-timeout terminal is not written", "internal/hook/handle.go",
  "\t\t\terr = h.st.SpillTerminal(term)\n", "\t\t\terr = nil\n", "TestRubricTEST4_")
m("test-4 a declaration lost to the lock is closed only when it opened", "internal/hook/handle.go",
  "\tif h.opened || (h.toolUseID != \"\" && reason == store.ReasonLockTimeout) {\n",
  "\tif h.opened || false {\n", "TestRubricTEST4_")
m("test-4 the lock-timeout terminal goes to the ordered stream", "internal/hook/handle.go",
  "\t\tif reason == store.ReasonLockTimeout {\n\t\t\terr = h.st.SpillTerminal(term)\n",
  "\t\tif false {\n\t\t\terr = h.st.SpillTerminal(term)\n", "TestRubricTEST4_")
