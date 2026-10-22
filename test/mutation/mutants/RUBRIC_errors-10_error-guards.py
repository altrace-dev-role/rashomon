# errors-10: settings.Error renders with no cause and as a nil pointer, and a
# parsed document keeps its own copy of the bytes it was given.
m("errors-10  ParseNamed keeps the caller's slice", "internal/settings/document.go",
  "\tdata = bytes.Clone(data)\n", "", "TestRubricERRORS10_")
m("errors-10  a nil *Error panics when rendered", "internal/settings/errors.go",
  "\tif e == nil {\n\t\treturn \"settings: <nil>\"\n\t}",
  "\tif false {\n\t\treturn \"settings: <nil>\"\n\t}", "TestRubricERRORS10_")
m("errors-10  an Error with no Err panics when rendered", "internal/settings/errors.go",
  "\tif e.Err != nil {\n\t\tproblem = e.Err.Error()\n\t}",
  "\tif true {\n\t\tproblem = e.Err.Error()\n\t}", "TestRubricERRORS10_")
m("errors-10  a nil *Error panics when unwrapped", "internal/settings/errors.go",
  "\tif e == nil {\n\t\treturn nil\n\t}\n\treturn e.Err",
  "\tif false {\n\t\treturn nil\n\t}\n\treturn e.Err", "TestRubricERRORS10_")
