# DONE1: a Stop whose done flag fired reaches the agent once, at the next
# prompt of the same session (card c4, PF-PG9). The Stop keeps the verdict in
# recap.json beside its claim; the prompt takes it once, before the preflight,
# and routes it as claim-vs-record/done-unverified; steering off (an unreadable
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
  "\tif !named && !proj.lessons.Any() && !done {\n", "\tif !named && !proj.lessons.Any() {\n", "TestDONE1_")
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
m("DONE1 every Stop is flagged", S,
  "\t_, ok := recap.DoneFlag(s.view, sessionID, fromPlugin)\n\treturn ok\n", "\treturn true\n", "TestDONE1b_")
m("DONE1 the Stop's verdict not kept", T,
  "clause, !pending && stop.unverified(d.SessionID, fromPlugin))", "clause, false)", "TestDONE1_")
m("DONE1 the prompt takes nothing", T,
  "pp.unverified = recap.TakeUnverified(st.Root(), pp.SessionID)", "pp.unverified = false", "TestDONE1_")
m("DONE1 the take not written before it answers", R,
  "\ts.Noted[sessionID] = flagged\n\treturn writeState(path, s, sessionID) == nil\n", "\treturn true\n", "TestDONE1g_")
m("DONE1 a taken Stop taken again", R,
  "\tif flagged == \"\" || flagged == s.Noted[sessionID] {\n", "\tif flagged == \"\" {\n", "TestDONE1_")
m("DONE1 the take reads another session's flag", R,
  "\tflagged := s.Unverified[sessionID]\n", "\tflagged := \"\"\n\tfor _, v := range s.Unverified {\n\t\tflagged = v\n\t}\n", "TestDONE1b_")
m("DONE1 the unverified map not capped", R,
  "[]map[string]string{s.Spoken, s.Checked, s.Shown, s.Unverified, s.Noted}", "[]map[string]string{s.Spoken, s.Checked, s.Shown}", "TestDONE1i_")
