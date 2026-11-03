# COMPACT-SLOT1 A: the continuity briefing (tech 4.2.4 :1255, STR-46). The
# person's words quoted verbatim after a compaction or refused whole by the
# line law; the steer line holds keyed digests and byte counts, never the
# words (D160); continuity-briefing admitted at priority 0 at SessionStart
# alone; the last compact_boundary read by its top-level keys; the briefing
# newest first inside its five lines and cap, counting what it leaves out.
S = "internal/steer/render/slot/said.go"
L = "internal/steer/slots.go"
F = "internal/steer/finding.go"
B = "internal/directive/boundary.go"
R = "internal/directive/briefing.go"
D = "internal/intent/digest.go"

m("CSL-1 a bare quote passes as the person's words", S,
  "\t\tcase c == '\\'' && (i == 0 || i == len(w)-1 || !isAlnum(w[i-1]) || !isAlnum(w[i+1])):\n"
  "\t\t\treturn errors.New(\"slot: said words hold a quote that is not an apostrophe\")\n", "",
  "TestCSL1_SaidQuotesThePersonsWordsOrRefusesThem")
m("CSL-1 a backtick passes as the person's words", S,
  "\t\tcase c == '<' || c == '>' || c == '`' || c == '\"':\n", "\t\tcase c == '<' || c == '>' || c == '\"':\n",
  "TestCSL1_SaidQuotesThePersonsWordsOrRefusesThem")
m("CSL-1 words over the cap are quoted", S,
  "\tif len(w) == 0 || len(w) > MaxSaidBytes {\n", "\tif len(w) == 0 {\n",
  "TestCSL1_SaidQuotesThePersonsWordsOrRefusesThem")
m("CSL-1 a high-entropy word is quoted", S,
  "\t\tif highEntropy(word) {\n", "\t\tif false && highEntropy(word) {\n",
  "TestCSL1_SaidQuotesThePersonsWordsOrRefusesThem")
m("CSL-1 the restated words skip the line law", S,
  "\tif s.restates != \"\" {\n\t\treturn sayable(s.restates)\n\t}\n", "",
  "TestCSL1_SaidQuotesThePersonsWordsOrRefusesThem")
m("CSL-1 the record form carries words", S,
  "\tcase s.words != \"\" || s.restates != \"\":\n\t\treturn errors.New(\"slot: said holds both words and digests\")\n", "",
  "TestCSL1_SaidRecordFormHoldsDigestsNeverWords")
m("CSL-1 the record form renders its digest", S,
  "\t\tt := \"a quote of \" + strconv.Itoa(s.n) + \" bytes\"\n", "\t\tt := \"a quote of \" + s.digest + \" bytes\"\n",
  "TestCSL1_SaidRecordFormHoldsDigestsNeverWords")
m("CSL-1 a steer line records the person's words", L,
  "\tif !v.Keyed() {\n\t\treturn fmt.Errorf(\"%w: %q holds the person's words, not their keyed digest\", errSlot, name)\n\t}\n", "",
  "TestCSL1_ALineRecordsTheKeyedFormAndNeverTheWords")
m("CSL-1 the briefing admitted on every event", F,
  "\treturn p == pattern.ContinuityBriefing && event == EventSessionStart\n", "\treturn p == pattern.ContinuityBriefing\n",
  "TestCSL1_TheBriefingIsAdmittedOnItsOwnEventAlone")
m("CSL-1 an own-channel pattern admitted at any priority", F,
  "own && !numbered && f.Priority == 0 {\n", "own && !numbered {\n",
  "TestCSL1_TheBriefingIsAdmittedOnItsOwnEventAlone")
m("CSL-1 an own-channel pattern skips the seq floor", F,
  "\t\treturn f.checkSeqs(1)\n\t}\n\tif p, ok := pattern.Priority(f.Pattern); !ok || f.Priority != p {\n",
  "\t\treturn nil\n\t}\n\tif p, ok := pattern.Priority(f.Pattern); !ok || f.Priority != p {\n",
  "TestCSL1_TheBriefingIsAdmittedOnItsOwnEventAlone")
m("CSL-1 the first boundary taken", B,
  "\t\t\t\tlast, found = Boundary{UUID: id, Line: n}, true\n",
  "\t\t\t\tlast, found = Boundary{UUID: id, Line: n}, true\n\t\t\t\treturn last, found, nil\n",
  "TestCSL1_LastBoundaryIsTheLastCompactRecord")
m("CSL-1 lines counted from 0", B,
  "\tfor n := 1; ; n++ {\n", "\tfor n := 0; ; n++ {\n", "TestCSL1_LastBoundaryIsTheLastCompactRecord")
m("CSL-1 a nested boundary read", B,
  "text(line, out[bkType]) != \"system\" ||", "false ||", "TestCSL1_LastBoundaryIsTheLastCompactRecord")
m("CSL-1 the read cap not kept", B,
  "\tif fi.Size() > capBytes {\n", "\tif false && fi.Size() > capBytes {\n", "TestCSL1_LastBoundaryIsTheLastCompactRecord")
m("CSL-1 directives after the boundary quoted", R,
  "\t\tif d.Line > 0 && d.Line < line {\n", "\t\tif d.Line > 0 {\n",
  "TestCSL1_TheBriefingQuotesWhatTheLastCompactionRemoved|TestCSL1_NoBriefingWithoutAQuoteADigestOrABoundary")
m("CSL-1 the briefing oldest first", R,
  "{ return b.Line - a.Line })", "{ return a.Line - b.Line })",
  "TestCSL1_TheBriefingQuotesWhatTheLastCompactionRemoved|TestCSL1_TheBriefingKeepsTheNewestWholeAndCountsTheRest")
m("CSL-1 a correction loses the words it restates", R,
  "\t\t\tq.restates = e.Said\n", "\t\t\tq.restates = \"\"\n\t\t\t_ = e\n",
  "TestCSL1_TheBriefingQuotesWhatTheLastCompactionRemoved")
m("CSL-1 a restated directive quoted on a line of its own", R,
  "\t\t\tcarried[d.Restates] = true\n", "", "TestCSL1_TheBriefingQuotesWhatTheLastCompactionRemoved")
m("CSL-1 a refused quote not counted", R,
  "\t\t\twithheld++\n", "", "TestCSL1_TheBriefingKeepsTheNewestWholeAndCountsTheRest")
m("CSL-1 the briefing's record holds the words", R,
  "binding(qs[:n], len(qs)-n, withheld, digest))\n", "binding(qs[:n], len(qs)-n, withheld, nil))\n",
  "TestCSL1_TheBriefingQuotesWhatTheLastCompactionRemoved")
m("CSL-1 a briefing with no digest to record by", R,
  "\tif digest == nil || b.UUID == \"\" {\n", "\tif b.UUID == \"\" {\n",
  "TestCSL1_NoBriefingWithoutAQuoteADigestOrABoundary")
m("CSL-1 the budget keeps the oldest", R,
  "binding(qs[:n], len(qs)-n, withheld, nil))\n", "binding(qs[len(qs)-n:], len(qs)-n, withheld, nil))\n",
  "TestCSL1_TheBriefingKeepsTheNewestWholeAndCountsTheRest")
m("CSL-1 the briefing keyed apart from its boundary", R,
  "Key: b.UUID,", "Key: \"continuity\",", "TestCSL1_TheBriefingQuotesWhatTheLastCompactionRemoved")
m("CSL-1 the quote digest unkeyed", D,
  "\th := hmac.New(sha256.New, key)\n", "\th := hmac.New(sha256.New, nil)\n", "TestCSL1_SaidDigestIsKeyedAndApartFromPaths")
m("CSL-1 the quote digest in the path domain", D,
  "\th.Write([]byte(saidDomain))\n", "\th.Write([]byte(pathDomain))\n", "TestCSL1_SaidDigestIsKeyedAndApartFromPaths")

# COMPACT-SLOT1 B: the briefing printed at a compact start. Route's context
# carries the start's source and attends as a prompt (D-e); the briefing is
# direct, outside the prompt budget (tech 4.2.5), and the skip is narrow (mgr
# 11-02: a widened ownChannel dies by the budget test); SessionStart admits
# after_seq 0; one briefing per session, agent and compaction (D-b); only at
# source compact; the lead opens the mirror.
HC = "internal/hook/promptcontext.go"
HS = "internal/hook/steerstep.go"
DC = "internal/steer/decide.go"
G = "internal/directive/given.go"
C = "cmd/rashomon/continuity.go"

m("CSL-1 the start's source not carried", HC,
  "event: string(steer.EventSessionStart), startSource: source}", "event: string(steer.EventSessionStart)}",
  "TestCSL1_StartContextCarriesItsSourceToTheChannelRow")
m("CSL-1 the context drops the start's source", HS,
  "\t\tStartSource: p.startSource}\n", "\t\tStartSource: \"\"}\n", "TestCSL1_StartContextCarriesItsSourceToTheChannelRow")
m("CSL-1 a compact start attends as a post", HC,
  " || event == string(steer.EventSessionStart) {\n", " {\n", "TestCSL1_StartAttendsAsThePromptDoes")
m("CSL-1 a compaction before any call is refused", F,
  " && event != EventSessionStart || f.AfterSeq != 0 {\n", " || f.AfterSeq != 0 {\n",
  "TestCSL1_TheBriefingIsAdmittedOnItsOwnEventAlone")
m("CSL-1 the briefing clamped for its missing prompt", DC,
  "\tcase ctx.PromptID == nil && !direct:\n", "\tcase ctx.PromptID == nil:\n",
  "TestCSL1_TheBriefingIsDirectOutsideThePromptBudget")
m("CSL-1 every pattern is direct at SessionStart", F,
  "\treturn p == pattern.ContinuityBriefing && event == EventSessionStart\n", "\treturn event == EventSessionStart\n",
  "TestCSL1_TheBriefingIsDirectOutsideThePromptBudget")
m("CSL-1 the briefing charged to the prompt's cap", DC,
  "\t\tcase p.direct && note < 0:\n\t\t\tp.state, p.decide, note = LineStateDelivered, false, i\n", "",
  "TestCSL1_TheBriefingIsDirectOutsideThePromptBudget")
m("CSL-1 a briefing whose key was delivered is given again", DC,
  "\t\tcase have.keys[p.key]:\n\t\t\tp.suppress(SuppressedByBudgetPattern)\n\t\tcase p.direct && note < 0:\n",
  "\t\tcase p.direct && note < 0:\n\t\t\tp.state, p.decide, note = LineStateDelivered, false, i\n"
  "\t\tcase have.keys[p.key]:\n\t\t\tp.suppress(SuppressedByBudgetPattern)\n\t\tcase false:\n",
  "TestCSL1_TheBriefingIsDirectOutsideThePromptBudget")
m("CSL-1 another compaction's briefing counts as given", G,
  "\t\t\ttext(b, out[gkKey]) != key || text(b, out[gkAgent]) != agent {\n", "\t\t\ttext(b, out[gkAgent]) != agent {\n",
  "TestCSL1_GivenIsOneBriefingPerSessionAgentAndCompaction")
m("CSL-1 a subagent's briefing counts as the main agent's", G,
  "\t\t\ttext(b, out[gkKey]) != key || text(b, out[gkAgent]) != agent {\n", "\t\t\ttext(b, out[gkKey]) != key {\n",
  "TestCSL1_GivenIsOneBriefingPerSessionAgentAndCompaction")
m("CSL-1 another pattern's line counts as given", G,
  " || text(b, out[gkPattern]) != string(pattern.ContinuityBriefing) ||\n",
  " || text(b, out[gkPattern]) != string(pattern.ContinuityBriefing) && false ||\n",
  "TestCSL1_GivenIsOneBriefingPerSessionAgentAndCompaction")
m("CSL-1 a briefing suppressed on its way counts as given", G,
  "\t\tif text(b, out[gkState]) == string(steer.LineStateDelivered) ||",
  "\t\tif true || text(b, out[gkState]) == string(steer.LineStateDelivered) ||",
  "TestCSL1_GivenIsOneBriefingPerSessionAgentAndCompaction")
m("CSL-1 an arm-off briefing is given again", G,
  " || text(b, out[gkSuppressedBy]) == string(steer.SuppressedByArmOff) {\n",
  " || false && text(b, out[gkSuppressedBy]) == string(steer.SuppressedByArmOff) {\n",
  "TestCSL1_GivenIsOneBriefingPerSessionAgentAndCompaction")
m("CSL-1 the briefing at every start source", C,
  "source != \"compact\" || ", "false || ", "TestCSL1_NoBriefingWithoutABoundaryOrAQuote")
m("CSL-1 a briefing given twice for one compaction", C,
  "\tif directive.Given(lines, b.UUID, \"\") {\n\t\treturn false\n\t}\n", "\t_ = lines\n",
  "TestCSL1_OneBriefingPerCompaction")
m("CSL-1 the briefing's mirror drops the lead", C,
  "\t\tout.SystemMessage = lead + \"\\n\" + out.SystemMessage\n", "", "TestCSL1_TheLeadOpensTheBriefingsMirror")
m("CSL-1 the lead closes the briefing's mirror", C,
  "\t\tout.SystemMessage = lead + \"\\n\" + out.SystemMessage\n", "\t\tout.SystemMessage += \"\\n\" + lead\n",
  "TestCSL1_TheLeadOpensTheBriefingsMirror")
m("CSL-1 the compact start routed as a startup", C,
  "mode, source, env)\n", "mode, \"startup\", env)\n", "TestCSL1_ACompactStartQuotesThePersonsWordsAndKeepsNone")
m("CSL-1 the quotes recorded by their words", C,
  "func(w string) string { return intent.SaidDigest(key, w) }", "func(w string) string { _, _ = key, intent.SaidDigest; return w }",
  "TestCSL1_ACompactStartQuotesThePersonsWordsAndKeepsNone")
m("CSL-1 the briefing's line filed under no project at v5", C,
  "\t\tproject = baseline.Key(cwd)\n", "\t\tproject, _ = \"\", baseline.Key\n", "TestCSL1_ACompactStartQuotesThePersonsWordsAndKeepsNone")
