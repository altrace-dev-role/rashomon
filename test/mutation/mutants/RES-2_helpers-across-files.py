# RES-2: the package-scope regexp gate follows a var initialiser into a
# helper declared in any file of the package, and reads the helper's
# compile through that file's own name for "regexp". A compile split across
# two files, a var in one and the helper in another, was invisible before.
IMPORTS["RES-2 a helper in another file compiles a regexp at package scope"] = ("internal/verify/state.go",
  "package verify\n",
  "package verify\n\nimport \"regexp\"\n\nfunc compileCrossForm() *regexp.Regexp { return regexp.MustCompile(`^[a-z_]+$`) }\n")
m("RES-2 a helper in another file compiles a regexp at package scope", "internal/verify/snapshot.go",
  "// PointKinds is the closed list in the archive's order.\n",
  "var crossForm = compileCrossForm()\n\n// PointKinds is the closed list in the archive's order.\n", "TestRES2c_")
m("RES-2 a helper's compile is read through the caller's file", "internal/verify/res2_test.go",
  "\t\t\t\t\tvisit(callee.decl.Body, callee.regexpName, next)\n",
  "\t\t\t\t\tvisit(callee.decl.Body, regexpName, next)\n", "TestRES2e_")
