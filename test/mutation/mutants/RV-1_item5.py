# RV-1 item 5: the Stop says once per session what the claim gate's reading
# of the record does to the claim checks, worded by its effect, and once that
# they are on again; two run-directory markers remember each line, taken only
# with the line. watch's agent refusal says the same of a session it leaves
# unrecorded. Each break below says the notice at every Stop (TestRV1_5a_,
# TestRV1_5b_), words a fault no record clears as one that clears
# (TestRV1_5b_), repeats or invents the cleared line (TestRV1_5c_), takes a
# marker without its line (TestRV1_5e_), brings back the per-turn coverage
# sentence or prints any notice (TestRV1_5a_, TestLineCoverageUnverified),
# moves the refusal's clause (TestRV1_5d_), or drops a marker from an
# inventory (TestRV1_5a_, TestTRU23_, TestTRU28_).
RS = "cmd/rashomon/recap_stop.go"
RC = "internal/recap/recap.go"
SAID = "\tif speak && s.marker != \"\" {\n"
m("RV-1 item 5 the notice is said at every Stop the checks are off", RS,
  "\tcase verify.CoverageOff(snap.Notes):\n\t\tif st.NoticeTaken(sessionID, store.FileNoticeCoverage) {\n"
  "\t\t\treturn \"\", \"\"\n\t\t}\n",
  "\tcase verify.CoverageOff(snap.Notes):\n",
  "TestRV1_5a_|TestRV1_5b_")
m("RV-1 item 5 a fault no record clears is worded as one that clears", RS,
  "\t\tif verify.CoverageStuck(run) {\n", "\t\tif false {\n",
  "TestRV1_5b_")
m("RV-1 item 5 the cleared line ignores its own marker", RS,
  " &&\n\t\t!st.NoticeTaken(sessionID, store.FileNoticeCoverageClear):\n", ":\n",
  "TestRV1_5c_")
m("RV-1 item 5 the cleared line needs no notice before it", RS,
  "snap.Recording == verify.Complete && st.NoticeTaken(sessionID, store.FileNoticeCoverage) &&",
  "snap.Recording == verify.Complete &&",
  "TestRV1_5c_")
m("RV-1 item 5 the cleared line's marker is never taken", RS,
  SAID, "\tif speak && s.marker == store.FileNoticeCoverage {\n",
  "TestRV1_5c_")
m("RV-1 item 5 a marker is taken without its line", RS,
  SAID, "\tif s.marker != \"\" {\n",
  "TestRV1_5e_")
m("RV-1 item 5 no marker is taken", "cmd/rashomon/recap_turn.go",
  "\tstop.noticeSaid(st, d.SessionID, speak)\n", "",
  "TestRV1_5a_")
m("RV-1 item 5 the per-turn coverage sentence is back", RC,
  "\tswitch notice {\n",
  "\tif d.Fired(report.DetectorCoverageUnverified) {\n"
  "\t\tsentences = append(sentences, \"coverage unverified (\"+reasonList(d.Coverage.Reasons)+\")\")\n"
  "\t}\n\tswitch notice {\n",
  "TestRV1_5a_|TestLineCoverageUnverified")
m("RV-1 item 5 any notice handed in is printed", RC,
  "\tswitch notice {\n\tcase ChecksOff, ChecksOffRest, ChecksOn:\n\t\tsentences = append(sentences, notice)\n\t}\n",
  "\tif notice != \"\" {\n\t\tsentences = append(sentences, notice)\n\t}\n",
  "TestLineCoverageUnverified")
m("RV-1 item 5 the notice is not printed", RC,
  "\t\tsentences = append(sentences, notice)\n", "",
  "TestRV1_5a_|TestRV1_5c_|TestLineCoverageUnverified")
m("RV-1 item 5 coverage absent says no notice", "internal/verify/recording.go",
  "return slices.Contains(got, notes[noteUnverified]) || slices.Contains(got, notes[noteAbsent])",
  "return slices.Contains(got, notes[noteUnverified])",
  "TestRV1_5c_")
m("RV-1 item 5 coverage unverified says no notice", "internal/verify/recording.go",
  "return slices.Contains(got, notes[noteUnverified]) || slices.Contains(got, notes[noteAbsent])",
  "return slices.Contains(got, notes[noteAbsent])",
  "TestRV1_5a_|TestRV1_5b_")
m("RV-1 item 5 the watch refusal leaves no clause", "cmd/rashomon/consent.go",
  "\tif verb != \"watch\" {\n", "\tif true {\n",
  "TestRV1_5d_")
m("RV-1 item 5 every refusal carries the clause", "cmd/rashomon/consent.go",
  "\tif verb != \"watch\" {\n", "\tif false {\n",
  "TestRV1_5d_")
m("RV-1 item 5 the watch refusal drops what it leaves", "cmd/rashomon/watch.go",
  "verb, \"written\"+refusalLeaves(verb))\n",
  "verb, \"written\")\n",
  "TestRV1_5d_")
m("RV-1 item 5 the store takes no coverage marker", "internal/store/notice.go",
  "\tcase FileNoticeChannel, FileNoticeHoldout, FileNoticeCoverage, FileNoticeCoverageClear:\n",
  "\tcase FileNoticeChannel, FileNoticeHoldout:\n",
  "TestRV1_5a_|TestTRU23_EveryWrittenPathIsInTheInventory")
m("RV-1 item 5 a taken marker reads as untaken", "internal/store/notice.go",
  "\t_, err := os.Lstat(filepath.Join(s.RunDir(sessionID), name))\n\treturn err == nil\n",
  "\t_, err := os.Lstat(filepath.Join(s.RunDir(sessionID), name))\n\treturn err != nil && false\n",
  "TestRV1_5a_|TestRV1_5c_")
m("RV-1 item 5 purge leaves the coverage markers", "internal/purge/purge.go",
  "store.FileNoticeHoldout, store.FileNoticeCoverage, store.FileNoticeCoverageClear, fold.FileName,",
  "store.FileNoticeHoldout, fold.FileName,",
  "TestTRU28_PurgeListsEveryNameItsWritersExport")
