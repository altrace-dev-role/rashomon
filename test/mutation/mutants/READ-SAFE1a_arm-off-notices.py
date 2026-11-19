# READ-SAFE1a, the turn's notices under the off arm (c5 and D7, card
# MEASURE1). The off arm withholds every hook print, and promptTurnNotes
# (cmd/rashomon/preflight.go) withholds the index cap notice and the manifest
# notice with them and takes neither printed marker, so a session measured
# with the arm off still owes both. printTurn reads no arm of its own, so the
# row's break both prints the lines and takes the markers. Written, not run:
# for the end sweep.
m("READ-SAFE1a the off arm says the turn's notices", "cmd/rashomon/preflight.go",
  "\tif t.due() && armOff(root) {\n\t\treturn turnNotes{}\n\t}\n", "",
  "TestRS1a_C5_D7_ArmOffSaysNoNoticeAndTakesNoMarker")
