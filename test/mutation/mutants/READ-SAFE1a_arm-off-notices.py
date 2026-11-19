# READ-SAFE1a, the turn's notice under the off arm (c5, card MEASURE1). The
# off arm withholds every hook print, and promptTurnNotes
# (cmd/rashomon/preflight.go) withholds the index cap notice with them and
# takes no printed marker, so a session measured with the arm off still owes
# it. printTurn reads no arm of its own, so the row's break both prints the
# line and takes the marker. Written, not run: for the end sweep.
m("READ-SAFE1a the off arm says the turn's notices", "cmd/rashomon/preflight.go",
  "\tif t.due() && armOff(root) {\n\t\treturn turnNotes{}\n\t}\n", "",
  "TestRS1a_C5_ArmOffSaysNoNoticeAndTakesNoMarker")
