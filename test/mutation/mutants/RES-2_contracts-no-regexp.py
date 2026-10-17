# RES-2: no package compiles a regexp at package scope. The contract packages
# are not linked into the binary yet, so their own static test holds the rule
# until they are; a package-scope compile in one of them is the whole regression.
IMPORTS["RES-2 a contract package compiles a regexp at package scope"] = ("internal/verify/state.go",
  "package verify\n", "package verify\n\nimport \"regexp\"\n")
m("RES-2 a contract package compiles a regexp at package scope", "internal/verify/state.go",
  "// State is a gate's state at a point (2.2.4).\n",
  "var stateForm = regexp.MustCompile(`^[a-z_]+$`)\n\n// State is a gate's state at a point (2.2.4).\n", "TestRES2c_")
