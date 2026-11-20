# MS-1 (PR 325): under arm off the post says no line of its own and takes
# no marker, on both exits of the steer step, so the next edit with the arm
# unset says the line. Written, not run: for the end sweep.
kindline = "internal/hook/kindline.go"

# TestMS1_ArmOffSaysNoPostLineAndTakesNoMarker. Break: read no arm before
# the line is taken (either exit says the line under arm off, and its
# marker is taken).
m("MS-1 the post line is taken under the off arm", kindline,
  "\tif arm == steer.ArmOff {\n\t\treturn \"\", \"\"\n\t}\n", "",
  "^TestMS1_ArmOffSaysNoPostLineAndTakesNoMarker$")
