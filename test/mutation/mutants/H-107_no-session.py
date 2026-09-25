# H-107 -- payloads that name no session: a harness that runs Claude Code's
# hooks without being Claude Code (per Cursor's documentation, not measured
# here, Cursor names a conversation_id where Claude Code sends session_id).
# The first break is the defect as it shipped; the rest are the other ways to
# lose the fix -- the wrong code, and a pointer at a session nobody has.
m("H-107 the recap falls back to the newest run for a turn that names no session", "cmd/rashomon/main.go",
  "\td := digest.Sessionless(now)\n\tif payload.SessionID != \"\" {\n",
  "\td := digest.Sessionless(now)\n\tif newest, _, _ := st.NewestRun(); payload.SessionID == \"\" && newest != \"\" {\n\t\tpayload.SessionID = newest\n\t}\n\tif payload.SessionID != \"\" {\n",
  "TestH107_")
m("H-107 a turn that names no session is reported as no_store", "internal/digest/digest.go",
  "\treturn unread(now, \"\", \"\", ReasonNoSessionID)", "\treturn unread(now, \"\", \"\", ReasonNoStore)",
  "TestH107_|TestSessionless_|TestLineWithNoSession")
m("H-107 the pointer names a session a sessionless turn does not have", "internal/recap/recap.go",
  "\tif sessionID != \"\" {\n\t\tb.WriteString(\" --session \")", "\tif true {\n\t\tb.WriteString(\" --session \")",
  "TestH107_|TestLineWithNoSession")
