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
