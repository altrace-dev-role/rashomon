# RES-8: a report over many sessions costs what rendering the one it is
# currently assembling costs, not what holding every session's records (or
# every session's whole final message) at once would. Reverting either half
# of that -- the per-session read step, or dropping the final message once
# its two readers have run -- turns the report command's peak RSS back into
# a figure that scales with how many sessions the store holds.
m("RES-8 Build reads every session before assembling any, instead of one at a time", "internal/report/gather/gather.go",
  "\tfor _, name := range names {\n\t\tsi, err := readSession(st, name, byDir, gapsSkipped, cfg, forgottenBefore)\n\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\trep.Sessions = append(rep.Sessions, report.AssembleSession(si, nonoConfigured, forgotten, forgottenBefore))\n\t}\n",
  "\tvar sessions []report.SessionInput\n\tfor _, name := range names {\n\t\tsi, err := readSession(st, name, byDir, gapsSkipped, cfg, forgottenBefore)\n\t\tif err != nil {\n\t\t\treturn nil, err\n\t\t}\n\t\tsessions = append(sessions, si)\n\t}\n\tfor _, si := range sessions {\n\t\trep.Sessions = append(rep.Sessions, report.AssembleSession(si, nonoConfigured, forgotten, forgottenBefore))\n\t}\n",
  "TestRES8_")
m("RES-8 the whole final message is kept past the session it belongs to",
  "internal/report/assemble.go",
  "\t// Both of full's readers have now run; a report holding many sessions\n\t// must not hold every one's whole final message past this point.\n\tsess.Account = sess.Account.dropFull()\n",
  "",
  "TestRES8_")
