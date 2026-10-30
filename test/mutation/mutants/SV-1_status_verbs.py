# SV-1: status and lessons (STATUS-VERBS1a). status counts the sessions each
# installed event fired in from the entries' fired markers and names one that
# stopped firing; it says the harness stand-downs, the last Stop's recording
# with its notes and the consent agreed, with closed --json keys; lessons lists
# by tier. Each break below is one of those promises undone.
m("SV-1 status --json drops not_firing", "cmd/rashomon/verbs_status.go",
  "\tNotFiring   bool   `json:\"not_firing\"`\n", "\tNotFiring   bool   `json:\"-\"`\n",
  "TestSV1f_EveryVerbHasJSONWithClosedKeys")
m("SV-1 lessons --json drops since_unix_ms", "cmd/rashomon/verbs_lessons.go",
  "\tSinceUnixMS int64      `json:\"since_unix_ms\"`\n", "\tSinceUnixMS int64      `json:\"-\"`\n",
  "TestSV1f_EveryVerbHasJSONWithClosedKeys")
m("SV-1 lessons is left out of the verb table", "cmd/rashomon/verbs.go",
  "\tcase \"lessons\":\n\t\treturn cmdLessons\n", "", "TestSV1f_EveryVerbHasJSONWithClosedKeys")
m("SV-1 an event that never fired is not named", "cmd/rashomon/verbs_status.go",
  "\t\tev.NotFiring = onlyWhen == \"\" && ev.Of > 0 && ev.Sessions == 0 && !ev.ThisSession\n", "",
  "TestSV1_StatusNamesAnEventThatNeverFired")
m("SV-1 Stop and SessionEnd are judged with the newest session in", "cmd/rashomon/verbs_status.go",
  "\t\tif earlierOnly {\n\t\t\tev.Of--\n\t\t}\n\t\tfor _, r := range window {\n\t\t\tif (!earlierOnly || r.name != newest.name) && slices.Contains(r.Fired, e) {\n",
  "\t\tfor _, r := range window {\n\t\t\tif slices.Contains(r.Fired, e) {\n",
  "TestSV1_StatusNamesAnEventThatNeverFired|TestSV1b_StatusCountsPerEvent")
m("SV-1 the 7-day window is ignored", "cmd/rashomon/verbs_status.go",
  "\t\tif now.Sub(a.LastAt) <= statusWindow {\n", "\t\tif true {\n", "TestSV1b_StatusCountsPerEvent")
m("SV-1 the harness block says none always", "cmd/rashomon/verbs_status.go",
  "\t\tif t.StoodDown > 0 {\n", "\t\tif false {\n", "TestSV1l_StatusSaysHarnessStandDowns")
m("SV-1 RecordingAt drops the notes", "internal/verify/recording.go",
  "func RecordingAt(run *store.Run, at int64) (Recording, []string) { return recording(run, at) }\n",
  "func RecordingAt(run *store.Run, at int64) (Recording, []string) { r, _ := recording(run, at); return r, nil }\n",
  "TestSV1m_RecordingAtIsRecording")
m("SV-1 a consent to an older steer disclosure reads as current", "cmd/rashomon/verbs_status.go",
  " && rec.SteerDisclosure >= steer.DisclosureVersion,\n", ",\n", "TestSV1n_StatusSaysTheConsentAgreed")
m("SV-1 lessons does not say the quarantine", "cmd/rashomon/verbs_lessons.go",
  "\tb.WriteString(quarantineLine(quarantined))\n", "", "TestSV1d_LessonsByTier")
m("SV-1 MarkFired rewrites the marker on every fire", "internal/store/fired.go",
  "\tif _, err := os.Lstat(marker); err == nil {\n\t\treturn nil\n\t}\n\tif err := os.MkdirAll(dir, dirMode); err != nil {\n\t\treturn err\n\t}\n\tf, err := os.OpenFile(marker, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)\n",
  "\tif err := os.MkdirAll(dir, dirMode); err != nil {\n\t\treturn err\n\t}\n\tf, err := os.OpenFile(marker, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fileMode)\n",
  "TestSV1h_MarkFiredWritesOnceAndRefusesOtherNames")
m("SV-1 IsFiredFile matches the prefix alone", "internal/store/fired.go",
  "\treturn ok && slices.Contains(firedEvents[:], event)\n", "\treturn ok\n",
  "TestSV1h_IsFiredFileNamesTheEightMarkersOnly")
m("SV-1 purge leaves the fired markers", "internal/purge/purge.go",
  "\tif slices.Contains(runFiles, name) || store.IsFiredFile(name) {\n", "\tif slices.Contains(runFiles, name) {\n",
  "TestSV1k_PurgeTakesTheFiredMarkers|TestTRU28_PurgeListsEveryNameItsWritersExport")
m("SV-1 the hook entry marks nothing", "cmd/rashomon/main.go",
  "\t\tmarkFired(st, sessionOf(payload), \"PreToolUse\")\n", "", "TestSV1i_EachEntryMarksItsEvent")
m("SV-1 post marks PostToolUse for both events", "cmd/rashomon/fired.go",
  "name == \"PostToolUseFailure\" {", "false {", "TestSV1i_EachEntryMarksItsEvent")
m("SV-1 recap marks while paused", "cmd/rashomon/recap_turn.go",
  "\tif paused, _, perr := store.Paused(root); perr == nil && !paused {\n",
  "\tif _, _, perr := store.Paused(root); perr == nil {\n", "TestSV1i_EachEntryMarksItsEvent")
