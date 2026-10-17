m("TL the timeline is not put in seq order", "internal/report/timeline.go",
  "\tsort.SliceStable(decls, func(i, j int) bool { return decls[i].Seq < decls[j].Seq })",
  "\tsort.SliceStable(decls, func(i, j int) bool { return false })", "TestTimeline_")
m("TL a subagent's call is not attributed to its agent", "internal/report/timeline.go",
  "\t\t\tAgent:        timelineAgent(d),", "\t\t\tAgent:        nil,", "TestTimeline_")
m("TL a denied call reads as failed", "internal/report/timeline.go",
  "\tcase LinkOutcomeDenied:\n\t\treturn GroupNeverRan", "\tcase LinkOutcomeDenied:\n\t\treturn GroupFailed", "TestTimeline_")
m("TL a call with no execution record reads as never ran", "internal/report/timeline.go",
  "\tdefault:\n\t\treturn GroupUnknown\n\t}\n}", "\tdefault:\n\t\treturn GroupNeverRan\n\t}\n}", "TestTimeline_")
m("TL an earlier success counts as a later one", "internal/report/timeline.go",
  "\t\tif *e.pos <= *failed.pos {", "\t\tif false {", "TestTimeline_")
m("TL the same digest under another tool is the same call", "internal/report/timeline.go",
  "\t\tif c.ToolName != failed.call.ToolName {\n\t\t\tcontinue\n\t\t}\n\t\t// A failed call's ok record", "\t\t// A failed call's ok record", "TestTimeline_")
m("TL the same-command tier is skipped", "internal/report/timeline.go",
  "\t\tcommand := failed.digest != \"\" && e.digest == failed.digest", "\t\tcommand := false", "TestTimeline_")
m("TL a call that did not fail is followed up", "internal/report/timeline.go",
  "\t\tif e.call.Group != GroupFailed {", "\t\tif e.call.Group == GroupOK {", "TestTimeline_")
m("TL a failure with no later success says nothing", "internal/report/timeline_text.go",
  "\t\treturn \"  → no later success of the same command or program recorded\"", "\t\treturn \"\"", "TestTimeline_")
m("TL the capped listing drops the rest silently", "internal/report/timeline_text.go",
  "\t\t\tfmt.Fprintf(b, \"    %d more call%s, see --json\\n\", rest, plural(rest))\n",
  "\t\t\t_ = rest\n", "TestTimeline_")
m("TL --timeline never reaches the renderer", "cmd/rashomon/main.go",
  "\t\t\topts = append(opts, report.WithTimeline())", "\t\t\t_ = report.WithTimeline", "TestTimeline_")

# #36 review fixes: the timeline says what the record supports and no more.
m("TL an execution with no declaration and no terminal is left off", "internal/report/timeline.go",
  "\tfor _, x := range run.Executions {\n\t\tadd(x.ToolUseID)\n\t}\n", "", "TestTimeline_")
m("TL a dropped call's execution is thrown away", "internal/report/timeline.go",
  "\t\tc.Outcome = timelineOutcome(id, rec, executed, denied)", "\t\tc.Outcome = LinkUnknown", "TestTimeline_")
m("TL an undeclared call loses its exit code", "internal/report/timeline.go",
  "\t\tc.ExitCode = outcomeExitCode(rec)\n\t\tentries = append(entries, timelineEntry{call: c, digest: effectiveDigest(\"\", rec),",
  "\t\tentries = append(entries, timelineEntry{call: c, digest: effectiveDigest(\"\", rec),", "TestTimeline_")
m("TL an undeclared call loses the tool name its execution carries", "internal/report/timeline.go",
  "\t\t\tc.ToolName = recs[len(recs)-1].ToolName", "\t\t\t_ = recs", "TestTimeline_")
m("TL an undeclared call is attributed to the main agent", "internal/report/timeline.go",
  "\t\t\tAgentUnknown: true,", "\t\t\tAgentUnknown: false,", "TestTimeline_")
m("TL an undeclared row renders as the main agent", "internal/report/timeline_text.go",
  "\t\tagent = LinkUnknown", "\t\tagent = \"main\"", "TestTimeline_")
m("TL an undeclared failure is compared with nothing to compare", "internal/report/timeline.go",
  "\t\tif e.call.Seq == nil || e.pos == nil {", "\t\tif e.pos == nil {", "TestTimeline_")
m("TL a failure with no position is compared", "internal/report/timeline.go",
  "\t\tif e.call.Seq == nil || e.pos == nil {", "\t\tif e.call.Seq == nil {", "TestTimeline_")
m("TL later is judged by declaration order", "internal/report/timeline.go",
  "\t\tif *e.pos <= *failed.pos {", "\t\tif *c.Seq <= *failed.call.Seq {", "TestTimeline_")
m("TL the first success in row order wins over the first recorded", "internal/report/timeline.go",
  "\t\t\tif sameCommand == nil || *e.pos < *sameCommand.pos {", "\t\t\tif sameCommand == nil {", "TestTimeline_")
m("TL an unplaced success reads as no later success", "internal/report/timeline.go",
  "\t\tif e.pos == nil {\n\t\t\tunplaced = true\n\t\t\tcontinue", "\t\tif e.pos == nil {\n\t\t\tcontinue", "TestTimeline_")
m("TL an unplaced success outranks a placed one", "internal/report/timeline.go",
  "\tswitch {\n\tcase sameCommand != nil:", "\tswitch {\n\tcase unplaced:\n\t\treturn nil, false\n\tcase sameCommand != nil:", "TestTimeline_")
m("TL an unplaced same-program success is not a candidate", "internal/report/timeline.go",
  "\t\tif e.pos == nil {\n\t\t\tunplaced = true", "\t\tif e.pos == nil {\n\t\t\tunplaced = command", "TestTimeline_")
m("TL a not-checked failure counts as no later success", "internal/report/timeline.go",
  "\t\t\tcase !c.LaterChecked:\n\t\t\t\tn.NotChecked++\n", "", "TestTimeline_")
m("TL a not-checked failure says no later success", "internal/report/timeline_text.go",
  "\t\treturn \"  → not checked for a later success\"", "\t\treturn \"  → no later success of the same command or program recorded\"", "TestTimeline_")
m("TL git status follows up a failed git push", "internal/report/timeline.go",
  "\treturn program != \"\" && !subcommandProgram(program)\n", "\treturn program != \"\"\n", "TestTimeline_")
m("TL the same-program label claims different arguments", "internal/report/timeline_text.go",
  "\"  → same program ok at %d, recorded after%s\"", "\"  → same program ok at %d (different arguments%s)\"", "TestTimeline_")
m("TL the same-command label drops recorded after", "internal/report/timeline_text.go",
  "\"  → same command ok at %d, recorded after%s\"", "\"  → same command ok at %d%s\"", "TestTimeline_")
m("TL the counts line says no later success without recorded", "internal/report/timeline_text.go",
  "%d no later success recorded%s)", "%d no later success%s)", "TestTimeline_")
m("TL agent_type reaches the terminal raw", "internal/report/timeline_text.go",
  "\ttyp := printable(a.Type)", "\ttyp := a.Type", "TestTimeline_")
m("TL agent_id reaches the terminal raw", "internal/report/timeline_text.go",
  "[]rune(strings.TrimPrefix(printable(a.ID), \"agent-\"))", "[]rune(strings.TrimPrefix(a.ID, \"agent-\"))", "TestTimeline_")
m("TL the agent id is cut by byte", "internal/report/timeline_text.go",
  "\tid := []rune(strings.TrimPrefix(", "\tid := []byte(strings.TrimPrefix(", "TestTimeline_")
# Re-anchored after the masked-status refactor: the legend gained a clause for a
# masked call; dropping everything after "no execution record" is the same cut.
m("TL the unknown legend names only no execution record", "internal/report/timeline_text.go",
  "; or outcome unobserved: it ran and how it ended was not recorded, or it was moved to the background before it ended; or ok as a line that did not return a build or test run's exit status)", ")", "TestTimeline_")
m("TL undeclared calls go unaccounted in the legend", "internal/report/timeline_text.go",
  "\tif n.AgentUnknown > 0 {\n\t\tfmt.Fprintf(b, \"    %d call%s with no declaration", "\tif false {\n\t\tfmt.Fprintf(b, \"    %d call%s with no declaration", "TestTimeline_")
m("TL an undeclared row does not say it has no declaration", "internal/report/timeline_text.go",
  "\t\tr += \", no declaration recorded\"", "\t\tr += \"\"", "TestTimeline_")
m("TL the date is never shown", "internal/report/timeline_text.go",
  "\t\t\t\tfmt.Fprintf(b, \"    %s (UTC)\\n\", day)", "\t\t\t\t_ = day", "TestTimeline_")
m("TL a subagent's call counts as the main agent's", "internal/report/timeline.go",
  "\t\tcase c.Agent == nil:\n\t\t\tn.MainAgent++", "\t\tdefault:\n\t\t\tn.MainAgent++", "TestTimeline_")
m("TL the header names the main agent without its count", "internal/report/timeline_text.go",
  "\twho := fmt.Sprintf(\"%d main agent\", n.MainAgent)", "\twho := \"main agent\"", "TestTimeline_")
m("TL the digest reaches the JSON", "internal/report/timeline.go",
  "\t\t\tok:        failedCallOK(c, d.Shape.Digest, executed[d.ToolUseID]),\n\t\t})\n",
  "\t\t\tok:        failedCallOK(c, d.Shape.Digest, executed[d.ToolUseID]),\n\t\t})\n\t\tentries[len(entries)-1].call.VerbClass = d.Shape.Digest\n",
  "TestTimeline_JSONCarriesNoDigest")

# #36 review round 3.
m("TL a rewritten success counts as the same command", "internal/report/timeline.go",
  "\t\treturn rec.ExecutedDigest\n", "\t\treturn declared\n", "TestTimeline_")
m("TL an undeclared later success reads as no later success", "internal/report/timeline.go",
  "\t\t\tif pos == nil || *pos > *failed.pos {\n\t\t\t\tunplaced = true", "\t\t\tif false && (pos == nil || *pos > *failed.pos) {\n\t\t\t\tunplaced = true", "TestTimeline_")
m("TL an undeclared earlier success leaves the failure unchecked", "internal/report/timeline.go",
  "\t\t\tif pos == nil || *pos > *failed.pos {\n\t\t\t\tunplaced = true", "\t\t\tif true || pos == nil || *pos > *failed.pos {\n\t\t\t\tunplaced = true", "TestTimeline_")
m("TL the never-ran group is spelled unlike every other enum", "internal/report/timeline.go",
  "\tGroupNeverRan    = \"never_ran\"", "\tGroupNeverRan    = \"never ran\"", "TestTimeline_")
m("TL the report builds the timeline without the denials", "internal/report/assemble.go",
  "\tsess.Timeline = timelineFrom(run, executed, denied, tb)", "\tsess.Timeline = timelineFrom(run, executed, nil, tb)", "TestTimeline_")
m("TL the never-ran count is not printed", "internal/report/timeline_text.go",
  "\tfmt.Fprintf(b, \"    never ran    %d  (denied before running)\\n\", n.NeverRan)\n", "", "TestTimeline_")
m("TL the marker claims no later success of any kind", "internal/report/timeline_text.go",
  "\t\treturn \"  → no later success of the same command or program recorded\"", "\t\treturn \"  → no later success recorded\"", "TestTimeline_")
m("TL the counts line claims a same-program check the row does not", "internal/report/timeline_text.go",
  "%d no later success recorded%s)", "%d no later success of the same command or program recorded%s)", "TestTimeline_")
m("TL the legend does not say another fix goes undetected", "internal/report/timeline_text.go",
  "\tif n.Failed > 0 {\n\t\tfmt.Fprintln(b, \"                 (only a later success", "\tif false {\n\t\tfmt.Fprintln(b, \"                 (only a later success", "TestTimeline_")
m("TL undeclared rows are ordered by id, not by their results", "internal/report/timeline.go",
  "\t\tif pi != pj {\n\t\t\treturn pi < pj\n", "\t\tif false && pi != pj {\n\t\t\treturn pi < pj\n", "TestTimeline_")
m("TL undeclared rows with no position come first", "internal/report/timeline.go",
  "\t\tif oki != okj {\n\t\t\treturn oki\n", "\t\tif oki != okj {\n\t\t\treturn okj\n", "TestTimeline_")
m("TL tool_name reaches the terminal raw on the timeline", "internal/report/timeline_text.go",
  "\tcall := printable(c.ToolName)", "\tcall := c.ToolName", "TestTimeline_")
m("TL tool_name reaches the terminal raw in the by-tool counts", "internal/report/text.go",
  "fmt.Sprintf(\"%s %d\", printable(name), counts[name])", "fmt.Sprintf(\"%s %d\", name, counts[name])", "TestText_ToolNameIsPrintable")
m("TL tool_name reaches the terminal raw in a chain row", "internal/report/text.go",
  "\t\tl.Seq, printable(l.ToolName), shape,", "\t\tl.Seq, l.ToolName, shape,", "TestText_ToolNameIsPrintable")
m("TL the capped listing prints every row anyway", "internal/report/timeline_text.go",
  "plural(rest))\n\t\t\tbreak\n", "plural(rest))\n", "TestTimeline_")
m("TL the exit code is read from the highest-seq record, not the outcome's", "internal/report/timeline.go",
  "\t\tc.ExitCode = outcomeExitCode(rec)\n\t\tentries = append(entries, timelineEntry{\n",
  "\t\tc.ExitCode = outcomeExitCode(outcomeRecord(executed[d.ToolUseID][max(len(executed[d.ToolUseID])-1, 0):]))\n\t\tentries = append(entries, timelineEntry{\n", "TestTimeline_")
m("TL the position is read from the highest-seq record, not the outcome's", "internal/report/timeline.go",
  "\t\t\tpos:       outcomeSeq(rec),\n\t\t\trewritten:",
  "\t\t\tpos:       outcomeSeq(outcomeRecord(executed[d.ToolUseID][max(len(executed[d.ToolUseID])-1, 0):])),\n\t\t\trewritten:", "TestTimeline_")
m("TL a failed record behind a later ok one is not the outcome record", "internal/report/timeline.go",
  "\t\tif recs[i].Outcome == store.ExecFailed {\n\t\t\treturn &recs[i]", "\t\tif false {\n\t\t\treturn &recs[i]", "TestTimeline_")
m("TL a failure behind a later ok record reads ok", "internal/report/timeline.go",
  "\tif rec != nil && rec.Outcome == store.ExecFailed {\n\t\treturn store.ExecFailed\n\t}\n", "", "TestTimeline_")
m("TL the first failed record, not the last, is the outcome record", "internal/report/timeline.go",
  "\tfor i := len(recs) - 1; i >= 0; i-- {\n\t\tif recs[i].Outcome == store.ExecFailed {", "\tfor i := 0; i < len(recs); i++ {\n\t\tif recs[i].Outcome == store.ExecFailed {", "TestTimeline_")
m("TL the header does not say --json carries the tool_use_id", "internal/report/timeline_text.go",
  "\tfmt.Fprintln(b, \"    rows omit tool_use_id; --json carries it for every call\")\n", "", "TestTimeline_")
m("TL the follow-up arrow does not name the agent", "internal/report/timeline_text.go",
  "\t\twho = \", \" + agentLabel(l.Agent)", "\t\t_ = l.Agent", "TestTimeline_")
m("TL the follow-up's agent is left out of the JSON", "internal/report/timeline.go",
  "\t\treturn &LaterSuccess{Kind: LaterSameCommand, Seq: *sameCommand.call.Seq, Agent: sameCommand.call.Agent}, true",
  "\t\treturn &LaterSuccess{Kind: LaterSameCommand, Seq: *sameCommand.call.Seq}, true", "TestTimeline_JSONMatchesTheText")
m("TL the interrupted count is not printed", "internal/report/timeline_text.go",
  "\tif n.Interrupted > 0 {", "\tif false {", "TestTimeline_")
m("TL the interleave caveat is not printed", "internal/report/timeline_text.go",
  "\tif n.Subagents > 0 {\n\t\tfmt.Fprintln(b, \"    calls from agents running at once", "\tif false {\n\t\tfmt.Fprintln(b, \"    calls from agents running at once", "TestTimeline_")
m("TL command is dropped from the wrapper list", "internal/report/timeline.go",
  "\t\"exec\": true, \"command\": true,\n", "\t\"exec\": true,\n", "TestTimeline_")
m("TL python.exe is the program", "internal/report/timeline.go",
  "\tbase := strings.TrimSuffix(p, \".exe\")", "\tbase := p", "TestTimeline_")
m("TL pythonw is the program", "internal/report/timeline.go",
  "\treturn subcommandPrograms[base] || subcommandPrograms[strings.TrimSuffix(base, \"w\")]", "\treturn subcommandPrograms[base]", "TestTimeline_")
m("TL any name ending in w is its base", "internal/report/timeline.go",
  "\treturn subcommandPrograms[base] || subcommandPrograms[strings.TrimSuffix(base, \"w\")]", "\treturn subcommandPrograms[base] || subcommandPrograms[strings.TrimRight(base, \"w\")] || strings.HasSuffix(base, \"w\")", "TestTimeline_")
m("TL the effective digest is read from the highest-seq record, not the outcome's", "internal/report/timeline.go",
  "\t\t\tdigest:    effectiveDigest(d.Shape.Digest, rec),\n", "\t\t\tdigest:    effectiveDigest(d.Shape.Digest, outcomeRecord(executed[d.ToolUseID][max(len(executed[d.ToolUseID])-1, 0):])),\n", "TestTimeline_")
m("TL an undeclared call's exit code is read from its first record", "internal/report/timeline.go",
  "\t\tc.ExitCode = outcomeExitCode(rec)\n\t\tentries = append(entries, timelineEntry{call: c, digest: effectiveDigest(\"\", rec),",
  "\t\tc.ExitCode = outcomeExitCode(outcomeRecord(recs[:min(len(recs), 1)]))\n\t\tentries = append(entries, timelineEntry{call: c, digest: effectiveDigest(\"\", rec),", "TestTimeline_")
m("TL the same-program follow-up's agent is left out", "internal/report/timeline.go",
  "\t\treturn &LaterSuccess{Kind: LaterSameProgram, Seq: *sameProgram.call.Seq, Agent: sameProgram.call.Agent}, true",
  "\t\treturn &LaterSuccess{Kind: LaterSameProgram, Seq: *sameProgram.call.Seq}, true", "TestTimeline_")
m("TL the follow-up legend is printed under failed 0", "internal/report/timeline_text.go",
  "\tif n.Failed > 0 {\n\t\tfmt.Fprintln(b, \"                 (only a later success", "\tif true {\n\t\tfmt.Fprintln(b, \"                 (only a later success", "TestTimeline_")
m("TL the interrupted count is printed when there is none", "internal/report/timeline_text.go",
  "\tif n.Interrupted > 0 {", "\tif true {", "TestTimeline_")
m("TL the interleave caveat is printed with no subagent", "internal/report/timeline_text.go",
  "\tif n.Subagents > 0 {\n\t\tfmt.Fprintln(b, \"    calls from agents running at once", "\tif true {\n\t\tfmt.Fprintln(b, \"    calls from agents running at once", "TestTimeline_")
m("TL sudo ls follows up a failed sudo systemctl", "internal/report/timeline.go",
  "\t\"sudo\": true, \"doas\": true, \"su\": true, \"runuser\": true, \"chroot\": true, \"ssh\": true,\n", "", "TestTimeline_")
m("TL a versioned interpreter is the program", "internal/report/timeline.go",
  "\tif i := strings.IndexAny(base, \"0123456789\"); i >= 0 {\n\t\tbase = base[:i]\n\t}\n", "", "TestTimeline_")
m("TL any name with a digit is a versioned interpreter", "internal/report/timeline.go",
  "\treturn subcommandPrograms[base] || subcommandPrograms[strings.TrimSuffix(base, \"w\")]", "\treturn base != p", "TestTimeline_")

# #36 review round 2.
m("TL a rewritten success counts as the same program", "internal/report/timeline.go",
  "\t\tif program && (e.rewritten || failed.rewritten) {", "\t\tif false {", "TestTimeline_")
m("TL a failed call's ok record is no success", "internal/report/timeline.go",
  "\t\tif c.Group == GroupFailed {\n\t\t\tif ok := e.ok;", "\t\tif false {\n\t\t\tif ok := e.ok;", "TestTimeline_")
m("TL a failed call's ok record before the failure leaves it unchecked", "internal/report/timeline.go",
  "ok != nil && (ok.pos == nil || *ok.pos > *failed.pos) {", "ok != nil {", "TestTimeline_")
m("TL an undeclared success's executed digest is thrown away", "internal/report/timeline.go",
  "timelineEntry{call: c, digest: effectiveDigest(\"\", rec), pos:", "timelineEntry{call: c, pos:", "TestTimeline_")
m("TL an undeclared success with no tool name is ruled out", "internal/report/timeline.go",
  "\t\t\tif tool != failed.call.ToolName && tool != LinkUnknown {", "\t\t\tif tool != failed.call.ToolName {", "TestTimeline_")
m("TL an undeclared success of another digest is ruled out under a program tier", "internal/report/timeline.go",
  "digest != \"\" && digest != failed.digest && !programTier {", "digest != \"\" && digest != failed.digest {", "TestTimeline_")
m("TL an undeclared success of an unknown digest is ruled out", "internal/report/timeline.go",
  "digest != \"\" && digest != failed.digest && !programTier {", "digest != failed.digest && !programTier {", "TestTimeline_")
m("TL the marker claims a same-program check it did not make", "internal/report/timeline_text.go",
  "\t\tif !sameProgramTier(c.Program) {\n\t\t\treturn \"  → no later success of the same command recorded\"", "\t\tif false {\n\t\t\treturn \"  → no later success of the same command recorded\"", "TestTimeline_")
m("TL tool_name reaches the terminal raw in the rewritten list", "internal/report/text.go",
  "\t\t\tprintable(r.ToolUseID), printable(r.ToolName), shape, what)", "\t\t\tprintable(r.ToolUseID), r.ToolName, shape, what)", "TestText_ToolNameIsPrintable")
m("TL tool_use_id reaches the terminal raw in the rewritten list", "internal/report/text.go",
  "\t\t\tprintable(r.ToolUseID), printable(r.ToolName), shape, what)", "\t\t\tr.ToolUseID, printable(r.ToolName), shape, what)", "TestText_ToolNameIsPrintable")
m("TL the rewritten list blanks the tool name", "internal/report/text.go",
  "\t\t\tprintable(r.ToolUseID), printable(r.ToolName), shape, what)", "\t\t\tprintable(r.ToolUseID), \"\", shape, what)", "TestText_ToolNameIsPrintable")
m("TL the program reaches the terminal raw in the rewritten list", "internal/report/text.go",
  "\t\t\tshape = printable(r.Program) + \", \" + r.VerbClass", "\t\t\tshape = r.Program + \", \" + r.VerbClass", "TestText_ToolNameIsPrintable")
m("TL a dropped call's id reaches the terminal raw", "internal/report/text.go",
  "everything but the id is unknown\\n\", printable(l.ToolUseID))", "everything but the id is unknown\\n\", l.ToolUseID)", "TestText_ToolNameIsPrintable")
m("TL the program reaches the terminal raw on the timeline", "internal/report/timeline_text.go",
  "\t\tcall += \" \" + printable(c.Program)", "\t\tcall += \" \" + c.Program", "TestText_ToolNameIsPrintable")
m("TL undeclared rows are ordered by their last record, not their outcome's", "internal/report/timeline.go",
  "\t\tp := outcomeSeq(outcomeRecord(executed[id]))", "\t\tp := outcomeSeq(outcomeRecord(executed[id][max(len(executed[id])-1, 0):]))", "TestTimeline_")
m("TL the legend does not name the programs with no program tier", "internal/report/timeline_text.go",
  "\t\tfmt.Fprintln(b, \"                 for wrappers and multi-command programs\")\n\t\tfmt.Fprintln(b, \"                 such as git, go, make, npm, python and sudo,\")\n", "", "TestTimeline_")
m("TL the legend leaves out calls with no program", "internal/report/timeline_text.go",
  "\t\tfmt.Fprintln(b, \"                 and for calls with no program such as Read or Edit, only the same command is;\")\n", "", "TestTimeline_")

# #36 review round 3, Still prints something false 1.
m("TL an undeclared failed call's ok record is dropped", "internal/report/timeline.go",
  "ok: failedCallOK(c, \"\", recs)})", "ok: nil})", "TestTimeline_")
m("TL only an undeclared ok row is weighed as a lost declaration", "internal/report/timeline.go",
  "\t\tif c.Seq == nil {\n\t\t\tpos, digest, tool :=", "\t\tif c.Seq == nil && c.Group == GroupOK {\n\t\t\tpos, digest, tool :=", "TestTimeline_")
m("TL an unnamed undeclared record is ruled out on its digest", "internal/report/timeline.go",
  "\t\t\tif tool == failed.call.ToolName && digest != \"\"", "\t\t\tif digest != \"\"", "TestTimeline_")
# #36 review round 3, Fix before merge 1.
m("TL a failed call's ok record never matches by command", "internal/report/timeline.go",
  "\t\t\t\tcommand := failed.digest != \"\" && ok.digest == failed.digest", "\t\t\t\tcommand := false", "TestTimeline_")
m("TL a failed call's ok record never matches by program", "internal/report/timeline.go",
  "\t\t\t\tprogram := programTier && c.Program == failed.call.Program", "\t\t\t\tprogram := false", "TestTimeline_")
m("TL a failed call's ok record matches by program with no program tier", "internal/report/timeline.go",
  "\t\t\t\tprogram := programTier && c.Program == failed.call.Program", "\t\t\t\tprogram := c.Program == failed.call.Program", "TestTimeline_")
m("TL a failed call's ok record with no position is ruled out", "internal/report/timeline.go",
  "(ok.pos == nil || *ok.pos > *failed.pos)", "(ok.pos != nil && *ok.pos > *failed.pos)", "TestTimeline_")
m("TL a failed call's first ok record is weighed, not its last", "internal/report/timeline.go",
  "\tfor i := len(recs) - 1; i >= 0; i-- {\n\t\tif recs[i].Outcome == store.ExecOK {", "\tfor i := 0; i < len(recs); i++ {\n\t\tif recs[i].Outcome == store.ExecOK {", "TestTimeline_")
m("TL an unknown digest matches a failed call's unknown ok digest", "internal/report/timeline.go",
  "\t\t\t\tcommand := failed.digest != \"\" && ok.digest == failed.digest", "\t\t\t\tcommand := ok.digest == failed.digest", "TestTimeline_")
m("TL a failed call's ok record is weighed by its declared digest", "internal/report/timeline.go",
  "digest: effectiveDigest(declared, &recs[i]), tool:", "digest: declared, tool:", "TestTimeline_")
# #36 review round 3, Fix before merge 2.
m("TL any executed digest reads as a rewrite", "internal/report/timeline.go",
  "rewritten: rec != nil && rec.ExecutedDigest != \"\" && rec.ExecutedDigest != d.Shape.Digest,", "rewritten: rec != nil && rec.ExecutedDigest != \"\",", "TestTimeline_")

# #36 review round 4, Fix before merge 1.
m("TL an undeclared ok record is ruled out under its call's tool name", "internal/report/timeline.go",
  "\t\t\tif tool == failed.call.ToolName && digest != \"\"", "\t\t\tif c.ToolName == failed.call.ToolName && digest != \"\"", "TestTimeline_")
m("TL an ok record with no tool name keeps an empty one", "internal/report/timeline.go",
  "\t\t\tif tool == \"\" {\n\t\t\t\ttool = LinkUnknown\n\t\t\t}\n", "", "TestTimeline_")

# #36 review round 4, Fix before merge 2.
m("TL an undeclared failed call is weighed by its failed record's digest", "internal/report/timeline.go",
  "\t\t\t\tpos, digest, tool = e.ok.pos, e.ok.digest, e.ok.tool\n", "\t\t\t\tpos, tool = e.ok.pos, e.ok.tool\n", "TestTimeline_")
m("TL an undeclared failed call's ok record has no position", "internal/report/timeline.go",
  "\t\t\t\tpos, digest, tool = e.ok.pos, e.ok.digest, e.ok.tool\n", "\t\t\t\tpos, digest, tool = nil, e.ok.digest, e.ok.tool\n", "TestTimeline_")
m("TL an undeclared interrupted or recordless call is weighed as a success", "internal/report/timeline.go",
  "\t\t\t\tpos, digest, tool = e.ok.pos, e.ok.digest, e.ok.tool\n\t\t\tdefault:\n\t\t\t\tcontinue\n", "\t\t\t\tpos, digest, tool = e.ok.pos, e.ok.digest, e.ok.tool\n", "TestTimeline_")

# Re-anchored after the masked-status refactor: the legend now goes on to the
# masked clause after this one; the backgrounded clause alone is cut.
m("TL the unknown legend drops the backgrounded call", "internal/report/timeline_text.go",
  ", or it was moved to the background before it ended;",
  ";",
  "TestTimeline_ABackgrounded")
