# SCL-1: the bare spec verbs find the spec in a plain terminal, and the trust
# print's undo line names the path (card SPEC-CLI1). One break per promise:
# the cwd fallback dropped (a plain terminal with no .git and no
# CLAUDE_PROJECT_DIR finds nothing), the fallback made to replace the walk
# (a subdirectory of a project no longer reaches the project's spec), and the
# path dropped from the undo line.
C = "cmd/rashomon/spec.go"
m("SCL-1 a plain terminal finds no spec", C, "\t\tlocs = hook.SpecLocations(cwd, cwd)\n", "",
  "TestSCL1_BareTrustFindsTheCwdSpecWithNoGit")
m("SCL-1 the cwd replaces the walk", C, "\tif len(locs) == 0 {\n", "\tif true {\n",
  "TestSCL1c_TheWalkIsUnchangedWhereItWasFound")
m("SCL-1 the undo line drops the path", C, "untrust %s\\n\", path, typed, shellWord(path))",
  "untrust\\n\", path, typed)", "TestSCL1b_TheUndoLineNamesThePath")
