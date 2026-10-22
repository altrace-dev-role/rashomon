# TRU-5: rashomon consent appends one record holding the hash of the screen
# it showed.
m("TRU-5 consent hashes other bytes than it showed", "cmd/rashomon/consent.go",
  "\trec, err := plan.record(screen, ans)\n", "\trec, err := plan.record(append(screen, '\\n'), ans)\n",
  "TestTRU5_ConsentAppends")
m("TRU-5 consent appends nothing", "cmd/rashomon/consent.go",
  "\tif err := consent.Append(plan.root, rec); err != nil {\n", "\tif err := error(nil); err != nil {\n",
  "TestTRU5_ConsentAppends")
