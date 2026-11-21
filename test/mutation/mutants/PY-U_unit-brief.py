# PY-U: the post's unit brief is the brief of the unit the reader that
# claims the edited file places it in (LandedGoEdit.Unit), for a .py file
# as for a .go file. The break names the unit by the file's directory under
# one language's id: the .go fixture's unit is the directory of its file,
# so the tests over it cannot tell the two apart; a py unit's brief is then
# absent under that name, so no line is delivered and a miss names another
# unit.
J = "cmd/rashomon/graph_unitbrief.go"
m("PY-U the post names the edit's unit by its directory under one language's id", J,
  "\tunit, ok := e.Unit()\n",
  "\tunit, ok := graph.UnitID(\"go\", path.Dir(e.Rel)), true\n",
  "TestPost_UnitBriefOfAPyUnitDeliversOnceAtTheFirstEdit|TestUnitBrief_PyUnitMissIsCounted|TestUnitBrief_PyEditInNoIndexedUnitIsNeitherSaidNorCounted")
