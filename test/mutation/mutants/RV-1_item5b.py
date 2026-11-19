# RV-1 item 5b: while the coverage notice stands (the checks off, the notice
# said, no line since saying they are on), the Stop and the catch-up drop the
# bare incomplete line, which says nothing the notice did not; a line with any
# rider prints, and the catch-up says no notice. Each break below prints the
# bare line under the notice (TestRV1_5f_ ii), drops it with no notice
# standing (TestRV1_5f_ iii), drops a line with a rider (TestRV1_5f_ v and
# vi's control), quiets the Stop but not the catch-up (TestRV1_5f_ vi), keeps
# the notice standing after the cleared line (TestRV1_5f_ iv), says the notice
# at the catch-up (TestRV1_5g_), or moves watch's refusal off its ruled text
# (TestRV1_5d_).
RS = "cmd/rashomon/recap_stop.go"
RV = "internal/recap/value.go"
DROP = "\tif s.stands && recap.BareIncomplete(s.view, l, sessionID, fromPlugin) {\n"
m("RV-1 item 5b the bare incomplete line prints under a standing notice", RS,
  DROP, "\tif false && recap.BareIncomplete(s.view, l, sessionID, fromPlugin) {\n",
  "TestRV1_5f_")
m("RV-1 item 5b the bare incomplete line is dropped with no notice standing", RS,
  DROP, "\tif recap.BareIncomplete(s.view, l, sessionID, fromPlugin) {\n",
  "TestRV1_5f_")
m("RV-1 item 5b a line with a rider is dropped as bare", RV,
  " &&\n\t\tline == prefix+v.Summary.Sentence+\".\\n\"+pointer(sessionID, fromPlugin)\n", "\n",
  "TestRV1_5f_")
m("RV-1 item 5b the notice stands at the Stop alone", RS,
  "\tstands := noticeStands(st, p.SessionID, &v.Snap)\n",
  "\tstands := event == install.EventStop && noticeStands(st, p.SessionID, &v.Snap)\n",
  "TestRV1_5f_")
m("RV-1 item 5b the cleared line does not end the standing notice", RS,
  " &&\n\t\t!st.NoticeTaken(sessionID, store.FileNoticeCoverageClear)\n}\n", "\n}\n",
  "TestRV1_5f_")
m("RV-1 item 5b the catch-up says the coverage notice", RS,
  "\tif event == install.EventStop {\n\t\tnotice, marker = coverageNotice(",
  "\tif true {\n\t\tnotice, marker = coverageNotice(",
  "TestRV1_5g_")
m("RV-1 item 5b watch's refusal leaves its ruled lead-in", "cmd/rashomon/watch.go",
  "\"the screen above is yours to agree to\"", "\"the screen above is yours to approve\"",
  "TestRV1_5d_")
# The notice stands only while the checks are off: with them back on and the
# record incomplete for another reason, the sentence prints (TestRV1_5f_ vii).
m("RV-1 item 5b the notice stands with the checks back on", RS,
  "\treturn verify.CoverageOff(snap.Notes) && st.NoticeTaken(sessionID, store.FileNoticeCoverage) &&\n",
  "\treturn st.NoticeTaken(sessionID, store.FileNoticeCoverage) &&\n",
  "TestRV1_5f_")
# BareIncomplete holds only for an incomplete record whose row is unknown;
# each break drops one of the two terms (TestBareIncomplete_).
m("RV-1 item 5b a complete record's line reads as bare", RV,
  "\treturn v.Snap.Recording != verify.Complete && v.Summary.Row == verify.RowUnknown &&\n",
  "\treturn v.Summary.Row == verify.RowUnknown &&\n",
  "TestBareIncomplete_")
m("RV-1 item 5b a line of another row reads as bare", RV,
  "\treturn v.Snap.Recording != verify.Complete && v.Summary.Row == verify.RowUnknown &&\n",
  "\treturn v.Snap.Recording != verify.Complete &&\n",
  "TestBareIncomplete_")
