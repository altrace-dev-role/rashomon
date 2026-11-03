# STOP-PRECEDENCE1 (mgr 11-02 ruling (a)): recap.Line's sentences carry the
# turn's platform lines, after the sentences and before the pointer, so a
# failure count also names the package it left unmet; StopFailure, which
# prints no platform line (tech 2.2.11 condition 4), names none, because its
# view reads no platform graph.
RT = "cmd/rashomon/recap_turn.go"
RS = "cmd/rashomon/recap_stop.go"
RC = "internal/recap/recap.go"

m("STP-1 the failure count drops the platform line", RT,
  "stop.rules()+recap.PlatformLines(stop.view, fromPlugin))\n", "stop.rules())\n",
  "TestSTP1_AFailureCountCarriesThePlatformLine")
m("STP-1 the platform line after the pointer", RC,
  "\".\\n\" + lines + pointer(sessionID, fromPlugin), true\n", "\".\\n\" + pointer(sessionID, fromPlugin) + lines, true\n",
  "TestSTP1_AFailureCountCarriesThePlatformLine")
m("STP-1 StopFailure reads the platform graph", RS,
  "\tif event == install.EventStop {\n\t\t// The Stop alone reads the platform graph", "\tif writes {\n\t\t// The Stop alone reads the platform graph",
  "TestSTP1_AFailureCountCarriesThePlatformLine")
