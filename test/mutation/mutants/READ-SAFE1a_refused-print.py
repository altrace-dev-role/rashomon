# READ-SAFE1a (PR 325): a prompt's notice that does not print gives the
# turn's markers back, so the next prompt says it. Written, not run: for the
# end sweep.
preflight = "cmd/rashomon/preflight.go"

# TestRS1a_C5_ARefusedPrintGivesTheMarkerBack. Break: drop the giveBack
# after a refused marshal, and the refused print keeps the marker, so the
# next prompt says nothing.
m("READ-SAFE1a a refused print keeps the turn's markers", preflight,
  "\tif err != nil || b == nil {\n\t\ttn.giveBack(taken)\n\t\treturn nil\n\t}\n",
  "\tif err != nil || b == nil {\n\t\treturn nil\n\t}\n",
  "^TestRS1a_C5_ARefusedPrintGivesTheMarkerBack$")
