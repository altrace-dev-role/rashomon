# Rubric conc-5: safe.Go is the only sanctioned way to start a goroutine, and
# a test now enforces it over every non-test source file. A bare go
# statement in the hook package is the regression it exists to catch.
m("conc-5 the hook package starts a bare goroutine", "internal/hook/signals.go",
  "\tsignal.Notify(s.ch, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)\n\treturn s\n",
  "\tsignal.Notify(s.ch, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)\n\tgo func() {}()\n\treturn s\n",
  "TestRubricCONC5_")
m("conc-5 an import that only looks like time or context is flagged anyway", "internal/safe/safe_test.go",
  "\t\tif err != nil || (path != \"time\" && path != \"context\") {\n",
  "\t\tif err != nil {\n",
  "TestUnsafeGoroutineStarts_")
m("conc-5 an aliased time or context import is read under its default name", "internal/safe/safe_test.go",
  "\t\tif imp.Name != nil {\n\t\t\tlocal = imp.Name.Name\n\t\t}\n",
  "",
  "TestUnsafeGoroutineStarts_")
