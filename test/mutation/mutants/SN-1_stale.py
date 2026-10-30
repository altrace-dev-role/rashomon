# SN-1 stale evidence at the write (card STALE-NUDGE1, with INDEX-POST1 folded
# in by D164; the trigger by D166). Each break removes one rule: the key that
# tells a gate once per prompt, the required gate, the call's own files, the
# first write after the green, the project index the post reads, no raise
# before a family is established, the arm the write must take, and the one
# key walk the spec and the index share; and names the test that holds it.
V = "internal/verify/steer.go"
H = "internal/hook/steerstep.go"
m("SN-1 the key is per call, not per gate", "internal/verify/tosteer_card.go",
  "\tcase pattern.StaleEvidence:\n\t\treturn string(f.Gate.GateID), \"\"\n",
  "\tcase pattern.StaleEvidence:\n\t\treturn string(f.Gate.GateID) + \"|\" + strconv.FormatInt(at.Seq, 10), \"\"\n",
  "TestSN1i_OncePerGatePerPrompt")
m("SN-1 an advisory gate raises", V,
  "if !g.Required || g.State != Stale", "if g.State != Stale", "TestSN1c_AdvisoryGatesStaySilent")
m("SN-1 a write beside the call is named as its own", V,
  "i < len(writes) && writes[i].Seq == decl; i++ {", "i < len(writes); i++ {", "TestSN1d_ForeignMoveIsNamedNotBlamed")
m("SN-1 every write after the green raises", V,
  "\t\t\treturn w.Seq == decl\n", "\t\t\treturn g.LastWrite != nil && g.LastWrite.Seq == decl\n",
  "TestSN1_TheWriteAfterGreenRaisesOnce")
m("SN-1 the post judges with no index", H,
  "idx, _ = verify.ReadIndex(verify.IndexPath(p.st.Root(), key))",
  "_, _ = verify.ReadIndex(verify.IndexPath(p.st.Root(), key))", "TestIP1_PostJudgesUnderTheProjectIndex")
m("SN-1 a gate raises before a family is established", V,
  "if c.learned && event == string(steer.EventPostToolUse) {", "if event == string(steer.EventPostToolUse) {",
  "TestSN1c_AdvisoryGatesStaySilent")
m("SN-1 a write is judged without taking the arm", H,
  "return key, p.st.TakeStaleArm(p.sessionID)", "return key, true", "TestSN1g_OnlyAnArmedRelevantWriteIsJudged")
m("SN-1 the spec walks from a key that is not the project's", H,
  "p.sp = discoverSpecAt(p.cwd, key,", "p.sp = discoverSpecAt(p.cwd, p.cwd,", "TestSP15_HookDiscoversTheNearestSpec")
