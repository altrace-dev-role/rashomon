# SEED-1 (card SEED1, PF-AU1 and PF-AU2): `rashomon audit` counts the
# corrections a person made to their agent from the transcripts on disk, each
# kind once, beside its controls, and prints a card of counts and our words
# only, over the window asked for, writing nothing.
S = "internal/seed/seed.go"
C = "internal/seed/class.go"
K = "internal/seed/card.go"
A = "cmd/rashomon/audit.go"

# The kinds.
m("SEED-1 a rejection is not counted", S,
  "\tcase \"user-rejected\":\n\t\tf.add(rejected, c.c, l.SessionID)\n", "\tcase \"user-rejected\":\n", "TestSEED1a_")
m("SEED-1 the text fallback misses a rejection", S,
  "\tcase denied && report.UserRejected(text):\n", "\tcase false && report.UserRejected(text):\n", "TestSEED1a_")
m("SEED-1 a rejection read from its text does not end the turn", S,
  "\t\tf.add(rejected, c.c, l.SessionID)\n\t\tf.turn = turn{}\n\tcase denied:\n",
  "\t\tf.add(rejected, c.c, l.SessionID)\n\tcase denied:\n", "TestSEED1a_")
m("SEED-1 a rule's refusal is not counted", S,
  "\tcase \"permission-rule\":\n\t\tf.add(blocked, c.c, l.SessionID)\n", "\tcase \"permission-rule\":\n", "TestSEED1a_")
m("SEED-1 a failure tried again is not counted", S,
  "\t\tif p.name == name {\n\t\t\tf.add(failedAgain, p.c, session)\n", "\t\tif p.name == name {\n", "TestSEED1a_")
m("SEED-1 any later call counts as the retry", S,
  "\t\tif p.name == name {\n", "\t\tif true {\n", "TestSEED1a_")
m("SEED-1 a subagent's echoed interrupt counts", S,
  "\tif !f.sub && !l.IsSidechain && f.turn.acted &&", "\tif f.turn.acted &&", "TestSEED1a_")
m("SEED-1 a line counts with no action since the last", S,
  "\tif !f.sub && !l.IsSidechain && f.turn.acted &&", "\tif !f.sub && !l.IsSidechain &&", "TestSEED1a_")
m("SEED-1 a no answering its question counts", S,
  "\t\tcase pushback(text) && !(f.turn.question && startsNo(text)):\n", "\t\tcase pushback(text):\n", "TestSEED1a_")
m("SEED-1 no problem counts as a pushback", C,
  "\t\tcase \"problem\", \"worries\", \"rush\", \"thanks\", \"need\":\n", "\t\tcase \"\":\n", "TestSEED1a_")

# The classes.
m("SEED-1 a shell call is not read for its runner", C,
  "\tcase \"Bash\":\n\t\treturn shellClass(input)\n", "\tcase \"Bash\":\n\t\treturn cShell\n", "TestSEED1a_")
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
  "spend.Discover(configDir, spend.WindowStart(time.Now(), days))", "spend.Discover(configDir, time.Time{})", "TestSEED1d_")
