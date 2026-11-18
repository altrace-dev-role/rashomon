# DONE1: a Stop whose done flag fired reaches the agent once, at the next
# prompt of the same session (card c4, PF-PG9). The Stop keeps the verdict in
# recap.json beside its claim; the prompt takes it once, before the preflight,
# where its one read of recap.json (Turn) saw it, and routes it as claim-vs-record/done-unverified; steering off (an unreadable
# config reads as off) and no consent drop it before Route and still consume it. The kill row
# is the named test's.
P = "cmd/rashomon/preflight.go"
D = "cmd/rashomon/preflight_done.go"
T = "cmd/rashomon/recap_turn.go"
S = "cmd/rashomon/recap_stop.go"
R = "internal/recap/state.go"

# The delivery: routed beside the preflight, and a prompt that names nothing
# still carries it.
m("DONE1 the delivery dropped", P,
  "ok && done && !withheld(&ctx) {\n", "ok && false && !withheld(&ctx) {\n", "TestDONE1_")
m("DONE1 a prompt naming nothing carries no note", P,
  "\tif !named && !proj.lessons.Any() && !done && p.owed.Seq <= 0 && !briefed && !graphed {\n", "\tif !named && !proj.lessons.Any() && p.owed.Seq <= 0 && !briefed && !graphed {\n", "TestDONE1_")
m("DONE1 the note is a flag, not a note", D,
  "MaxLevel: steer.LevelNudge,", "MaxLevel: steer.LevelFlag,", "TestDONE1_")
m("DONE1 the note carries another kind", D,
  "Kind: \"done-unverified\"", "Kind: \"absent\"", "TestDONE1_")

# The switches: dropped before Route, so nothing prints as a flag.
m("DONE1 steer off does not withhold", D,
  "return ctx.SteerOff || !ctx.Consent\n", "return !ctx.Consent\n", "TestDONE1c_")
m("DONE1 no consent does not withhold", D,
  "return ctx.SteerOff || !ctx.Consent\n", "return ctx.SteerOff\n", "TestDONE1c_")

# The Stop's side: only a Stop that raised the flag keeps it, and the prompt
# takes it once.
# flag is a named result of doneFlag, so the mutant assigns it (=): a := with
# no new name on its left does not build.
m("DONE1 every Stop is flagged", S,
  "\tflag, ok := recap.DoneFlag(s.view, sessionID, fromPlugin)\n\tif !ok {\n\t\treturn \"\", \"\"\n\t}\n",
  "\tflag, _ = recap.DoneFlag(s.view, sessionID, fromPlugin)\n", "TestDONE1b_")
m("DONE1 the Stop's verdict not kept", S,
  "\t\tm.Flagged = digest\n", "", "TestDONE1_")
m("DONE1 the prompt takes nothing", T,
  "pp.unverified = flagged && recap.TakeUnverified(root, d.SessionID)", "pp.unverified = flagged && false", "TestDONE1_")
# The prompt's one read (#279): Turn says whether there is a flag to take, so
# the take, which reads again and writes, runs only then.
m("DONE1 Turn sees no flag to take", R,
  "u != \"\" && u != s.Noted[sessionID]", "u != \"\" && false", "TestDONE1_|TestDONE1j_")
m("DONE1 Turn sees a taken flag again", R,
  "u != \"\" && u != s.Noted[sessionID]", "u != \"\"", "TestDONE1j_")
# Both maps are nil until their one write, as Owed is: the write makes them.
m("DONE1 a Stop flags into a nil map", R,
  "\t\tif s.Unverified == nil {\n\t\t\ts.Unverified = map[string]string{}\n\t\t}\n", "", "TestDONE1g_|TestDONE1j_")
m("DONE1 the take writes into a nil map", R,
  "\tif s.Noted == nil {\n\t\ts.Noted = map[string]string{}\n\t}\n", "", "TestDONE1g_|TestDONE1j_")
m("DONE1 the take not written before it answers", R,
  "\ts.Noted[sessionID] = flagged\n\treturn writeState(path, s, sessionID) == nil\n", "\treturn true\n", "TestDONE1g_")
# The two guards below are shadowed in the hook path: promptNotes takes only
# where recap.Turn already found a flag not yet noted for this session
# (u != "" && u != s.Noted[sessionID], TestDONE1j_), so the acceptance tests
# cannot see the take's own checks. They are judged where the take is
# observed directly (W-M1.9; both survived TestDONE1_ and TestDONE1b_).
m("DONE1 a taken Stop taken again", R,
  "\tif flagged == \"\" || flagged == s.Noted[sessionID] {\n", "\tif flagged == \"\" {\n", "TestDONE1g_")
m("DONE1 the take reads another session's flag", R,
  "\tflagged := s.Unverified[sessionID]\n", "\tflagged := \"\"\n\tfor _, v := range s.Unverified {\n\t\tflagged = v\n\t}\n", "TestDONE1g_")
m("DONE1 the unverified map not capped", R,
  "[]map[string]string{s.Spoken, s.Checked, s.Shown, s.Unverified, s.Noted}", "[]map[string]string{s.Spoken, s.Checked, s.Shown}", "TestDONE1i_")

# Step 1b: the note and the flag once per evidence (verify.DoneEvidence, a
# digest of what the flag judged). A Stop over the evidence whose note already
# reached the agent keeps no flag; the digest does not depend on the gates'
# order; only a prompt whose printed note is the done note marks it
# delivered; the digests are capped with the other maps.
V = "internal/verify/doneevidence.go"
m("DONE-1j the delivered evidence flags again", R,
  " && done.Flagged != s.DoneEvidence[sessionID].Delivered {\n", " {\n", "TestDONE1j_UnchangedEvidenceDeliversOnce|TestDONE1l_")
m("DONE-1k the digest follows the gates' order", V,
  "\tslices.Sort(parts)\n", "", "TestDONE1k_")
# Where no gate is required the row and the sentence read the learned gates,
# so the digest takes the turn gates; the Required-only filter it replaced
# keeps a learned gate's new run out of the digest.
m("DONE-1s the digest reads the required gates alone", V,
  "\tadvisory := Advisory(v.Snap.Gates)\n\tfor i := range v.Snap.Gates {\n\t\tg := &v.Snap.Gates[i]\n\t\tif !TurnGate(*g, advisory) {\n",
  "\tfor i := range v.Snap.Gates {\n\t\tg := &v.Snap.Gates[i]\n\t\tif !g.Required || g.offTurn {\n", "TestDONE1s_")
m("DONE1q an outranked note counts as delivered", T,
  "\tif pp.unverified && note.AdditionalContext != \"\" && note.AdditionalContext == doneText() {\n", "\tif pp.unverified {\n", "TestDONE1q_")
m("DONE1 the done evidence not capped", R,
  "\tcapSessions(s.DoneEvidence, current)\n", "", "TestDONE1i_")
