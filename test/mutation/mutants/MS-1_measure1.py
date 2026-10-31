# MS-1 (card MEASURE1, PR A1): the paired run's arm switch, arm off's
# decide-time fold, recap's silence under arm off, and the lab gate.
m("MS-1 contextOf keeps the live arm whatever env names", "internal/hook/steerstep.go",
  "\t\tArm:              armOf(env),\n",
  "\t\tArm:              steer.ArmLive,\n", "TestMS1_")
m("MS-1 SteerArm reads OFF as off", "internal/hook/steerstep.go",
  "\tcase \"off\":\n",
  "\tcase \"off\", \"OFF\":\n", "TestMS1c_")
m("MS-1 a forced arm still draws the holdout", "internal/hook/steerstep.go",
  "\tif env.HoldoutOff || env.Arm != \"\" {\n",
  "\tif env.HoldoutOff {\n", "TestMS1d_")
m("MS-1 arm_off back at the top of decide's switch", "internal/steer/decide.go",
  "\t\tswitch {\n\t\tcase rule == ruleNoChannel:\n",
  "\t\tswitch {\n\t\tcase arm == ArmOff:\n\t\t\tp.suppress(SuppressedByArmOff)\n\t\tcase rule == ruleNoChannel:\n", "TestMS1o_")
m("MS-1 arm off withholds every line, not only what charge delivered", "internal/steer/decide.go",
  "\t\tif plans[i].state == LineStateDelivered {\n\t\t\tplans[i].suppress(SuppressedByArmOff)\n",
  "\t\tif plans[i].state != LineStateCandidate {\n\t\t\tplans[i].suppress(SuppressedByArmOff)\n", "TestMS1o_")
m("MS-1 the budget scan passes over a line arm off withheld", "internal/steer/budget.go",
  " && !s.is(b, s.out[bkSuppressedBy], string(SuppressedByArmOff)) {\n",
  " {\n", "TestMS1o_")
m("MS-1 recap prints under arm off", "cmd/rashomon/recap_turn.go",
  "\tif merr != nil || armOff(root) {\n",
  "\tif merr != nil {\n", "TestMS1n_")
m("MS-1 armOff runs in every arm", "internal/steer/decide.go",
  "\tif arm == ArmOff {\n\t\tnote, flag = armOff(plans)\n",
  "\tif true {\n\t\tnote, flag = armOff(plans)\n", "TestMS1b_")
m("MS-1 the lab marker not checked (the arm env would defeat the holdout on a normal store)", "cmd/rashomon/steerenv.go",
  "\tif arm == \"\" || store.Lab(root) {\n",
  "\tif true {\n", "TestMS1v_")
