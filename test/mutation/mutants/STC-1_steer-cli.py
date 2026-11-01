# STC-1 (card STEER-CLI1, claims no requirement row): the user's controls over
# AutoSteer. The config reader behind steer.Context.SteerOff is read by the
# hook from the user's key or the project's, an unreadable file clamps, and
# SteerOff clamps a note to a flag; `steer off|on` write under
# UpdateConfig's lock; `steer log`, `steer explain` and `scorecard` print only
# closed values; the screen names the controls. Each break removes one rule
# and names the test that holds it.
H = "internal/hook/steerstep.go"
PC = "internal/hook/promptcontext.go"
D = "internal/steer/decide.go"
C = "internal/store/config.go"
S = "cmd/rashomon/verbs_steer.go"
SC = "cmd/rashomon/verbs_scorecard.go"
SM = "cmd/rashomon/verbs_summary.go"
VS = "cmd/rashomon/verbs_switch.go"
RS = "cmd/rashomon/recap_stop.go"
DP = "internal/disclose/paragraphs.go"
SCREEN = "internal/consent/screen.go"

# The hook reads the switch.
m("STC-1 the config is ignored at the hook", H,
  "\t\tSteerOff:         steerOff,\n", "\t\tSteerOff:         false,\n", "TestH145_|TestSTC1b_|TestSTC1g_|TestSTC1h_")
m("STC-1 the user's key is ignored", H,
  "\tif c.SteerOffUser() {\n\t\treturn true, false\n\t}\n", "", "TestH145_|TestSTC1h_")
m("STC-1 the project's key is ignored", H,
  "\treturn len(c.SteerProjects) > 0 && c.SteerOffFor(baseline.Key(p.cwd)), false\n", "\treturn false, false\n", "TestSTC1b_ProjectKeyIsThatProjectsOnly|TestSTC1h_")
m("STC-1 every project reads off for one's key", H,
  "\treturn len(c.SteerProjects) > 0 && c.SteerOffFor(baseline.Key(p.cwd)), false\n", "\treturn len(c.SteerProjects) > 0, false\n", "TestSTC1b_ProjectKeyIsThatProjectsOnly")
m("STC-1 an unreadable config reads as the notes on", H,
  "\tif err != nil {\n\t\treturn true, true\n\t}\n\tif c.SteerOffUser() {", "\tif err != nil {\n\t\treturn false, false\n\t}\n\tif c.SteerOffUser() {", "TestSTC1g_")
m("STC-1 the prompt's context has no project to look up", PC,
  " permissionMode: permissionMode, cwd: cwd,\n", " permissionMode: permissionMode,\n", "TestSTC1h_")

# Off is a clamp: nothing for the agent, a flag for the user (tech 4.2.4).
m("STC-1 steer off is not a clamp, a note reaches the agent", D,
  "\tcase ctx.SteerOff:\n\t\treturn reason(SuppressedBySteerOff)\n", "", "TestSTC1f_|TestH145_|TestST_ClampsRecordTheirReason")
m("STC-1 an unreadable config is logged as the user's switch", D,
  "\tcase ctx.ConfigUnreadable:\n\t\treturn reason(SuppressedByConfigUnreadable)\n", "", "TestSTC1g_")
m("STC-1 the hook does not say the config is unreadable", H,
  "\t\treturn true, true\n", "\t\treturn true, false\n", "TestSTC1g_")
m("STC-1 steer off clamps for another reason", D,
  "\t\treturn reason(SuppressedBySteerOff)\n", "\t\treturn reason(SuppressedByConsentMissing)\n", "TestSTC1f_|TestST_ClampsRecordTheirReason")

# The config: scopes, lock, unknown keys.
m("STC-1 only the exact word off reads as off", C,
  "func steerSwitchOff(v string) bool { return v != \"\" && v != SwitchOn }", "func steerSwitchOff(v string) bool { return v == SwitchOff }", "TestSTC1n_")
m("STC-1 an unrecognised user switch value reads as on", C,
  "func (c Config) SteerOffUser() bool { return steerSwitchOff(c.SteerUser) }", "func (c Config) SteerOffUser() bool { return c.SteerUser == SwitchOff }", "TestSTC1n_")
m("STC-1 a project's switch reads for every project", C,
  "\treturn c.SteerOffUser() || steerSwitchOff(c.SteerProjects[project])\n", "\treturn c.SteerOffUser() || len(c.SteerProjects) > 0\n", "TestSTC1_ScopesAreEachOwn")
m("STC-1 the user's steer switch is not a scope", C,
  "\treturn c.SteerOffUser() || steerSwitchOff(c.SteerProjects[project])\n", "\treturn steerSwitchOff(c.SteerProjects[project])\n", "TestSTC1_ScopesAreEachOwn")
m("STC-1 a project's summary switch is ignored", C,
  "\treturn c.SummaryOn() && c.SummaryProjects[project] != SwitchOff\n", "\treturn c.SummaryOn()\n", "TestSTC1_ScopesAreEachOwn|TestSTC1_SummaryScopes")
m("STC-1 the config writer takes no lock", C,
  "\tunlock, _, err := lockCounted(lf, configLockBudget)\n", "\tunlock, err := func() {}, error(nil)\n", "TestSTC1_ConcurrentSwitchesBothLand")
m("STC-1 a later build's key is dropped on write", C,
  "\tfor k, v := range c.extra {\n", "\tfor k, v := range map[string]json.RawMessage{} {\n", "TestSTC1_ConfigKeepsWhatItDoesNotName")
m("STC-1 a file that does not parse is replaced", C,
  "\tc, err := ReadConfig(root)\n\tif err != nil && !(repair && errors.Is(err, ErrConfigUnreadable)) {\n\t\treturn Config{}, err\n\t}\n\tchange(&c)", "\tc, _ := ReadConfig(root)\n\tchange(&c)", "TestSTC1_UnparsableConfigIsNotReplaced")
m("STC-1 --repair does not replace an unparseable file", C,
  "!(repair && errors.Is(err, ErrConfigUnreadable))", "true", "TestSTC1o_")
m("STC-1 the Stop line ignores the project's summary key", RS,
  "summary: summaryOn(st.Root(), key),", "summary: summaryOn(st.Root(), \"\"),", "TestSTC1m_")

# The verbs.
m("STC-1 steer on --project leaves the project's key", S,
  "\t\t\tdelete(c.SteerProjects, key)\n", "", "TestSTC1b_ProjectScopeOnlyThatProject")
m("STC-1 steer off --project writes the user's key", S,
  "\t\tcase project && off:\n", "\t\tcase off:\n", "TestSTC1_SteerOffUserWritesTheUsersKey|TestSTC1b_ProjectScopeOnlyThatProject")
m("STC-1 steer says nothing of an unreadable config", S,
  "\tif errors.Is(err, store.ErrConfigUnreadable) {\n\t\t_, err = fmt.Fprintln(stdout, unreadableLine(", "\tif false {\n\t\t_, err = fmt.Fprintln(stdout, unreadableLine(", "TestSTC1o_")
m("STC-1 steer explain says nothing of an unreadable config", S,
  "errors.Is(err, store.ErrConfigUnreadable) {\n\t\t\tfmt.Fprintln(", "false {\n\t\t\tfmt.Fprintln(", "TestSTC1o_")
m("STC-1 steer on an unreadable file gives the raw error", S,
  "\tif errors.Is(err, store.ErrConfigUnreadable) {\n\t\treturn unreadableConfigText(", "\tif false {\n\t\treturn unreadableConfigText(", "TestSTC1o_")
m("STC-1 --repair works in an agent session", S,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" {\n\t\treturn fmt.Errorf(\"%s is set, so this is an agent session; nothing was written. Run `rashomon steer on --repair`",
  "\tif s := \"\"; s != \"\" {\n\t\treturn fmt.Errorf(\"%s is set, so this is an agent session; nothing was written. Run `rashomon steer on --repair`", "TestSTC1o_")
m("STC-1 steer on and off work in an agent session", S,
  "\tif s := selfguard.AgentSignal(os.Getenv); s != \"\" {\n\t\treturn fmt.Errorf(\"%s is set, so this is an agent session; nothing was written. Run `rashomon steer %s`",
  "\tif s := \"\"; s != \"\" {\n\t\treturn fmt.Errorf(\"%s is set, so this is an agent session; nothing was written. Run `rashomon steer %s`", "TestSTC1q_")
m("STC-1 --repair replaces without the person saying yes", S,
  "\tif !ok {\n\t\treturn errors.New(\"nothing was written\")\n\t}\n", "", "TestSTC1o_")
m("STC-1 --repair asks about a file that reads", S,
  "\tif err == nil {\n\t\treturn steerSet(stdout, false, project)\n\t}\n", "", "TestSTC1o_")
m("STC-1 --repair is taken with off", S,
  "\t\t\tif args[0] != \"on\" {\n", "\t\t\tif false {\n", "TestSTC1o_")
m("STC-1 config_unreadable has no sentence", S,
  "\tsteer.SuppressedByConfigUnreadable:  \"config.json did not read, so the notes were off; nothing reached the agent (fix or delete the file, or run rashomon steer on --repair)\",\n", "", "TestSTC1e_EveryReasonIsExplained")
m("STC-1 bare summary reports the user's switch alone", SM,
  "\tif !c.SummaryOnFor(key) {\n\t\tstate = \"off\"\n\t}\n", "\tif !c.SummaryOn() {\n\t\tstate = \"off\"\n\t}\n", "TestSTC1_SummaryScopes|TestSTC1m_")
m("STC-1 bare summary leaves out the project's scope", SM,
  "\tif c.SummaryProjects[key] == store.SwitchOff {\n\t\tproj = \"off\"\n\t}\n\treturn fmt.Sprintf(\"summary:", "\treturn fmt.Sprintf(\"summary:", "TestSTC1_SummaryScopes")
m("STC-1 steer log drops the delivery", S,
  "\t\t\tr.Level, r.State, r.SuppressedBy, r.Delivery)\n", "\t\t\tr.Level, r.State, r.SuppressedBy, \"-\")\n", "TestSTC1c_SteerLogPrintsClosedFields")
m("STC-1 steer log --json drops the level", S,
  "\tLevel        string `json:\"level\"`\n", "\tLevel        string `json:\"-\"`\n", "TestSTC1c_SteerLogPrintsClosedFields")
m("STC-1 steer log prints a value outside its vocabulary", S,
  "\treturn \"?\"\n}\n\n// vocab", "\treturn v\n}\n\n// vocab", "TestSTC1c_SteerLogPrintsClosedFields")
m("STC-1 steer log prints every decision, not the last twenty", S,
  "\tif !all && len(rows) > logTail {\n", "\tif false {\n", "TestSTC1c_SteerLogIsTheLastTwentyUnlessAll")
m("STC-1 steer explain names the oldest decision", S,
  "\tr := rows[len(rows)-1]\n", "\tr := rows[0]\n", "TestSTC1e_ExplainNamesTheLastDecision")
m("STC-1 a suppressed_by reason has no sentence", S,
  "\tsteer.SuppressedByLate:              \"the note arrived after the moment it applied to\",\n", "", "TestSTC1e_EveryReasonIsExplained")
m("STC-1 scorecard counts a held-out note as delivered", SC,
  "\t\t\trow.Holdout++\n", "\t\t\trow.Delivered++\n", "TestSTC1d_ScorecardCountsByPattern")
m("STC-1 scorecard counts a suppressed line as other", SC,
  "\t\tcase r.Delivery != \"-\" || r.State == string(steer.LineStateSuppressed):\n", "\t\tcase r.Delivery != \"-\":\n", "TestSTC1d_ScorecardCountsByPattern")
m("STC-1 scorecard drops the reasons", SC,
  "\t\tif r.SuppressedBy != \"-\" {\n", "\t\tif false {\n", "TestSTC1d_ScorecardCountsByPattern")

# What the verbs print from a file a hand wrote.
m("STC-1 safeText lets the C1 controls through", VS,
  "case r < 0x20, r >= 0x7f && r <= 0x9f,", "case r < 0x20, r == 0x7f,", "TestSTC1p_")
m("STC-1 safeText lets the bidi overrides through", VS,
  ", r >= 0x202a && r <= 0x202e,", ",", "TestSTC1p_")
m("STC-1 safeText lets the bidi isolates through", VS,
  ", r >= 0x2066 && r <= 0x2069:", ":", "TestSTC1p_")
m("STC-1 safeText maps a neighbour of the bidi range too", VS,
  "r >= 0x202a && r <= 0x202e", "r >= 0x2029 && r <= 0x202f", "TestSTC1p_")

# The screen.
m("STC-1 the screen says every note is still shown to you", DP,
  "is shown to you as a flag where the call has room for one (one flag per call, five per prompt), and nothing is shown on an event that has no way to carry one;", "is still shown to you;", "TestSTC1l_")
m("STC-1 the screen stops naming the steer switch", DP,
  "`rashomon steer off --user` stops the notes to your agent, and ", "", "TestSTC1l_")
m("STC-1 the closing table stops naming the steer switch", SCREEN,
  "\t{\"Stop the notes to your agent\", \"rashomon steer off --user\"},\n", "\t{\"Stop the notes; remove the hooks\", \"rashomon detach\"},\n", "TestSTC1l_")
