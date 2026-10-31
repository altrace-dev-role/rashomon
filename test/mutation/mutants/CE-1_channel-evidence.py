# CE-1: channel evidence (card CHANNEL-EVIDENCE1, D226). At Stop each delivered
# note is looked for in the transcript as the text of a hook_additional_context
# attachment and labelled seen, absent or unreadable; one absent reading on a
# version the floor trusts shadows that event for the next session through an
# O_EXCL marker, which a probe's removal or a newer Claude Code clears. Each
# break removes one rule and names the test that holds it.
E = "internal/steer/evidence.go"
S = "internal/steer/evidence_scan.go"
J = "internal/steer/evidence_json.go"
R = "internal/steer/route.go"
C = "internal/steer/client.go"
N = "internal/steer/notice.go"
ST = "internal/store/evidence.go"
RT = "cmd/rashomon/recap_turn.go"
P = "internal/purge/purge.go"
D = "internal/disclose/disclose.go"

# The label.
m("CE-1 a found note is not seen", E, "\t\tcase found[i]:\n", "\t\tcase found[i] && false:\n", "TestCE1_SeenAbsentUnreadable")
m("CE-1 a read that did not cover the note reads absent", E, "\t\tcase complete:\n", "\t\tcase complete || true:\n", "TestCE1_SeenAbsentUnreadable")
m("CE-1 a single-string attachment is not read", J, "\t\tif s := textOf(b, sp); s != \"\" {\n", "\t\tif s := textOf(b, sp); false {\n",
  "TestCE1_SeenAbsentUnreadable")
m("CE-1 an exclusion on the line is not read", E, "stringsOf(b, out[nkExcluded])", "nil", "TestCE1b_OnlyATrustedVersionShadows")
m("CE-1 any attachment counts, not only hook_additional_context", S,
  "textOf(b, att[0]) != \"hook_additional_context\"", "false", "TestCE1a_")
m("CE-1 a transcript with no record reads as read", S, "s.records == 0", "false", "TestCE1_SeenAbsentUnreadable")

# Who is judged, and once.
m("CE-1 a subagent's note is judged", E, "textOf(b, out[nkAgent]) != \"\"", "false", "TestCE1d_ASubagentsNoteIsNotJudged")
m("CE-1 a note too young to be in the transcript is judged", E, "\t\tif now.Sub(at) < evidenceSettle {\n", "\t\tif false {\n",
  "TestCE1d_ANoteIsJudgedOnce")
m("CE-1 a judged note is judged again", E, "\t\t\tjudged[textOf(b, out[nkSteerID])] = true\n", "", "TestCE1d_ANoteIsJudgedOnce")

# The policy on absent.
m("CE-1 an absent note shadows on any version", E, "if out[i].Evidence == EvidenceAbsent && n.trusted {", "if out[i].Evidence == EvidenceAbsent {",
  "TestCE1b_OnlyATrustedVersionShadows")
m("CE-1 an unreadable note shadows", E, "if out[i].Evidence == EvidenceAbsent && n.trusted {", "if out[i].Evidence != EvidenceSeen && n.trusted {",
  "TestCE1b_AnUnreadableTranscriptChangesNoState")
m("CE-1 a version below the floor is trusted", E, "compareVersions(version, floor) < 0", "false", "TestCE1b_OnlyATrustedVersionShadows")
m("CE-1 an excluded version is trusted", E, "return !channelBasis{floor: floor, excluded: excluded}.excludes(version)", "return true",
  "TestCE1b_OnlyATrustedVersionShadows")
m("CE-1 the marker holds no version", E, "st.TakeChannelEvidence(n.event, n.version)", "st.TakeChannelEvidence(n.event, \"\")",
  "TestCE1b_AbsentShadowsNextSessionOnce")

# The shadow, and the way back.
m("CE-1 the route never reads the marker", R, "\t\tc.EvidenceAbsent = evidenceShadow(ctx, c, table)\n", "", "TestCE1b_AbsentShadowsNextSessionOnce")
m("CE-1 a marker shadows nothing in shutBy", C, "\tcase c.EvidenceAbsent:\n\t\treturn shutEvidence\n", "", "TestCE1b_AbsentShadowsNextSessionOnce")
m("CE-1 the notice gives no reason", N, "did not show a delivered note for this event", "did not show a note", "TestCE1b_AbsentShadowsNextSessionOnce")
m("CE-1 a newer version does not clear", E, "compareVersions(c.Version, marked) > 0", "compareVersions(c.Version, marked) < 0",
  "TestCE1c_ProbePassAndNewerVersionClear")
m("CE-1 a newer version is let through and the marker stays", E, "\t\tctx.Store.ClearChannelEvidence(event)\n", "",
  "TestCE1c_ProbePassAndNewerVersionClear")
m("CE-1 the same version clears", E, "compareVersions(c.Version, marked) > 0", "compareVersions(c.Version, marked) >= 0",
  "TestCE1c_ProbePassAndNewerVersionClear")
m("CE-1 a second Stop overwrites the first marker", ST, "os.O_CREATE|os.O_EXCL|os.O_WRONLY", "os.O_CREATE|os.O_WRONLY|os.O_TRUNC",
  "TestCE1d_MarkerIsCreatedOnceAndKeepsItsVersion")
m("CE-1 clearing removes nothing", ST, "\t\t_ = os.Remove(s.evidencePath(event))\n", "", "TestCE1d_MarkerIsCreatedOnceAndKeepsItsVersion")
m("CE-1 any event name reaches a path", ST, "(c < 'a' || c > 'z') && (c < 'A' || c > 'Z')", "c == 0", "TestCE1d_OnlyAnEventNameReachesAPath")

# The bound.
m("CE-1 the whole file is read, whatever its size", S, "\tif size > lim.MaxBytes {\n", "\tif false {\n", "TestCE1e_TheReadIsBounded")
m("CE-1 a window that begins after the note reads as covering it", S,
  "return found, off == 0 || (!s.first.IsZero() && !s.first.After(oldest))", "return found, true", "TestCE1e_TheReadIsBounded")
m("CE-1 a window's first record is not timed", S, "windowed && s.first.IsZero() && s.tried < stampLines", "false", "TestCE1e_TheReadIsBounded")
m("CE-1 the deadline is not checked", S, "\t\tif !lim.Deadline.IsZero() && time.Now().After(lim.Deadline) {\n", "\t\tif false {\n",
  "TestCE1e_TheReadIsBounded")
m("CE-1 the Stop's byte bound is lifted", S, "MaxBytes: 16 << 20", "MaxBytes: 1 << 40", "TestCE1e_DefaultLimitsAreTheStopsBound")
m("CE-1 the Stop's line bound is lifted", S, "MaxLine: 1 << 20", "MaxLine: 1 << 30", "TestCE1e_DefaultLimitsAreTheStopsBound")
m("CE-1 the Stop has no deadline", S, "Deadline: now.Add(time.Second)", "Deadline: now.Add(time.Hour)", "TestCE1e_DefaultLimitsAreTheStopsBound")

# The Stop.
m("CE-1 the Stop never reads the transcript", RT, "\t\tdefer checkEvidence(st, payload.SessionID, payload.Harness.TranscriptPath)\n", "",
  "TestH118_AbsentNoteShadowsItsEventNextSession")
m("CE-1 a StopFailure reads the transcript", RT, "if !pending && payload.Harness.HookEventName == install.EventStop {", "if !pending {",
  "TestH118_OnlyAStopReadsTheTranscript")

# Disclosure.
m("CE-1 purge leaves the marker", P, "\tif store.IsChannelEvidenceMarker(name) {\n\t\treturn true\n\t}\n", "",
  "TestCE1d_PurgeRemovesTheChannelEvidenceMarker")
m("CE-1 the inventory omits the marker", D, "slices.Concat(defaultFiles(), learnFiles(), specFiles(), evidenceFiles())",
  "slices.Concat(defaultFiles(), learnFiles(), specFiles())", "TestTRU23_EveryWrittenPathIsInTheInventory")
