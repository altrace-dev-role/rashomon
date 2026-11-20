# RH-8: the report leads with the answer (report.md :225 to :231). The story
# opens with the last Stop's strict claims not green, the notes by pattern,
# when checks were off and how far to trust the record. The claims are the
# checkpoint's strict claims not green: a gate state other than green
# (recap.ClaimUnbacked, the done flag's claim term), or no_gate_known, which
# the flag reads as backed (D79). No record keeps the flag's decision, so the
# line and its key never say flagged. Tool-use ids and long subagent lists stay behind
# --ids and --json. Each row is killed by the TestRH8 test its last argument
# names, whose doc comment states the break.

R = "internal/recap/flagged.go"
I = "cmd/rashomon/report_impact.go"
C = "cmd/rashomon/report.go"
T = "internal/report/text.go"
D = "internal/report/text_destinations.go"
A = "internal/report/text_autosteer.go"
J = "internal/report/autosteer.go"

m("RH-8 ClaimUnbacked flags a loose claim", R,
  "\treturn c.Strict && verify.Rank(verify.State(c.Verdict)) >= 0 && c.Verdict != verify.Verdict(verify.Green)\n",
  "\treturn verify.Rank(verify.State(c.Verdict)) >= 0 && c.Verdict != verify.Verdict(verify.Green)\n", "TestRH8j_")
m("RH-8 ClaimUnbacked flags a green claim", R,
  "\treturn c.Strict && verify.Rank(verify.State(c.Verdict)) >= 0 && c.Verdict != verify.Verdict(verify.Green)\n",
  "\treturn c.Strict && verify.Rank(verify.State(c.Verdict)) >= 0\n", "TestRH8j_")
m("RH-8 ClaimUnbacked flags a verdict outside the gate states", R,
  "\treturn c.Strict && verify.Rank(verify.State(c.Verdict)) >= 0 && c.Verdict != verify.Verdict(verify.Green)\n",
  "\treturn c.Strict && c.Verdict != verify.Verdict(verify.Green)\n", "TestRH8j_")

m("RH-8 an unnamed report attaches the block over two sessions", I,
  "\t\tif n != 1 || sessionID == \"\" {\n",
  "\t\tif n < 1 || sessionID == \"\" {\n", "TestRH8d_")
m("RH-8 the calls without a session id count as a session", I,
  "\t\t\tif s.SessionID != store.UnattributedSession {\n",
  "\t\t\tif true {\n", "TestRH8d_")
m("RH-8 the strict claims not green lose the window mark", I,
  "\ta.NotGreen = notGreenOf(r.Checkpoint, v.Done != nil)\n",
  "\ta.NotGreen = notGreenOf(r.Checkpoint, false)\n", "TestRH8h_")
m("RH-8 notGreenOf keeps a green claim", I,
  "\t\tif !recap.ClaimUnbacked(c) && !unchecked {\n",
  "\t\tif false && !recap.ClaimUnbacked(c) && !unchecked {\n", "TestRH8g_")
m("RH-8 notGreenOf drops a strict claim no gate checked", I,
  "\t\tunchecked := c.Strict && c.Verdict == verify.NoGateKnown\n",
  "\t\tunchecked := false && c.Strict && c.Verdict == verify.NoGateKnown\n", "TestRH8g_")
m("RH-8 notGreenOf keeps a loose claim no gate checked", I,
  "\t\tunchecked := c.Strict && c.Verdict == verify.NoGateKnown\n",
  "\t\tunchecked := c.Verdict == verify.NoGateKnown\n", "TestRH8g_")
m("RH-8 notGreenOf drops the no-gate count", I,
  "\t\t\t\tf.NoGate++\n",
  "\t\t\t\t_ = f.NoGate\n", "TestRH8g_")
m("RH-8 notGreenOf takes a green gate's last run, not its last green", I,
  "\t\t\t\tat = cp.Gates[i].LastGreenSeq\n",
  "\t\t\t\t_ = cp.Gates[i].LastGreenSeq\n", "TestRH8g_")

m("RH-8 report does not read --ids", C,
  "\t\tcase \"--ids\":\n",
  "\t\tcase \"--ids-off\":\n", "TestRH8i_")
m("RH-8 report --ids does not pass the option", C,
  "\t\tif ids {\n\t\t\topts = append(opts, report.WithIDs())\n",
  "\t\tif false && ids {\n\t\t\topts = append(opts, report.WithIDs())\n", "TestRH8i_")
m("RH-8 --ledger does not refuse --ids", C,
  "{timeline, \"--timeline\"}, {ids, \"--ids\"}, ",
  "{timeline, \"--timeline\"}, {false, \"--ids\"}, ", "TestRH8i_")

m("RH-8 the text lists the ids without execution without --ids", T,
  "\tif !cfg.ids && len(d.WithoutExecution) > 0 {\n",
  "\tif false && !cfg.ids && len(d.WithoutExecution) > 0 {\n", "TestRH8f_")
m("RH-8 the subagent list has no cap", T,
  "\t\tif i == maxListedSubagents {\n",
  "\t\tif false && i == maxListedSubagents {\n", "TestRH8e_")
m("RH-8 the text lists the rewritten calls' ids without --ids", D,
  "\tif !ids && len(d.Rewritten) > 0 {\n",
  "\tif false && !ids && len(d.Rewritten) > 0 {\n", "TestRH8f_")

m("RH-8 the strict-claims line has no cap", A,
  "\t\tif i == maxStoryGates {\n\t\t\tparts = append(parts, fmt.Sprintf(\"and %d more\", len(a.NotGreen)-maxStoryGates))\n",
  "\t\tif false && i == maxStoryGates {\n\t\t\tparts = append(parts, fmt.Sprintf(\"and %d more\", len(a.NotGreen)-maxStoryGates))\n", "TestRH8_")
m("RH-8 the strict-claims line drops the claims not kept", A,
  "\tif a.ClaimsOmitted > 0 {\n",
  "\tif false && a.ClaimsOmitted > 0 {\n", "TestRH8_")
m("RH-8 a claim drops its earlier-window mark", A,
  "\tif c.EarlierWindow {\n",
  "\tif false && c.EarlierWindow {\n", "TestRH8_")
m("RH-8 a claim drops its no-gate count", A,
  "\tif c.NoGate > 0 {\n",
  "\tif false && c.NoGate > 0 {\n", "TestRH8_")
m("RH-8 the checks-off line drops an unrecorded start", A,
  "\tif !sess.Coverage.StartRecorded {\n",
  "\tif false && !sess.Coverage.StartRecorded {\n", "TestRH8_")
m("RH-8 the checks-off line names every gap paused", A,
  "\t\tif g.Reason == store.GapPaused {\n",
  "\t\tif true {\n", "TestRH8_")
m("RH-8 the notes line prints a count for acted on", A,
  "acted on: not measured; ignored: not measured\\n\", notesByPattern(a))\n",
  "acted on: 0; ignored: not measured\\n\", notesByPattern(a))\n", "TestRH8b_")
m("RH-8 the trust line drops the skipped-record count", A,
  "countOf(sess.SkippedRecords, \"skipped record\"), claims)\n",
  "countOf(0, \"skipped record\"), claims)\n", "TestRH8c_")

m("RH-8 the strict-claims line says flagged", A,
  "\tfmt.Fprintf(b, \"    strict claims not green%s\\n\", notGreenText(a))\n",
  "\tfmt.Fprintf(b, \"    flagged claims%s\\n\", notGreenText(a))\n", "TestRH8k_")
m("RH-8 the strict-claims key says flagged", J,
  "`json:\"strict_claims_not_green\"`",
  "`json:\"flagged\"`", "TestRH8k_")
