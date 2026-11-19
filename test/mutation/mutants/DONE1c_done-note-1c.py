# DONE1c: a Stop reads the done claim of the prompt it names, and a Stop
# whose named prompt made no call of the main agent marks no turn checked. A
# later Stop of the same prompt whose claim is read and backed deletes the
# entry a flagged Stop left, and only that prompt's entry. The kill row is the
# named test's.
T = "cmd/rashomon/recap_turn.go"
R = "internal/recap/state.go"

# The Stop names its prompt: without the id, a text-only Stop is read as the
# turn before it.
m("DONE1c the Stop reads no prompt id", T,
  "payload.SessionID, payload.StopPromptID, payload.LastAssistantMessage, now)",
  "payload.SessionID, \"\", payload.LastAssistantMessage, now)", "TestDONE1t_")

# The delete: a backed claim clears the entry, so the next prompt carries no
# note.
m("DONE1c a backing Stop leaves the entry", R,
  "\tif done.Backed && sessionID != \"\" && promptID != \"\" && s.Unverified[sessionID] == promptID {\n"
  "\t\tdelete(s.Unverified, sessionID)\n\t}\n",
  "", "TestDONE1v_")
m("DONE1c an unflagged Stop counts as backed", R,
  "\tif done.Backed && sessionID", "\tif done.Flagged == \"\" && sessionID", "TestDONE1x_")
m("DONE1c the delete takes another prompt's entry", R,
  " && s.Unverified[sessionID] == promptID {\n", " {\n", "TestDONE1x_")

# The checked mark: only a prompt that made a call is marked checked, so a
# text-only Stop does not hide the turn before it from the catch-up.
m("DONE1c a prompt with no call marked checked", R,
  "\tif declared && sessionID != \"\" && promptID != \"\" {\n",
  "\tif sessionID != \"\" && promptID != \"\" {\n", "TestDONE1t_")
