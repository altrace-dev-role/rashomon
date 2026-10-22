# H-104: resume's closing gap used to run from now to now whenever it could
# not read or parse the marker, because store.Resume's only fallback is now.
# cmdResume now recovers the real instant from pause's own gap record when one
# is there to recover it from.
m("H-104 cmdResume stops reading the ledger for the true pause start", "cmd/rashomon/main.go",
  "\t\tif gaps, _, err := st.ReadGaps(); err == nil {\n\t\t\tsince = earlierPauseStart(gaps, since)\n\t\t}\n",
  "",
  "TestCmdResume_RecoversTrueStartFromTheGapLedgerWhenTheMarkerCannotBeRead")
m("H-104 a gap with a different reason is read as pause's own marker", "cmd/rashomon/main.go",
  "\t\tif g.Reason != store.GapPaused {\n\t\t\tcontinue\n\t\t}\n",
  "",
  "TestEarlierPauseStart_SkipsPastAGapWithADifferentReason")
m("H-104 a closed prior episode is read as an open marker", "cmd/rashomon/main.go",
  "\t\tif g.FromUnixMS != g.ToUnixMS {\n\t\t\treturn since\n\t\t}\n",
  "",
  "TestEarlierPauseStart_IgnoresAClosedPriorEpisode")
m("H-104 a marker no earlier than since is used anyway", "cmd/rashomon/main.go",
  "\t\tif g.FromUnixMS < since.UnixMilli() {\n\t\t\treturn time.UnixMilli(g.FromUnixMS)\n\t\t}\n",
  "\t\treturn time.UnixMilli(g.FromUnixMS)\n",
  "TestEarlierPauseStart_KeepsSinceWhenTheLedgerIsNotEarlier")
