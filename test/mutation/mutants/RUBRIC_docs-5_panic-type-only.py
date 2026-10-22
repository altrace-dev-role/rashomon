# Rubric docs-5: a recovered panic's error names the value's type and nothing
# else, because the value is often a string the program held (a command
# line, a prompt) and guarded() prints the error. Formatting the value into
# it is the regression; nothing caught it before internal/safe had tests.
m("docs-5 the panic value is formatted into the error", "internal/safe/safe.go",
  "\treturn &PanicError{TypeName: fmt.Sprintf(\"%T\", v)}",
  "\treturn &PanicError{TypeName: fmt.Sprintf(\"%T: %v\", v, v)}",
  "TestRubricDOCS5_")
