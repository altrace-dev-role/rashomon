# SEED-1 (card SEED1, PF-AU1 and PF-AU2): `rashomon audit` counts the
# corrections a person made to their agent from the transcripts on disk, each
# kind once, beside its controls, and prints a card of counts and our words
# only, over the window asked for, writing nothing.
S = "internal/seed/seed.go"
C = "internal/seed/class.go"
T = "internal/trace/claudecode/claudecode.go"
K = "internal/seed/card.go"
A = "cmd/rashomon/audit.go"

# The kinds.
m("SEED-1 a rejection is not counted", S,
  "\tcase trace.DeniedByPerson:\n\t\tf.add(rejected, c.c, s.Session)\n", "\tcase trace.DeniedByPerson:\n", "TestSEED1a_")
m("SEED-1 the text fallback misses a rejection", T,
  "\tcase denied && report.UserRejected(text):\n", "\tcase false && report.UserRejected(text):\n", "TestSEED1a_")
m("SEED-1 a rejection does not end the turn", S,
  "\t\tf.add(rejected, c.c, s.Session)\n\t\tf.turn = turn{}\n",
  "\t\tf.add(rejected, c.c, s.Session)\n", "TestSEED1a_")
m("SEED-1 a rule's refusal is not counted", S,
  "\tcase trace.DeniedByRule:\n\t\tf.add(blocked, c.c, s.Session)\n", "\tcase trace.DeniedByRule:\n", "TestSEED1a_")
m("SEED-1 a failure tried again is not counted", S,
  "\t\tif p.name == name {\n\t\t\tf.add(failedAgain, p.c, session)\n", "\t\tif p.name == name {\n", "TestSEED1a_")
m("SEED-1 any later call counts as the retry", S,
  "\t\tif p.name == name {\n", "\t\tif true {\n", "TestSEED1a_")
m("SEED-1 a subagent's echoed interrupt counts", S,
  "\tif !f.sub && !s.Sidechain && f.turn.acted &&", "\tif f.turn.acted &&", "TestSEED1a_")
m("SEED-1 a line counts with no action since the last", S,
  "\tif !f.sub && !s.Sidechain && f.turn.acted &&", "\tif !f.sub && !s.Sidechain &&", "TestSEED1a_")
m("SEED-1 a no answering its question counts", S,
  "\t\tcase pushback(text) && !(f.turn.question && startsNo(text)):\n", "\t\tcase pushback(text):\n", "TestSEED1a_")
m("SEED-1 no problem counts as a pushback", C,
  "\t\tcase \"problem\", \"worries\", \"rush\", \"thanks\", \"need\":\n", "\t\tcase \"\":\n", "TestSEED1a_")

# The transcript adapter (internal/trace/claudecode): the words audit reads.
m("SEED-1 markup in the person's slot counts as their words", T,
  "\tcase strings.HasPrefix(t, \"<\"):\n", "\tcase false:\n", "TestSaid_OriginAndSignal|TestSEED_AuditGolden")
m("SEED-1 the interrupt marker is not a signal", T,
  "\tcase t == interruptMark:\n", "\tcase false:\n", "TestSEED1a_|TestSaid_OriginAndSignal")
m("SEED-1 Claude Code's own context lines count", T,
  "\tif l.IsMeta {\n\t\treturn\n\t}\n", "", "TestSEED1a_|TestRead_PassesOver")
m("SEED-1 an edit's missing text is no tool error", T,
  "\tcase strings.HasPrefix(t, \"String to replace not found in file\"):\n", "\tcase false:\n",
  "TestSEED1a_|TestToolError_Openings")
m("SEED-1 a denial kind the reader does not know is read from its text", T,
  "\tdefault:\n\t\tr.Denied = trace.DeniedUnrecognised\n\t\treturn s\n\t}\n", "\tdefault:\n\t}\n",
  "TestResult_Denials|TestSEED_AuditGolden")

# The classes.
m("SEED-1 a shell call is not read for its runner", C,
  "\tcase trace.ActExec:\n\t\treturn shellClass(c.Input)\n", "\tcase trace.ActExec:\n\t\treturn cShell\n", "TestSEED1a_")
m("SEED-1 a shell call is not a command", T,
  "\tcase \"Bash\":\n\t\treturn trace.ActExec\n", "\tcase \"Bash\":\n\t\treturn trace.ActOther\n", "TestSEED1a_|TestActClass_")
m("SEED-1 a check runner is a test run", C,
  "\t\tif *s.RunnerKind == shape.RunnerTest {\n", "\t\tif *s.RunnerKind != shape.RunnerBuild {\n",
  "TestSEED1a_OnlyATestRunnerIsATestRun")
m("SEED-1 a test file is not told apart", C,
  "\tcase shape.LabelTestFile:\n", "\tcase \"never\":\n", "TestSEED1b_")

# The card.
m("SEED-1 the card lists a row seen in one session", K,
  "\t\tif r.Sessions < cardRecurAt || n == cardMaxRows {\n", "\t\tif n == cardMaxRows {\n", "TestSEED1b_")

# The command.
m("SEED-1 audit drops the stale count", A,
  "\ts.Stale, s.Unread = found.Stale, s.Unread+unread\n", "\ts.Unread = s.Unread + unread\n", "TestSEED1d_")
m("SEED-1 audit accepts a zero window", A,
  "if err != nil || n < 1 || n > spend.MaxDays {", "if err != nil || n > spend.MaxDays {", "TestSEED1d_")
m("SEED-1 audit ignores the window", A,
  "spend.Discover(configDir, spend.WindowStart(end, days))", "spend.Discover(configDir, time.Time{})", "TestSEED1d_")
