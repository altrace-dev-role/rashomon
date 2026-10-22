# Rubric naming-11: install.Events was the one exported mutable package
# variable, and it decides what watch writes and detach removes. It is now a
# function that builds a fresh slice on every call; handing every caller one
# shared slice again is the regression.
m("naming-11 Events hands every caller one shared slice", "internal/install/install.go",
  "func Events() []string {\n\treturn []string{\n",
  "var sharedEvents = events()\n\nfunc Events() []string { return sharedEvents }\n\nfunc events() []string {\n\treturn []string{\n",
  "TestRubricNAMING11_")
