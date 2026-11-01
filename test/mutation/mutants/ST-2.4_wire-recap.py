# Card 2.4: recap at Stop. One read of the run feeds the digest and the Stop
# view; Stop and StopFailure append the checkpoint and write the project index;
# recap.Line's sentence wins, then the done flag, then the value line under the
# summary switch; the Flush barrier, then Claim (D93), then one systemMessage.
# The kill row is the named test's. "The done flag on a failure report" is the
# loose-claim mutant: a failure word loosens the claim in verify
# (claimDisclosures), and done.go's one Strict test serves both rows of TestPG9.
# "stopView before the pending gate" (g26) is equivalent here, since the
# catch-up's view writes nothing (deviation (a)); its place is taken by the
# catch-up writing.
DN = "internal/recap/done.go"
VL = "internal/recap/value.go"
CT = "internal/verify/claimtext.go"
VS = "internal/verify/stop.go"
AR = "internal/store/appendrecord.go"
RD = "internal/store/read.go"
RS = "cmd/rashomon/recap_stop.go"
RT = "cmd/rashomon/recap_turn.go"
IN = "cmd/rashomon/intake.go"
PU = "internal/purge/purge.go"
DI = "internal/disclose/disclose.go"

# The done flag (R7).
m("ST-2.4 the done flag on a loose claim", DN,
  '\t\tif c.Strict && verify.Rank(', '\t\tif verify.Rank(', "TestPG9_")
m("ST-2.4 the done flag on a green verdict", DN,
  ' >= 0 && c.Verdict != verify.Verdict(verify.Green) {\n', ' >= 0 {\n', "TestPG9_")

# The value line and the clause (R6, R8).
m("ST-2.4 and <k> more at four", VL, 'const maxGateNames = 3\n', 'const maxGateNames = 4\n', "TestVER65_ValueLineRowsRender")
IMPORTS["ST-2.4 a prefix word names the gate"] = (VL, '\t"slices"\n', '')
m("ST-2.4 a prefix word names the gate", VL,
  'if p := t.programs[g.Gate]; p != "" && !slices.Contains(prefixWords, p) {\n', 'if p := t.programs[g.Gate]; p != "" {\n', "TestVER65_ValueLineRowsRender")
m("ST-2.4 the no-write variant dropped", VL,
  '\tcase seq >= v.Turn.FirstSeq:\n\t\treturn " in this turn", false\n', '', "TestVER65_ValueLineRowsRender")
m("ST-2.4 the clause counts suppressed lines", VL,
  'if text(b, out[nkState]) != string(steer.LineStateDelivered) || text(b, out[nkPrompt]) != promptID {\n',
  'if text(b, out[nkPrompt]) != promptID {\n', "TestFUN15_")
m("ST-2.4 the clause counts another prompt's lines", VL,
  'if text(b, out[nkState]) != string(steer.LineStateDelivered) || text(b, out[nkPrompt]) != promptID {\n',
  'if text(b, out[nkState]) != string(steer.LineStateDelivered) {\n', "TestFUN15_")
m("ST-2.4 the clause counts a delivered flag", VL,
  '\t\tif model == "" {\n\t\t\tcontinue\n\t\t}\n', '', "TestFUN15_")

# The delivered strip (R8b).
m("ST-2.4 isDelivered requires the prefix again", CT,
  '\trest, _ := bytes.CutPrefix(lead, []byte("※"))\n',
  '\trest, ok := bytes.CutPrefix(lead, []byte("※"))\n\tif !ok {\n\t\treturn false\n\t}\n', "TestVER69b_")

# The Stop view (R4, R5, D80, D90).
m("ST-2.4 the incomplete-recording classification skipped", VS,
  '\tif snap.Recording != Complete {\n\t\tcounted.runs', '\tif false {\n\t\tcounted.runs', "TestVER65e_")
m("ST-2.4 a nil prompt id dereferenced", VS,
  'if d.PromptID != nil && *d.PromptID != "" && *d.PromptID == promptID', 'if *d.PromptID != "" && *d.PromptID == promptID',
  "TestVER67b_")
m("ST-2.4 Observe skipped", VS, '\t\tv.Index.Observe(c.runs, pt.SessionID, now.UnixMilli())\n', '', "TestVER68_")
m("ST-2.4 the checkpoint's claims dropped", VS, '\t\tv.Snap.Claims = v.Scan.Claims\n', '', "TestPG8b_")

# The store seam and the point (R3, R9).
m("ST-2.4 the checkpoint appended with seq nil", AR, '\t\trec.SetSeq(&seq)\n', '', "TestREC_AppendRecordSetsTheSeq")
m("ST-2.4 AppendRecord accepts an unknown kind", AR,
  '\tif kind, version := rec.Kind(); !AcceptsKind(kind, version) {\n\t\treturn ErrUnknownKind\n\t}\n', '',
  "TestREC_AppendRecordRefusesAnUnknownKind")
m("ST-2.4 LastSeq ignores terminals", RD,
  '\tfor _, t := range r.Terminals {\n\t\tif t.Seq != nil {\n\t\t\tlast = max(last, *t.Seq)\n\t\t}\n\t}\n', '',
  "TestREC_LastSeqIsTheGreatestOverTheRun")
m("ST-2.4 the point kind is not stop", RS, 'Kind: verify.PointStop, Seq: seq', 'Kind: verify.PointSubtask, Seq: seq', "TestPG8b_")
m("ST-2.4 an empty run writes a checkpoint", RS,
  '\tif seq == 0 {\n\t\treturn stopTurn{}, nil\n\t}\n', '', "TestSTOP_EmptyRunWritesNoCheckpoint")

# Writes by event (R10, R12).
m("ST-2.4 WriteIndex skipped", RS, '\t\t\t_, _ = verify.WriteIndex(path, v.Index)\n', '', "TestSTOP_IndexIsWrittenUnderTheProjectKey")
m("ST-2.4 the catch-up writes", RS,
  '\twrites := event == install.EventStop || event == install.EventStopFailure\n',
  '\twrites := event == install.EventStop || event == install.EventStopFailure || pending\n', "TestSTOP_PendingAppendsNoCheckpoint")
m("ST-2.4 StopFailure prints the value line", RS,
  'speaks: event == install.EventStop || pending}', 'speaks: true}', "TestSTOP_StopFailureWritesNoLine")

# Precedence, the switch, the barrier and the one key (R12, R2b).
m("ST-2.4 the value line printed when a recap sentence speaks", RT,
  '\tclause := ""\n\tif !wantSpeak {\n\t\tline, wantSpeak, clause = stop.line(d.SessionID, fromPlugin)\n\t}\n',
  '\tclause := ""\n\tif l, ok, c := stop.line(d.SessionID, fromPlugin); ok {\n\t\tline, wantSpeak, clause = l, true, c\n\t}\n', "TestFUN14_")
m("ST-2.4 the done flag loses to the value line", RS,
  '\tif flag, ok := recap.DoneFlag(s.view, sessionID, fromPlugin); ok {\n',
  '\tif flag, ok := recap.DoneFlag(s.view, sessionID, fromPlugin); ok && recap.ValueLine(s.view, s.impact, sessionID, fromPlugin) == "" {\n',
  "TestFUN14b_")
m("ST-2.4 the summary switch read as off", RS, '\treturn err != nil || c.SummaryOnFor(project)\n', '\treturn false\n', "TestFUN16_")
m("ST-2.4 the Flush barrier dropped before print", RT,
  '\tif err := fault.Fail(fault.PointRecapFlush); err != nil {\n\t\treturn err\n\t}\n\tif err := st.Flush(); err != nil {\n\t\treturn err\n\t}\n',
  '', "TestSTOP_FlushFailurePrintsNothing")
m("ST-2.4 Claim before the Flush barrier", RT,
  "\tif err := fault.Fail(fault.PointRecapFlush); err != nil {\n\t\treturn err\n\t}\n\tif err := st.Flush(); err != nil {\n\t\treturn err\n\t}\n"
  "\t// Claim's own error is intentionally ignored: recap.json is\n\t// bookkeeping, not evidence, and a failure to persist it must cost at\n"
  "\t// most a future duplicate or a stale health timestamp, never a\n\t// missing line and never a hook error (see Claim's doc).\n"
  "\tspeak, _ := recap.ClaimTurn(root, d.SessionID, d.PromptID, now, wantSpeak, clause, !pending && stop.unverified(d.SessionID, fromPlugin))\n",
  "\tspeak, _ := recap.ClaimTurn(root, d.SessionID, d.PromptID, now, wantSpeak, clause, !pending && stop.unverified(d.SessionID, fromPlugin))\n"
  "\tif err := fault.Fail(fault.PointRecapFlush); err != nil {\n\t\treturn err\n\t}\n\tif err := st.Flush(); err != nil {\n\t\treturn err\n\t}\n",
  "TestSTOP_FlushFailurePrintsNothing")
m("ST-2.4 a second systemMessage key", RT,
  'map[string]string{"systemMessage": line}', 'map[string]string{"systemMessage": line, "stopReason": line}', "TestR2b_")

# The payload (R11, QA B4).
m("ST-2.4 stop_hook_active read as a string", IN,
  'Max: stopHookActiveBytes, Mode: jsonscan.CaptureHead, Fold: true}',
  'Max: stopHookActiveBytes, Mode: jsonscan.CaptureHead, String: true, Fold: true}', "TestSTOP_PayloadBooleanDoesNotAbortTheRead")
m("ST-2.4 cwd not read", IN, '\t\t\tfm.CWD = string(v)\n', '\t\t\tfm.CWD = ""\n', "TestSTOP_PayloadBooleanDoesNotAbortTheRead")

# Inventories (R17).
m("ST-2.4 purge leaves the project index", PU,
  '\t\treturn level == 1 && verify.IsIndexFile(name)\n', '\t\treturn false\n', "TestTRU28_PurgeTakesTheProjectIndex")
m("ST-2.4 the index rows left out of the inventory", DI,
  '}, append(recordScopeFiles(), append(indexFiles(), append(bodiesFiles(), labFiles()...)...)...)...)...)\n', '}, append(recordScopeFiles(), append(bodiesFiles(), labFiles()...)...)...)...)\n', "TestTRU23_EveryWrittenPathIsInTheInventory")
