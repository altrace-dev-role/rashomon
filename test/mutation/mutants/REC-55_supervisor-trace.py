# REC-55: a killed child leaves a hook_killed record carrying the session id
# the supervisor read, written after a newline when the last line is torn,
# and the record never counts as the probe having run.
m("REC-55 the supervisor's record joins a torn line", "internal/store/supervisor.go",
  "\t\tappendOpts{budget: supervisorBudget, bestEffort: true, sealTail: true})",
  "\t\tappendOpts{budget: supervisorBudget, bestEffort: true})", "TestREC55b_")
m("REC-55 a killed probe counts as the probe having run", "internal/report/report.go",
  "\tcase store.ReasonRecordingPaused, store.ReasonHookKilled, store.ReasonSupervisorSpawnFailed:\n",
  "\tcase store.ReasonRecordingPaused, store.ReasonSupervisorSpawnFailed:\n", "TestREC55c_")
m("REC-55 the head scan keeps no session id", "internal/supervise/supervise.go",
  "\t\t\th.SessionID = string(value)\n", "", "TestREC55d_")
