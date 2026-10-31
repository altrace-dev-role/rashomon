# STC-1 (card STEER-CLI1, claims no requirement row): the user's controls over
# AutoSteer. The config reader behind steer.Context.SteerOff is read by the
# hook from the user's key or the project's, an unreadable file is quiet, and
# SteerOff silences a flag as it does a note; `steer off|on` write under
# UpdateConfig's lock; `steer log`, `steer explain` and `scorecard` print only
# closed values; the screen names the controls. Each break removes one rule
# and names the test that holds it.
H = "internal/hook/steerstep.go"
PC = "internal/hook/promptcontext.go"
D = "internal/steer/decide.go"
C = "internal/store/config.go"
S = "cmd/rashomon/verbs_steer.go"
SC = "cmd/rashomon/verbs_scorecard.go"
RS = "cmd/rashomon/recap_stop.go"
DP = "internal/disclose/paragraphs.go"
SCREEN = "internal/consent/screen.go"

# The hook reads the switch.
m("STC-1 the config is ignored at the hook", H,
  "\t\tSteerOff:   p.steerOff(),\n", "\t\tSteerOff:   false,\n", "TestH145_|TestSTC1b_|TestSTC1g_|TestSTC1h_")
m("STC-1 the user's key is ignored", H,
  "\tif c.SteerOffUser() {\n\t\treturn true\n\t}\n", "", "TestH145_|TestSTC1h_")
m("STC-1 the project's key is ignored", H,
  "\treturn len(c.SteerProjects) > 0 && c.SteerOffFor(baseline.Key(p.cwd))\n", "\treturn false\n", "TestSTC1b_ProjectKeyIsThatProjectsOnly|TestSTC1h_")
m("STC-1 every project reads off for one's key", H,
  "\treturn len(c.SteerProjects) > 0 && c.SteerOffFor(baseline.Key(p.cwd))\n", "\treturn len(c.SteerProjects) > 0\n", "TestSTC1b_ProjectKeyIsThatProjectsOnly")
m("STC-1 an unreadable config reads as the notes on", H,
  "\tif err != nil {\n\t\treturn true\n\t}\n\tif c.SteerOffUser() {", "\tif err != nil {\n\t\treturn false\n\t}\n\tif c.SteerOffUser() {", "TestSTC1g_")
m("STC-1 the prompt's context has no project to look up", PC,
  " permissionMode: permissionMode, cwd: project,\n", " permissionMode: permissionMode,\n", "TestSTC1h_")

# Off silences flags as it does notes.
m("STC-1 steer off silences a note and prints a flag", D,
  "\t\tcase ctx.SteerOff:\n", "\t\tcase ctx.SteerOff && p.level == LevelNudge:\n", "TestSTC1f_")
m("STC-1 steer off suppresses for another reason", D,
  "\t\t\tp.suppress(SuppressedBySteerOff) // the user's switch", "\t\t\tp.suppress(SuppressedByNoChannel) // the user's switch", "TestSTC1f_")

# The config: scopes, lock, unknown keys.
m("STC-1 a project's switch reads for every project", C,
  "\treturn c.SteerOffUser() || c.SteerProjects[project] == SwitchOff\n", "\treturn c.SteerOffUser() || len(c.SteerProjects) > 0\n", "TestSTC1_ScopesAreEachOwn")
m("STC-1 the user's steer switch is not a scope", C,
  "\treturn c.SteerOffUser() || c.SteerProjects[project] == SwitchOff\n", "\treturn c.SteerProjects[project] == SwitchOff\n", "TestSTC1_ScopesAreEachOwn")
m("STC-1 a project's summary switch is ignored", C,
  "\treturn c.SummaryOn() && c.SummaryProjects[project] != SwitchOff\n", "\treturn c.SummaryOn()\n", "TestSTC1_ScopesAreEachOwn|TestSTC1_SummaryScopes")
m("STC-1 the config writer takes no lock", C,
  "\tunlock, _, err := lockCounted(lf, configLockBudget)\n", "\tunlock, err := func() {}, error(nil)\n", "TestSTC1_ConcurrentSwitchesBothLand")
m("STC-1 a later build's key is dropped on write", C,
  "\tfor k, v := range c.extra {\n", "\tfor k, v := range map[string]json.RawMessage{} {\n", "TestSTC1_ConfigKeepsWhatItDoesNotName")
m("STC-1 a file that does not parse is replaced", C,
  "\tc, err := ReadConfig(root)\n\tif err != nil {\n\t\treturn Config{}, err\n\t}\n\tchange(&c)", "\tc, _ := ReadConfig(root)\n\tchange(&c)", "TestSTC1_UnparsableConfigIsNotReplaced")
m("STC-1 the Stop line ignores the project's summary key", RS,
  "summary: summaryOn(st.Root(), key),", "summary: summaryOn(st.Root(), \"\"),", "TestSTC1m_")

# The verbs.
m("STC-1 steer on --project leaves the project's key", S,
  "\t\t\tdelete(c.SteerProjects, key)\n", "", "TestSTC1b_ProjectScopeOnlyThatProject")
m("STC-1 steer off --project writes the user's key", S,
  "\t\tcase project && off:\n", "\t\tcase off:\n", "TestSTC1_SteerOffUserWritesTheUsersKey|TestSTC1b_ProjectScopeOnlyThatProject")
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

# The screen.
m("STC-1 the screen stops naming the steer switch", DP,
  "`rashomon steer off --user` stops the notes for you, and ", "", "TestSTC1l_")
m("STC-1 the closing table stops naming the steer switch", SCREEN,
  "\t{\"Stop the notes\", \"rashomon steer off --user\"},\n", "\t{\"Stop the notes; remove the hooks\", \"rashomon detach\"},\n", "TestSTC1l_")
