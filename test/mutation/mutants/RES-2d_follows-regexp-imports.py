# RES-2d: the package-scope regexp gate follows the file's own name for
# the "regexp" import -- aliased, or reached through a same-package
# helper a var initialiser calls -- rather than only the literal word
# "regexp". A compile reachable only those ways was invisible before.
IMPORTS["RES-2d an aliased regexp import still gets caught"] = ("internal/verify/state.go",
  "package verify\n", "package verify\n\nimport re \"regexp\"\n")
m("RES-2d an aliased regexp import still gets caught", "internal/verify/state.go",
  "// States is the closed list in the spec's declaration order (2.2.4).\n",
  "var statesForm = re.MustCompile(`^[a-z_]+$`)\n\n// States is the closed list in the spec's declaration order (2.2.4).\n", "TestRES2c_")

IMPORTS["RES-2d a helper a var initialiser calls still gets caught"] = ("internal/verify/state.go",
  "package verify\n", "package verify\n\nimport \"regexp\"\n")
m("RES-2d a helper a var initialiser calls still gets caught", "internal/verify/state.go",
  "// NoGateKnown is the verdict of a claim no gate can speak to.\n",
  "func compileNoGateForm() *regexp.Regexp { return regexp.MustCompile(`^[a-z_]+$`) }\n\nvar noGateForm = compileNoGateForm()\n\n// NoGateKnown is the verdict of a claim no gate can speak to.\n", "TestRES2c_")

IMPORTS["RES-2d a dot import is refused outright"] = ("internal/verify/state.go",
  "package verify\n", "package verify\n\nimport . \"regexp\"\n")
m("RES-2d a dot import is refused outright", "internal/verify/state.go",
  "// Worse reports whether a ranks before b in Order.\n",
  "var dotForm = MustCompile(`^[a-z_]+$`)\n\n// Worse reports whether a ranks before b in Order.\n", "TestRES2c_")
