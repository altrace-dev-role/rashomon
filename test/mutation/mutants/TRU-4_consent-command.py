# TRU-4: rashomon consent follows watch's terminal rule, records consent_yes
# under --yes, and refuses a machine with nothing installed.
m("TRU-4 consent runs with nothing installed", "cmd/rashomon/consent.go",
  "\tif plan.installID == \"\" {\n", "\tif false {\n",
  "TestTRU4_Consent")
m("TRU-4 consent records the watch via", "cmd/rashomon/watch.go",
  "\tif verb == \"consent\" {\n", "\tif false {\n",
  "TestTRU4_Consent")
