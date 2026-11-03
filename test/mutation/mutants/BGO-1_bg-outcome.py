# BGO-1: a background run is judged by its end (card BG-OUTCOME phase 1):
# the catch-up keeps an outcome_join for each end a notification prompt
# states, read from the closed tail of the summary only, and newRun gives a
# Backgrounded run that end's outcome and exit code, for classify's gate runs
# and a declared gate's line runs alike. Each break removes one
# rule and names the tests that hold it.
RT = "cmd/rashomon/recap_turn.go"
TE = "cmd/rashomon/taskend.go"
VC = "internal/verify/classify.go"
VD = "internal/verify/declared.go"
m("BGO-1 the catch-up keeps no end", RT,
  "\tkeepTaskEnds(st, sessionID, prompt, now)\n", "", "TestBGO1a_|TestBGO1c_")
m("BGO-1 no run takes its end", VC,
  "\tif end != nil && g.Backgrounded {\n", "\tif end != nil && false {\n", "TestBGO1a_|TestBGO1b_|TestBGO1g_")
m("BGO-1 a foreground run takes an end", VC,
  "\tif end != nil && g.Backgrounded {\n", "\tif end != nil {\n", "TestBGO1h_")
m("BGO-1 classify's gate runs take no end", VC,
  "\tended := taskEnds(run)\n", "\tended := map[string]*store.OutcomeJoin(nil)\n", "TestBGO1g_")
m("BGO-1 a declared gate's line runs take no end", VD,
  "\tended := taskEnds(run)\n", "\tended := map[string]*store.OutcomeJoin(nil)\n", "TestBGO1a_|TestBGO1b_|TestBGO1d_")
m("BGO-1 a failed end reads ok", TE,
  "\t\toutcome = store.ExecFailed\n", "", "TestBGO1a_|TestBGO1c_")
m("BGO-1 the first code the summary names is read", TE,
  "\t\ti := bytes.LastIndex(summary, []byte(\" failed with exit code \"))\n",
  "\t\ti := bytes.Index(summary, []byte(\" failed with exit code \"))\n", "TestBGO1d_")
m("BGO-1 a quoted notification is read", TE,
  "\t\tif !bytes.HasPrefix(rest, []byte(taskOpen)) {\n\t\t\treturn nil\n\t\t}\n",
  "\t\tif i := bytes.Index(rest, []byte(taskOpen)); i < 0 {\n\t\t\treturn nil\n\t\t} else {\n\t\t\trest = rest[i:]\n\t\t}\n", "TestBGO1e_")
