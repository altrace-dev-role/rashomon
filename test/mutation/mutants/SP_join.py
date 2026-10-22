# `rashomon spend`. Each rule in internal/spend -- the read path, the dedupe,
# the window, the price table, the cold-cache heuristic, refusals and extra
# attempts, and the join to the store -- broken once.
m("SP a response's lines are summed, not counted once", "internal/spend/scan.go",
  "\t\tkeep(prev, cand)\n", "\t\tsc.Responses = append(sc.Responses, cand)\n\t\t_ = prev\n", "TestDedupe_")
m("SP the dedupe is per file, so a response carried into a second file counts twice", "internal/spend/scan.go",
  "\tbyID := map[string]*Response{}\n\tfor i, f := range found.Files {\n",
  "\tfor i, f := range found.Files {\n\t\tbyID := map[string]*Response{}\n", "TestDedupe_OneResponseInTwoFiles")
m("SP the first line of a response is kept, not the completed one", "internal/spend/scan.go",
  "\tif !better {\n\t\treturn\n\t}", "\tif !better || true {\n\t\treturn\n\t}", "TestDedupe_TheCompletedLine")
m("SP a response starts at its latest line, not its earliest", "internal/spend/scan.go",
  "\tif cand.StartMS != 0 && (prev.StartMS == 0 || cand.StartMS < prev.StartMS) {",
  "\tif cand.StartMS != 0 && (prev.StartMS == 0 || cand.StartMS > prev.StartMS) {", "TestDedupe_TheCompletedLine")
m("SP the transcript line decodes message.content", "internal/spend/line.go",
  "\tModel       string       `json:\"model\"`\n", "\tModel       string       `json:\"model\"`\n\tContent     json.RawMessage `json:\"content\"`\n",
  "TestContentHasNoFieldToLandIn")
IMPORTS["SP the transcript line decodes message.content"] = ("internal/spend/line.go", 'import "strings"\n', 'import (\n\t"encoding/json"\n\t"strings"\n)\n')
m("SP the --days window is ignored", "internal/spend/spend.go",
  "r.StartMS == 0 || r.StartMS < s.FromUnixMS {", "r.StartMS == 0 {", "TestWindow_")
m("SP an undated response is dropped without being counted", "internal/spend/scan.go",
  "\t\t\tsc.Undated++\n", "", "TestWindow_")
m("SP a file last written before the window is still read", "internal/spend/discover.go",
  "\t\t\tstale[resolved(p)] = true\n\t\t\treturn false", "\t\t\tstale[resolved(p)] = true\n\t\t\treturn true",
  "TestDiscover_Skips")
m("SP a file under subagents/ is main-agent spend unless its lines say sidechain", "internal/spend/scan.go",
  "Subagent:   f.Subagent || l.IsSidechain,", "Subagent:   l.IsSidechain,", "TestAgent_")
m("SP a main-file line marked isSidechain is main-agent spend", "internal/spend/scan.go",
  "Subagent:   f.Subagent || l.IsSidechain,", "Subagent:   f.Subagent,", "TestAgent_")
m("SP a workflow's subagent transcripts two levels down are not read", "internal/spend/discover.go",
  "\t\tif ok, _ := filepath.Match(\"agent-*.jsonl\", d.Name()); ok {",
  "\t\tif ok, _ := filepath.Match(\"agent-*.jsonl\", d.Name()); ok && filepath.Dir(p) == dir {", "TestAgent_")
m("SP any trailing hyphenated word is read as a dated suffix", "internal/spend/price.go",
  "\tif i < 0 || !isDate(model[i+1:]) {", "\tif i < 0 {", "TestPriceKey_")
m("SP a seven-digit suffix passes as a date", "internal/spend/price.go",
  "\tif len(s) != 8 {", "\tif len(s) < 7 {", "TestPriceKey_")
m("SP a 5m cache write is priced at the input rate", "internal/spend/price.go",
  "return r.Input * 5 / 4 }", "return r.Input }", "TestPricing_")
m("SP a 1h cache write is priced as a 5m one", "internal/spend/price.go",
  "return r.Input * 2 }", "return r.Input * 5 / 4 }", "TestPricing_")
m("SP Fable 5.1's cache read is 0.1x input, not the table's 0.25", "internal/spend/price.go",
  "\t\"claude-fable-5-1\": {Input: mtok(1000), Output: mtok(5000), CacheRead: mtok(25)},", "\t\"claude-fable-5-1\": {Input: mtok(1000), Output: mtok(5000), CacheRead: mtok(100)},", "TestPricing_TheTable")
m("SP an unsplit cache write is priced at the 1h rate", "internal/spend/line.go",
  "\t\tout.CacheWrite5m += rest", "\t\tout.CacheWrite1h += rest", "TestPricing_CacheWrites")
m("SP an unknown model is priced at $0", "internal/spend/amounts.go",
  "\tc.addUnpriced(p.tokens())\n", "\tc.addPriced(0)\n", "TestUnknownModel_")
m("SP an unpriced total marshals as usd 0 instead of null", "internal/spend/spend.go",
  "\tif c.WhollyUnpriced() {\n\t\treturn nil\n", "\tif false {\n\t\treturn nil\n", "TestUnknownModel_IsUnknown")
m("SP an unpriced total is headlined as a dollar figure", "internal/spend/text.go",
  "\tif c.WhollyUnpriced() {\n\t\treturn fmt.Sprintf(\"cost unknown (", "\tif false {\n\t\treturn fmt.Sprintf(\"cost unknown (",
  "TestUnknownModel_IsUnknown")
m("SP a non-claude model id is printed verbatim", "internal/spend/spend.go",
  "\treturn \"other\", false", "\treturn model, false", "TestUnknownModel_ANonClaude")
m("SP a claude-prefixed id is printed whatever it carries", "internal/spend/spend.go",
  "\t\tif !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {", "\t\tif c == 0 {", "TestUnknownModel_ANonClaude")
m("SP a zero-token synthetic response is listed as a model", "internal/spend/spend.go",
  "\t\tif r.Tokens.Total() == 0 {\n\t\t\tif r.StopReason == \"refusal\"", "\t\tif false {\n\t\t\tif r.StopReason == \"refusal\"", "TestZeroTokenResponses_")
m("SP an amount under a cent renders as $0.00", "internal/spend/text.go",
  "\tif nano > 0 && nano < 1e7 {", "\tif false && nano < 1e7 {", "TestSubCentAmounts_")
m("SP the plan-subscription sentence is not printed", "internal/spend/text.go",
  "\tfmt.Fprintf(&b, \"       %s\\n\", s.Pricing.Note)\n", "", "TestOutput_StatesTheBasis")
m("SP a 5m write is judged against the 1h TTL", "internal/spend/spend.go",
  "\t\t\tif gap > ttl5m {", "\t\t\tif gap > ttl1h {", "TestCacheExpiry_")
m("SP a 1h write is judged against the 5m TTL", "internal/spend/spend.go",
  "\t\t\tif gap > ttl1h {", "\t\t\tif gap > ttl5m {", "TestCacheExpiry_")
m("SP the previous response is taken across files", "internal/spend/spend.go",
  "\t\t\tk := stream{f.idx, r.Subagent}\n\t\t\tbyFile[k] = append(byFile[k], r)\n\t\t}\n\t}\n\tout := map[*Response]Tokens{}\n\tfor k, rs := range byFile {\n\t\tsort.SliceStable(rs, func(i, j int) bool { return rs[i].StartMS < rs[j].StartMS })\n\t\tfor i := 1; i < len(rs); i++ {\n\t\t\tif rs[i].file != k.file {",
  "\t\t\tk := stream{0 * f.idx, r.Subagent}\n\t\t\tbyFile[k] = append(byFile[k], r)\n\t\t}\n\t}\n\tout := map[*Response]Tokens{}\n\tfor k, rs := range byFile {\n\t\tsort.SliceStable(rs, func(i, j int) bool { return rs[i].StartMS < rs[j].StartMS })\n\t\tfor i := 1; i < len(rs); i++ {\n\t\t\tif false && rs[i].file != k.file {",
  "TestCacheExpiry_")
m("SP the window is applied before the previous response is found", "internal/spend/spend.go",
  "\tfor _, r := range sc.Responses {\n\t\tif r.StartMS == 0 || r.Tokens.Total() == 0 {\n\t\t\tcontinue\n\t\t}\n\t\tfor _, f := range r.files {",
  "\tfor _, r := range sc.Responses {\n\t\tif r.StartMS < time.Now().Add(-48*time.Hour).UnixMilli() {\n\t\t\tcontinue\n\t\t}\n\t\tfor _, f := range r.files {",
  "TestCacheExpiry_ThePredecessor")
m("SP a savings line is printed with no figure under it", "internal/spend/spend.go",
  "\tif s.SilentFailureTurns.Cost.Nano > 0 {", "\tif true {",
  "TestNoSavingsWithoutAFigure")
m("SP a refusal is not recognised", "internal/spend/spend.go",
  "\t\tif r.StopReason == \"refusal\" {\n\t\t\ts.Refusals.Responses++", "\t\tif r.StopReason == \"refused\" {\n\t\t\ts.Refusals.Responses++", "TestRefusalsAndExtraAttempts")
m("SP the returned attempt is counted again as an extra one", "internal/spend/scan.go",
  "\tout := make([]Tokens, 0, len(its)-1)\n\tfor _, it := range its[:len(its)-1] {", "\tout := make([]Tokens, 0, len(its)-1)\n\tfor _, it := range its {",
  "TestRefusalsAndExtraAttempts")
m("SP a turn with failures fires whatever its summary says", "internal/spend/join.go",
  "\t\tif !sf.Fires {", "\t\tif sf.Failed == 0 {",
  "TestJoin_AnHonestSummary")
m("SP a firing turn's spend is every main response of its transcript, not its prompt's", "internal/spend/join.go",
  "\t\tif f[prompt].Responses[r.ID] {", "\t\tif len(f[prompt].Responses) > 0 {",
  "TestJoin_ATurnsSpendIsKeyedByItsPrompt")
m("SP the transcript does not tie the response that made a turn's first call to it", "internal/report/transcript.go",
  "\t\t\tif head.Message.ID != \"\" {\n\t\t\t\ttie(head.Message.ID)", "\t\t\tif head.Message.ID != \"\" && bytes.Contains(raw, []byte(`\"text\"`)) {\n\t\t\t\ttie(head.Message.ID)",
  "TestJoin_SpendInside|TestSpend_JoinsSilentlyFailedTurns")
m("SP a subagent's spend is never its turn's", "internal/spend/join.go",
  "\t\treturn r.prompt == prompt && slices.Contains(mains, f.Main)", "\t\treturn false && r.prompt == prompt && slices.Contains(mains, f.Main)",
  "TestJoin_SpendInside")
m("SP a subagent response its transcript ties to another prompt is priced into the turn", "internal/spend/join.go",
  "\t\treturn r.prompt == prompt && slices.Contains(mains, f.Main)", "\t\treturn slices.Contains(mains, f.Main)",
  "TestJoin_ATurnsSpendIsKeyedByItsPrompt")
m("SP a subagent response under another conversation's transcript is priced into the turn", "internal/spend/join.go",
  "\t\treturn r.prompt == prompt && slices.Contains(mains, f.Main)", "\t\treturn r.prompt == prompt || slices.Contains(mains[:0], f.Main)",
  "TestJoin_OneSessionIDTwoConversations")
m("SP a subagent response's prompt is not read from its own transcript", "internal/spend/scan.go",
  "\t\t\tcand.prompt = prompt", "\t\t\tcand.prompt = prompt[:0]", "TestJoin_SpendInside")
m("SP a subagent transcript's user lines are skipped before their promptId is read", "internal/spend/scan.go",
  "\t\tif !mayCarry && !f.Subagent {", "\t\tif !mayCarry {",
  "TestJoin_SpendInside")
m("SP a subagent user line with no promptId does not end the tie", "internal/spend/scan.go",
  "\t\t\t} else if !l.IsMeta {\n\t\t\t\tprompt = \"\"", "\t\t\t} else if false {\n\t\t\t\tprompt = \"\"",
  "TestJoin_ATurnsSpendIsKeyedByItsPrompt")
m("SP a subagent meta line ends the tie", "internal/spend/scan.go",
  "\t\t\t} else if !l.IsMeta {\n\t\t\t\tprompt = \"\"", "\t\t\t} else {\n\t\t\t\tprompt = \"\"",
  "TestJoin_SpendInside")
m("SP a subagent line that does not decode keeps the tie", "internal/spend/scan.go",
  "\t\t\tprompt = \"\"\n\t\t\tcontinue\n\t\t}", "\t\t\tcontinue\n\t\t}",
  "TestJoin_ATurnsSpendIsKeyedByItsPrompt")
m("SP a subagent user line that does not decode is counted as unparsed usage", "internal/spend/scan.go",
  "\t\t\tif bytes.Contains(raw, usageMarker) {\n\t\t\t\tsc.Unparsed++", "\t\t\tif bytes.Contains(raw, usageMarker) || true {\n\t\t\t\tsc.Unparsed++",
  "TestUnparsed_")
m("SP a failed turn with no words to judge goes uncounted", "internal/spend/join.go",
  "\t\tif !sf.FinalMessageAvailable {\n\t\t\tj.Unjudged++", "\t\tif !sf.FinalMessageAvailable {\n\t\t\t_ = sf",
  "TestJoin_AFailedTurnWithNoWords")
m("SP a failed turn whose records name no transcript goes uncounted", "internal/spend/join.go",
  "\t\tif len(mains) == 0 {\n\t\t\tj.Unjudged++", "\t\tif len(mains) == 0 {\n\t\t\t_ = mains",
  "TestJoin_CoverageIsPerTranscript")
m("SP unjudged turns marshal as a checked 0 with nothing covered", "internal/spend/join.go",
  "\tif j.CoveredTranscripts > 0 {\n\t\tout.Turns, out.Unjudged, out.Undeclared, out.Cost = &j.Turns, &j.Unjudged, &j.UndeclaredFailedCalls, &j.Cost",
  "\tout.Unjudged = &j.Unjudged\n\tif j.CoveredTranscripts > 0 {\n\t\tout.Turns, out.Undeclared, out.Cost = &j.Turns, &j.UndeclaredFailedCalls, &j.Cost",
  "TestJoin_NoStore")
m("SP an unjudged failed turn reads as a checked none", "internal/spend/text.go",
  "\tcase j.Turns == 0 && j.Unjudged == 0 && j.UndeclaredFailedCalls == 0:\n", "\tcase j.Turns == 0 && j.UndeclaredFailedCalls == 0:\n", "TestJoin_AFailedTurnWithNoWords")
m("SP the unjudged turn is not said", "internal/spend/text.go",
  "\tif j.Unjudged == 1 {", "\tif false {", "TestJoin_AFailedTurnWithNoWords")
m("SP unjudged turns are not said", "internal/spend/text.go",
  "\t} else if j.Unjudged > 1 {", "\t} else if false {", "TestJoin_AFailedTurnWithNoWords")
m("SP a silent turn before the window is counted", "internal/spend/join.go",
  "\t\t\tif t.lastMS < s.FromUnixMS {", "\t\t\tif false {", "TestJoin_ATurnBefore")
m("SP a transcript counts as covered when its session id has a run directory", "internal/spend/join.go",
  "\t\trun, err := st.ReadRun(id)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n", "\t\trun, err := st.ReadRun(id)\n\t\tif err != nil {\n\t\t\treturn err\n\t\t}\n\t\tfor m, rs := range byTranscript {\n\t\t\tif slices.Contains(rs[0].owners, id) {\n\t\t\t\tcovered[m] = true\n\t\t\t}\n\t\t}\n",
  "TestJoin_CoverageIsPerTranscript")
m("SP a row holding not-covered dollars is not named", "internal/spend/join.go",
  "\t\t\tnamed[displaySession(id)] = true\n", "",
  "TestJoin_SpendInside|TestJoin_CoverageIsPerTranscript")
m("SP a partly recorded session is marked recorded", "internal/spend/join.go",
  "\t\tdefault:\n\t\t\ts.PerSession[i].Coverage = CoveragePartly", "\t\tdefault:\n\t\t\ts.PerSession[i].Coverage = CoverageRecorded",
  "TestJoin_CoverageIsPerTranscript")
m("SP the text does not name the not-covered sessions", "internal/spend/text.go",
  "\tif len(n) == 0 {\n\t\treturn \"\"", "\tif true {\n\t\treturn \"\"",
  "TestJoin_SpendInside|TestJoin_CoverageIsPerTranscript|TestSpend_JoinsSilentlyFailedTurns")
m("SP a covered, clean record reads at least none across 0 turns", "internal/spend/text.go",
  "\tcase j.Turns == 0 && j.Unjudged == 0 && j.UndeclaredFailedCalls == 0:\n", "\tcase false:\n", "TestJoin_ACoveredZeroIsAZero")
m("SP a firing turn with nothing priced reads at least none", "internal/spend/text.go",
  "\tcase j.Cost.Priced == 0 && j.Cost.Unpriced == 0:\n", "\tcase false:\n", "TestJoin_AFiringTurnWithNoResponse")
m("SP the floor note is printed under a none", "internal/spend/text.go",
  "\tif j.Turns > 0 {\n\t\tfmt.Fprintf(&b, \"  (%s)\\n\", j.Bound)", "\tif true {\n\t\tfmt.Fprintf(&b, \"  (%s)\\n\", j.Bound)",
  "TestJoin_ACoveredZeroIsAZero")
m("SP an unrecorded session's spend is folded in as zero", "internal/spend/join.go",
  "\t\tcostOf(&j.NotCoveredCost, r)", "\t\t_ = r", "TestJoin_")
m("SP no store renders as a store that recorded nothing here", "internal/spend/text.go",
  "\tcase j.Store == StoreNone:", "\tcase j.Store == \"never\":", "TestJoin_NoStore|TestSpend_OpensNoStore")
m("SP spend creates a store to read one", "cmd/rashomon/main.go",
  "\tst, err := openStoreForRead()\n\tswitch {", "\tst, err := openStore()\n\tswitch {", "TestSpend_OpensNoStore")
m("SP spend ignores CLAUDE_CONFIG_DIR", "cmd/rashomon/main.go",
  "\tconfigDir, err := settings.ConfigDir()\n\tif err != nil {\n\t\treturn err\n\t}\n\tfiles, err := spend.Discover(configDir,",
  "\tconfigDir, err := os.UserHomeDir()\n\tconfigDir = filepath.Join(configDir, \".claude\")\n\tif err != nil {\n\t\treturn err\n\t}\n\tfiles, err := spend.Discover(configDir,",
  "TestSpend_ReadsWhere")
m("SP --days 0 is accepted", "cmd/rashomon/main.go",
  "if err != nil || n < 1 {", "if err != nil {", "TestSpend_RefusesAWindow")

m("SP a tool-less prompt's reply is read as the previous turn's summary", "internal/report/transcript.go",
  "\t\t\t\tcurrent = head.PromptID\n", "\t\t\t\tif want[head.PromptID] {\n\t\t\t\t\tcurrent = head.PromptID\n\t\t\t\t}\n",
  "TestJoin_AToolLessTurnBetween")
m("SP an unkeyed prompt's reply is credited to the turn before it", "internal/report/transcript.go",
  "\t\t\t} else if !head.IsMeta && !toolResultOnly(raw) {", "\t\t\t} else if false {",
  "TestJoin_AnUnkeyedPrompt|TestFinalAssistantTexts_")
m("SP a tool_result without promptId ends its turn's words", "internal/report/transcript.go",
  "\t\t\t} else if !head.IsMeta && !toolResultOnly(raw) {", "\t\t\t} else if !head.IsMeta {",
  "TestFinalAssistantTexts_")
m("SP a meta line without promptId ends its turn's words", "internal/report/transcript.go",
  "\t\t\t} else if !head.IsMeta && !toolResultOnly(raw) {", "\t\t\t} else if !toolResultOnly(raw) {",
  "TestFinalAssistantTexts_")
m("SP the bound says every unkeyed user line ends a tie", "internal/spend/join.go",
  "a user line with no promptId that is not a meta line or, in the main transcript, a tool result, or after a line that cannot be decoded (any in a subagent transcript; in the main transcript, unless it is a sidechain line), is tied to no turn", "a user line with no promptId is tied to no turn",
  "TestJoin_TheBoundNamesTheUnkeyedLinesThatKeepATie")
m("SP a subagent's sidechain line is read as the turn's summary", "internal/report/transcript.go",
  "\t\tif head.IsSidechain {\n", "\t\tif false && head.IsSidechain {\n",
  "TestFinalAssistantTexts_")
m("SP an unbilled synthetic line counts as the previous request", "internal/spend/spend.go",
  "\t\tif r.StartMS == 0 || r.Tokens.Total() == 0 {\n\t\t\tcontinue\n\t\t}\n\t\tfor _, f := range r.files {",
  "\t\tif r.StartMS == 0 {\n\t\t\tcontinue\n\t\t}\n\t\tfor _, f := range r.files {",
  "TestCacheExpiry_AnUnbilled")
m("SP an unchecked silent-failure line marshals as a checked $0", "internal/spend/join.go",
  "\tif j.CoveredTranscripts > 0 {\n\t\tout.Turns", "\tif true {\n\t\tout.Turns",
  "TestJoin_NoStore")
m("SP by-agent shares are printed beside an unknown", "internal/spend/text.go",
  "\tif main.Nano+sub.Nano == 0 || !main.Known() || !sub.Known() {", "\tif main.Nano+sub.Nano == 0 {",
  "TestAgent_NoShareBesideAnUnknown")
m("SP a user line's tool-result test decodes the text of its blocks", "internal/report/transcript.go",
  "\t\tContent []struct {\n\t\t\tType string `json:\"type\"`\n\t\t} `json:\"content\"`",
  "\t\tContent []struct {\n\t\t\tType string `json:\"type\"`\n\t\t\tText string `json:\"text\"`\n\t\t} `json:\"content\"`",
  "TestUserBlocks_")
m("SP a transcript's session id is printed whatever it carries", "internal/spend/spend.go",
  "\tif idShaped(id) {\n\t\treturn id", "\tif true {\n\t\treturn id", "TestSessionID_")
m("SP the window is a Duration of days, which overflows", "internal/spend/spend.go",
  "\treturn now.AddDate(0, 0, -days)", "\treturn now.Add(-time.Duration(days) * 24 * time.Hour)",
  "TestWindow_AVeryLong")
m("SP --days past the century is accepted", "cmd/rashomon/main.go",
  "\t\t\tif n > spend.MaxDays {", "\t\t\tif false && n > spend.MaxDays {", "TestSpend_RefusesAWindow")
m("SP a future-dated response counts as the last N days", "internal/spend/spend.go",
  "\t\tif r.StartMS > latest && r.Tokens.Total() > 0 {", "\t\tif false && r.StartMS > latest && r.Tokens.Total() > 0 {",
  "TestWindow_AFutureDated")
m("SP a response written while spend runs is future-dated", "internal/spend/spend.go",
  "\tlatest := now.Add(futureSlack).UnixMilli()", "\tlatest := now.UnixMilli()", "TestWindow_AFutureDated")
m("SP a transcript last written before the window is not counted", "internal/spend/discover.go",
  "\t\t\tstale[resolved(p)] = true\n\t\t\treturn false", "\t\t\treturn false",
  "TestWindow_OldTranscripts|TestDiscover_Skips")
m("SP old transcripts are reported as no transcripts found", "internal/spend/text.go",
  "\tcase s.Read.Files == 0 && s.Read.FilesBeforeWindow > 0:", "\tcase false:", "TestWindow_OldTranscripts")
m("SP a symlinked project folder is skipped", "internal/spend/discover.go",
  "\tif t&fs.ModeSymlink != 0 {", "\tif false && t&fs.ModeSymlink != 0 {", "TestDiscover_ASymlinked")
m("SP a recorded real path does not cover a transcript reached through a symlink", "internal/spend/join.go",
  "\t\tif real, err := filepath.EvalSymlinks(p); err == nil {", "\t\tif real, err := filepath.EvalSymlinks(p); err == nil && false {",
  "TestDiscover_ASymlinked")
m("SP a discovered transcript is resolved as written, not by its absolute spelling", "internal/spend/join.go",
  "\t\tif real, err := filepath.EvalSymlinks(p); err == nil {", "\t\tif real, err := filepath.EvalSymlinks(f.Path); err == nil {",
  "TestDiscover_ASymlinked")
m("SP a discovered transcript is not indexed by its absolute spelling", "internal/spend/join.go",
  "\tif abs, err := filepath.Abs(f.Path); err == nil {\n\t\t\tp = abs", "\tif abs, err := filepath.Abs(f.Path); err == nil {\n\t\t\t_ = abs",
  "TestDiscover_ASymlinked")
m("SP one transcript under two spellings is read twice", "internal/spend/discover.go",
  "\t\tif seen[real] {\n\t\t\tcontinue", "\t\tif false && seen[real] {\n\t\t\tcontinue",
  "TestDiscover_OneTranscriptUnderTwoSpellings")
m("SP a project folder that cannot be read is not counted", "internal/spend/discover.go",
  "\t\tif kind == kindUnreadable {\n\t\t\tunreadable[resolved(dir)] = true", "\t\tif kind == kindUnreadable {\n\t\t\t_ = dir",
  "TestDiscover_AnUnreadableFolder")
m("SP a session folder that cannot be read is not counted", "internal/spend/discover.go",
  "\t\t\tcase kind == kindUnreadable:\n\t\t\t\tunreadable[resolved(path)] = true", "\t\t\tcase kind == kindUnreadable:\n\t\t\t\t_ = path",
  "TestDiscover_AnUnreadableFolder")
m("SP folders that could not be read are not said", "internal/spend/text.go",
  "\tif s.Read.UnreadableDirs > 0 {", "\tif false {", "TestDiscover_AnUnreadableFolder")
m("SP a usage line that does not decode is dropped without a count", "internal/spend/scan.go",
  "\t\t\tif bytes.Contains(raw, usageMarker) {\n\t\t\t\tsc.Unparsed++", "\t\t\tif bytes.Contains(raw, usageMarker) {\n\t\t\t\t_ = sc",
  "TestUnparsed_")
m("SP an implausible usage is priced", "internal/spend/scan.go",
  "\t\tif !l.Message.Usage.plausible() {", "\t\tif false && !l.Message.Usage.plausible() {", "TestUnparsed_")
m("SP a negative token count is plausible", "internal/spend/line.go",
  "\t\t\tif v < 0 || v > maxTokens {", "\t\t\tif v > maxTokens {", "TestUnparsed_")
m("SP an implausibly large token count is plausible", "internal/spend/line.go",
  "\t\t\tif v < 0 || v > maxTokens {", "\t\t\tif v < 0 {", "TestUnparsed_")
m("SP an iteration's counts are not checked", "internal/spend/line.go",
  "\tfor _, it := range u.Iterations {\n\t\tif !ok(it.tokens) {", "\tfor _, it := range u.Iterations {\n\t\tif false && !ok(it.tokens) {",
  "TestUnparsed_")
m("SP unparsed usage lines are not said", "internal/spend/text.go",
  "\tif s.Read.UnparsedUsageLines > 0 {", "\tif false {", "TestUnparsed_")
m("SP a main transcript's sidechain lines share the main agent's cache stream", "internal/spend/spend.go",
  "\t\t\tk := stream{f.idx, r.Subagent}\n", "\t\t\tk := stream{f.idx, false}\n", "TestCacheExpiry_ThePreviousResponseIsTheSameAgents")
m("SP each agent share is rounded on its own", "internal/spend/text.go",
  "\tif pa+pb < 100 {", "\tif false {", "TestAgent_SharesSumTo100")
m("SP the leftover share point goes to the smaller remainder", "internal/spend/text.go",
  "\t\tif ra >= rb {", "\t\tif ra < rb {", "TestAgent_SharesSumTo100")
m("SP a non-zero side under 1% prints 0%", "internal/spend/text.go",
  "\t\tcase p == 0 && n > 0:", "\t\tcase false:", "TestAgent_SharesSumTo100")
m("SP a share of 100% is printed beside a non-zero other side", "internal/spend/text.go",
  "\t\tcase p == 100 && uint64(n) < total:", "\t\tcase false:", "TestAgent_SharesSumTo100")
m("SP per-session spend is JSON-only", "internal/spend/text.go",
  "\t\tfor i, line := range sessionLines(s.PerSession) {", "\t\tfor i, line := range sessionLines(nil) {",
  "TestSessions_")
m("SP the text lists every session", "internal/spend/text.go",
  "\t\tif i == maxNamed {", "\t\tif i == -1 {", "TestSessions_")
m("SP a session rashomon did not record is not marked", "internal/spend/text.go",
  "\t\tcase CoverageNotRecorded:\n\t\t\tline +=", "\t\tcase \"never\":\n\t\t\tline +=", "TestSessions_")
m("SP a partly recorded session is not marked", "internal/spend/text.go",
  "\t\tcase CoveragePartly:\n\t\t\tline +=", "\t\tcase \"never\":\n\t\t\tline +=", "TestJoin_CoverageIsPerTranscript")
m("SP a recorded transcript_path is not resolved through a symlinked ancestor", "internal/spend/join.go",
  "\t\tif r, err := filepath.EvalSymlinks(recorded); err == nil {", "\t\tif r, err := filepath.EvalSymlinks(recorded); err == nil && false {",
  "TestDiscover_ASymlinked")
m("SP a retired model's row is priced at a current model's rates", "internal/spend/price.go",
  "\t\"claude-opus-4-1\": {Input: mtok(1500), Output: mtok(7500), CacheRead: mtok(150)},", "\t\"claude-opus-4-1\": {Input: mtok(500), Output: mtok(2500), CacheRead: mtok(50)},",
  "TestPricing_TheTable")
m("SP Haiku 3.5 has no row and prices as unknown", "internal/spend/price.go",
  "\t\"claude-3-5-haiku\": {Input: mtok(80), Output: mtok(400), CacheRead: mtok(8)},\n", "",
  "TestPricing_TheTable|TestPriceKey_")
m("SP a tool_use block's input is decoded with an assistant line's text blocks", "internal/report/transcript.go",
  "type textBlock struct {\n\tType string `json:\"type\"`\n\tText string `json:\"text\"`\n}",
  "type textBlock struct {\n\tType  string          `json:\"type\"`\n\tText  string          `json:\"text\"`\n\tInput json.RawMessage `json:\"input\"`\n}",
  "TestAssistantLine_")
m("SP a block of any type is kept as an assistant line's text", "internal/report/transcript.go",
  "\t\tif blk.Type == \"text\" && strings.TrimSpace(blk.Text) != \"\" {", "\t\tif strings.TrimSpace(blk.Text) != \"\" {",
  "TestAssistantLine_")
m("SP a firing turn's final words are printed to stderr", "internal/spend/join.go",
  "\t\tsf := report.BuildSilentFailures(t.run, report.AccountFromMessage(lastSaid(byFile, t.prompt)))\n",
  "\t\tsf := report.BuildSilentFailures(t.run, report.AccountFromMessage(lastSaid(byFile, t.prompt)))\n\t\tfmt.Fprintln(os.Stderr, lastSaid(byFile, t.prompt))\n",
  "TestSpend_NoMessageTextReachesTheOutput")
IMPORTS["SP a firing turn's final words are printed to stderr"] = ("internal/spend/join.go", '\t"encoding/json"\n', '\t"encoding/json"\n\t"fmt"\n')
m("SP a firing turn's final words reach the JSON", "internal/spend/join.go",
  "\t\tif !sf.Fires {\n\t\t\tcontinue\n\t\t}\n\t\tj.Turns++\n",
  "\t\tif !sf.Fires {\n\t\t\tcontinue\n\t\t}\n\t\tj.Turns++\n\t\tj.Bound = lastSaid(byFile, t.prompt)\n",
  "TestJoin_NoMessageTextReachesTheOutput|TestSpend_NoMessageTextReachesTheOutput")
m("SP a main-transcript sidechain response is tied to no turn", "internal/report/transcript.go",
  "\t\t\tif head.Type == \"assistant\" && want[current] && head.Message.ID != \"\" {\n\t\t\t\ttie(head.Message.ID)",
  "\t\t\tif false && head.Type == \"assistant\" && want[current] && head.Message.ID != \"\" {\n\t\t\t\ttie(head.Message.ID)",
  "TestJoin_AMainTranscriptSidechain")
m("SP a main-transcript sidechain user line moves the tie", "internal/report/transcript.go",
  "\t\t\t\ttie(head.Message.ID)\n\t\t\t}\n\t\t\tcontinue\n",
  "\t\t\t\ttie(head.Message.ID)\n\t\t\t}\n\t\t\tif head.Type == \"user\" && head.PromptID != \"\" {\n\t\t\t\tcurrent = head.PromptID\n\t\t\t}\n\t\t\tcontinue\n",
  "TestJoin_AMainTranscriptSidechain")
m("SP a store that covers no transcript renders as a checked none", "internal/spend/text.go",
  "\tcase j.CoveredTranscripts == 0:\n", "\tcase false:\n", "TestJoin_AStoreThatCoversNoTranscript")
m("SP a transcript that cannot be read to the end is not counted", "internal/spend/scan.go",
  "\t\tif err := readFile(sc, byID, i, f); err != nil {\n\t\t\tsc.Unreadable++", "\t\tif err := readFile(sc, byID, i, f); err != nil {\n\t\t\t_ = sc",
  "TestRead_AnUnreadableTranscript")
m("SP a transcript that cannot be read to the end is not said", "internal/spend/text.go",
  "\tif s.Read.UnreadableFiles > 0 {", "\tif false {", "TestRead_AnUnreadableTranscript")
m("SP cmdSpend reads every transcript ever written", "cmd/rashomon/main.go",
  "\tfiles, err := spend.Discover(configDir, spend.WindowStart(now, days))", "\tfiles, err := spend.Discover(configDir, time.Time{})",
  "TestSpend_OldTranscriptsAreSkippedAndSaid")
m("SP a text line with no parseable timestamp is skipped, and the turn judged on earlier words", "internal/report/transcript.go",
  "\t\t\tif err != nil {\n\t\t\t\tunsay()\n\t\t\t\tcontinue\n\t\t\t}", "\t\t\tif err != nil {\n\t\t\t\tcontinue\n\t\t\t}",
  "TestFinalAssistantTexts_")
m("SP a main-transcript user line that does not decode keeps the tie", "internal/report/transcript.go",
  "\t\t\tunsay()\n\t\t\tcurrent = \"\"\n\t\t\tcontinue\n\t\tcase LineUndecodableSidechain:", "\t\t\tunsay()\n\t\t\tcontinue\n\t\tcase LineUndecodableSidechain:",
  "TestFinalAssistantTexts_")
m("SP a line that does not decode leaves the turn's earlier words standing", "internal/report/transcript.go",
  "\t\t\tunsay()\n\t\t\tcurrent = \"\"\n\t\t\tcontinue\n\t\tcase LineUndecodableSidechain:", "\t\t\tcurrent = \"\"\n\t\t\tcontinue\n\t\tcase LineUndecodableSidechain:",
  "TestFinalAssistantTexts_")
m("SP a tie between two main files' final words goes to the first", "internal/spend/join.go",
  "t.Said && t.AtMS >= bestMS {", "t.Said && t.AtMS > bestMS {", "TestJoin_TheLastWord")
m("SP an earlier main file's final word beats a later one", "internal/spend/join.go",
  "t.Said && t.AtMS >= bestMS {", "t.Said && (bestMS == math.MinInt64 || t.AtMS < bestMS) {", "TestJoin_TheLastWord")
m("SP a transcript not read to the end still gives a turn its words", "internal/report/transcript.go",
  "\tif sc.Err() != nil {\n\t\t// A file that cannot be read to the end may hold a later reply", "\tif false {\n\t\t// A file that cannot be read to the end may hold a later reply",
  "TestJoin_ATranscriptCut")
m("SP a turn's final word carries no time", "internal/report/transcript.go",
  "\t\t\ttf.Said, tf.Text, tf.AtMS = true, text, at.UnixMilli()", "\t\t\ttf.Said, tf.Text, tf.AtMS = true, text, 0*at.UnixMilli()",
  "TestFinalAssistantTexts_ATurnIsItsPrompt")
m("SP an id-less usage line with tokens is dropped without a count", "internal/spend/scan.go",
  "\t\t\tif l.Message.Usage.carriesTokens() {\n\t\t\t\tsc.Unparsed++", "\t\t\tif false {\n\t\t\t\tsc.Unparsed++",
  "TestUnparsed_")
m("SP an id-less usage line is counted as a usage line", "internal/spend/scan.go",
  "\t\tif l.Message.ID == \"\" {\n\t\t\t// No id to deduplicate against", "\t\tif l.Message.ID == \"\" {\n\t\t\tsc.UsageLines++\n\t\t\t// No id to deduplicate against",
  "TestUnparsed_")
m("SP a transcript that cannot be stat'ed is dropped without a count", "internal/spend/discover.go",
  "\t\t\treturn !errors.Is(err, fs.ErrNotExist)", "\t\t\treturn false", "TestDiscover_AFileThatCannotBeStated")
m("SP a transcript that vanished is still read", "internal/spend/discover.go",
  "\t\t\treturn !errors.Is(err, fs.ErrNotExist)", "\t\t\treturn true", "TestDiscover_AFileThatCannotBeStated")
m("SP a share's percentage overflows", "internal/spend/text.go",
  "\thi, lo := bits.Mul64(n, 100)\n\treturn bits.Div64(hi, lo, total)", "\t_, _ = bits.Mul64(n, 100)\n\treturn n * 100 / total, n * 100 % total",
  "TestAgent_Shares")
m("SP a declaration with no prompt_id is grouped as the turn \"\"", "internal/spend/join.go",
  "\t\tif d.PromptID == nil || *d.PromptID == \"\" {\n\t\t\tcontinue\n\t\t}",
  "\t\tif d.PromptID == nil {\n\t\t\tnone := \"\"\n\t\t\td.PromptID = &none\n\t\t}",
  "TestJoin_ADeclarationWithNoPromptID")
m("SP a declaration with no prompt_id is dereferenced", "internal/spend/join.go",
  "\t\tif d.PromptID == nil || *d.PromptID == \"\" {\n\t\t\tcontinue\n\t\t}", "",
  "TestJoin_ADeclarationWithNoPromptID")
m("SP a wholly unpriced silent-failure cost reads at least unknown", "internal/spend/text.go",
  "\tcase j.Cost.WhollyUnpriced():\n", "\tcase false:\n", "TestJoin_AWhollyUnpricedTurn")
m("SP an empty window reads unknown over 0 transcripts", "internal/spend/text.go",
  "\tcase j.Transcripts == 0:\n", "\tcase false:\n", "TestJoin_NoTranscriptInTheWindow")
m("SP a fast-mode response is not counted", "internal/spend/scan.go",
  "\t\tFast:       l.Message.Usage.Speed == \"fast\",", "\t\tFast:       l.Message.Usage.Speed == \"turbo\",",
  "TestFastMode_")
m("SP fast-mode responses are not said", "internal/spend/text.go",
  "\tif s.FastMode.Responses > 0 {", "\tif false {", "TestFastMode_")
m("SP the out-of-scope line does not name web-search fees", "internal/spend/text.go",
  "; web-search fees (%s)\\n\", WebSearchFee)", "; %s\\n\", WebSearchFee)", "TestFastMode_")
m("SP the web-search fee is not the dated table's", "internal/spend/price.go",
  "const WebSearchFee = \"$10 per 1,000 searches\"", "const WebSearchFee = \"$5 per 1,000 searches\"", "TestFastMode_")
m("SP Mythos 5 prices its cache reads at Mythos 5.1's rate", "internal/spend/price.go",
  "\t\"claude-mythos-5\":   {Input: mtok(1000), Output: mtok(5000), CacheRead: mtok(100)},", "\t\"claude-mythos-5\":   {Input: mtok(1000), Output: mtok(5000), CacheRead: mtok(25)},",
  "TestPricing_TheTable")
m("SP a decoded field under an allowed tag holds whatever it is handed", "internal/spend/line.go",
  "\tType        string  `json:\"type\"`", "\tType        any     `json:\"type\"`", "TestContentHasNoFieldToLandIn")
m("SP a zero-usage refusal line is not counted", "internal/spend/spend.go",
  "\t\t\t\ts.Refusals.WithoutUsage++", "\t\t\t\t_ = s", "TestRefusals_")
m("SP refusals without usage read as refusals none", "internal/spend/text.go",
  "\tif r.Responses == 0 && r.BeforeOutput == 0 && r.WithoutUsage == 0 {", "\tif r.Responses == 0 && r.BeforeOutput == 0 {",
  "TestRefusals_")
m("SP a refusal without usage is not said when nothing else was billed", "internal/spend/text.go",
  "\t} else if s.Refusals.WithoutUsage > 0 || s.Refusals.BeforeOutput > 0 {", "\t} else if false {",
  "TestRefusals_")
m("SP the silent-failure line says the turns ended with a failure", "internal/spend/text.go",
  'const lead = "in turns with a failed call the summary never mentioned: "', 'const lead = "in turns that ended with a failure the summary never mentioned: "',
  "TestJoin_SpendInsideASilentlyFailedTurn")
m("SP the silent-failure saving says it bought a done", "internal/spend/text.go",
  '"%s spent in turns with a failed call the summary never mentioned"', '"%s bought a \\"done\\" in turns whose recorded failures the summary never mentioned"',
  "TestJoin_SpendInsideASilentlyFailedTurn")
m("SP a write on a response that read back the previous cache is counted cold", "internal/spend/spend.go",
  "short := max(0, cached-cur.CacheRead)", "short := max(0, cached-0*cur.CacheRead)",
  "TestCacheExpiry_AWriteOnAWarmCacheIsNotCold")
m("SP a cold write is counted whole, not its shortfall", "internal/spend/spend.go",
  "\t\t\tshort := max(0, cached-cur.CacheRead)", "\t\t\tshort := cur.CacheWrite5m + cur.CacheWrite1h + 0*cached",
  "TestCacheExpiry_APartialExpiry")
m("SP a write with any cache read is not cold", "internal/spend/spend.go",
  "\t\t\tprev, cur := rs[i-1].Tokens, rs[i].Tokens\n", "\t\t\tprev, cur := rs[i-1].Tokens, rs[i].Tokens\n\t\t\tif cur.CacheRead > 0 {\n\t\t\t\tcontinue\n\t\t\t}\n",
  "TestCacheExpiry_APartialExpiry")
m("SP a cold write is priced at the full write rate, not over a cache read", "internal/spend/amounts.go",
  "w.CacheWrite5m*(rt.CacheWrite5m()-rt.CacheRead) + w.CacheWrite1h*(rt.CacheWrite1h()-rt.CacheRead)", "w.CacheWrite5m*rt.CacheWrite5m() + w.CacheWrite1h*(rt.CacheWrite1h()-rt.CacheRead)",
  "TestCacheExpiry_")
m("SP a cold 1h write is priced at the full write rate", "internal/spend/amounts.go",
  "w.CacheWrite5m*(rt.CacheWrite5m()-rt.CacheRead) + w.CacheWrite1h*(rt.CacheWrite1h()-rt.CacheRead)", "w.CacheWrite5m*(rt.CacheWrite5m()-rt.CacheRead) + w.CacheWrite1h*rt.CacheWrite1h()",
  "TestCacheExpiry_")
m("SP a response keeps only the file it was first seen in", "internal/spend/scan.go",
  "\tif !slices.ContainsFunc(prev.files, func(s sighting) bool { return s.idx == cand.file }) {\n\t\tprev.files = append(prev.files, cand.files[0])\n\t}\n", "\t_ = slices.ContainsFunc[[]sighting]\n",
  "TestJoin_ADuplicatedResponse")
m("SP a shared response's coverage follows the file that sorts first", "internal/spend/join.go",
  "\t\t\tfor _, m := range s.mainsOf(r) {\n\t\t\t\tif !covered[m] {\n\t\t\t\t\tnotCovered = true", "\t\t\tfor _, m := range s.mainsOf(r)[:1] {\n\t\t\t\tif !covered[m] {\n\t\t\t\t\tnotCovered = true",
  "TestJoin_ADuplicatedResponse")
m("SP a transcript holding only shared responses is not a transcript", "internal/spend/join.go",
  "\t\t\tfor _, m := range s.mainsOf(r) {\n\t\t\t\tbyTranscript[m] = append(byTranscript[m], r)", "\t\t\tfor _, m := range s.mainsOf(r)[:1] {\n\t\t\t\tbyTranscript[m] = append(byTranscript[m], r)",
  "TestJoin_ADuplicatedResponse")
m("SP a cold-cache stream holds only the responses first seen in its file", "internal/spend/spend.go",
  "\t\tfor _, f := range r.files {\n\t\t\tk := stream{f.idx, r.Subagent}", "\t\tfor _, f := range r.files[:1] {\n\t\t\tk := stream{f.idx, r.Subagent}",
  "TestJoin_ADuplicatedResponse")
m("SP an iteration entry's type is decoded as counts only", "internal/spend/line.go",
  "\tType string `json:\"type\"`\n\t// Model is the model that ran the attempt.", "\tType string `json:\"-\"`\n\t// Model is the model that ran the attempt.",
  "TestExtraAttempts_")
m("SP an iteration entry's model is not decoded", "internal/spend/line.go",
  "\tModel string `json:\"model\"`\n}", "\tModel string `json:\"-\"`\n}",
  "TestExtraAttempts_")
m("SP a fallback-served response is not counted", "internal/spend/spend.go",
  "\tif r.Fallback && r.StopReason != \"refusal\" {\n\t\te.FallbackServed++", "\tif false && r.Fallback {\n\t\te.FallbackServed++",
  "TestExtraAttempts_")
m("SP the headline does not say the total leaves out the extra attempts", "internal/spend/text.go",
  "\tif a := s.ExtraAttempts; a.Attempts > 0 {", "\tif a := s.ExtraAttempts; false && a.Attempts > 0 {",
  "TestExtraAttempts_TheFallbackPagesExample|TestRefusalsAndExtraAttempts")
m("SP a refusal's category is not decoded", "internal/spend/line.go",
  "\tCategory *string `json:\"category\"`", "\tCategory *string `json:\"-\"`",
  "TestRefusals_AreSplitByCategoryAndModel")
m("SP a category outside the vocabulary is kept as read", "internal/spend/line.go",
  "\tswitch c := *d.Category; c {\n\tcase CategoryCyber, CategoryBio, CategoryFrontierLLM, CategoryReasoningExtraction, CategoryGeneralHarms:\n\t\treturn c\n\t}\n\treturn CategoryOther\n",
  "\treturn *d.Category\n",
  "TestRefusals_AreSplitByCategoryAndModel|TestContentNeverReachesTheOutput")
m("SP a null category is read as other", "internal/spend/line.go",
  "\tif d == nil || d.Category == nil {\n\t\treturn CategoryUncategorized", "\tif d == nil || d.Category == nil {\n\t\treturn CategoryOther",
  "TestRefusals_")
m("SP a zero-usage refusal is not split by category", "internal/spend/spend.go",
  "\t\t\t\trefusal(r).WithoutUsage++\n", "",
  "TestRefusals_")
m("SP refusals are not split by model", "internal/spend/spend.go",
  "\t\tk := [2]string{r.Category, refusalModel(r.Model)}", "\t\tk := [2]string{r.Category, \"other\"}",
  "TestRefusals_AreSplitByCategoryAndModel")
m("SP a refusal's model is printed as read", "internal/spend/spend.go",
  "\t\tk := [2]string{r.Category, refusalModel(r.Model)}", "\t\tk := [2]string{r.Category, r.Model}",
  "TestContentNeverReachesTheOutput")
m("SP the refusal categories are not printed", "internal/spend/text.go",
  "\tfor _, g := range r.ByCategory {\n\t\tvar p []string", "\tfor _, g := range r.ByCategory[:0] {\n\t\tvar p []string",
  "TestRefusals_")
m("SP a refusal's category reaches the output end to end", "internal/spend/line.go",
  "\tswitch c := *d.Category; c {\n\tcase CategoryCyber, CategoryBio, CategoryFrontierLLM, CategoryReasoningExtraction, CategoryGeneralHarms:\n\t\treturn c\n\t}\n\treturn CategoryOther\n",
  "\treturn *d.Category\n",
  "TestSpend_NoMessageTextReachesTheOutput")
m("SP a shared response's not-covered cost is counted in every unrecorded transcript", "internal/spend/join.go",
  "\t\tcostOf(&j.NotCoveredCost, r)\n", "\t\tfor _, m := range s.mainsOf(r) {\n\t\t\tif !covered[m] {\n\t\t\t\tcostOf(&j.NotCoveredCost, r)\n\t\t\t}\n\t\t}\n",
  "TestJoin_ASharedResponse")
m("SP a response a covered turn counted is also not covered", "internal/spend/join.go",
  "\t\tif !counted[r] {\n\t\t\tfor _, m := range s.mainsOf(r) {", "\t\tif true {\n\t\t\tfor _, m := range s.mainsOf(r) {",
  "TestJoin_ASharedResponseIsNeverBoth")
m("SP a shared response's session is the first sighting's", "internal/spend/scan.go",
  "\t\tr.owners = sc.ownerSessions(r)", "\t\tr.owners = []string{r.files[0].session}",
  "TestJoin_ADuplicatedResponse|TestJoin_ASharedResponseBelongs|TestJoin_ARecordedOriginal")
m("SP a transcript is tallied by the session of its first-seen response", "internal/spend/join.go",
  "\tfor m := range byTranscript {\n\t\tid := sessionOf[m]", "\tfor m, rs := range byTranscript {\n\t\tid := rs[0].files[0].session",
  "TestJoin_ADuplicatedResponse|TestJoin_ASharedResponse")
m("SP a refusal's synthetic line is counted again as a pre-output refusal", "internal/spend/scan.go",
  "\t\t\tif into := billed[key{r.file, r.requestID}]; into != nil {", "\t\t\tif into := billed[key{r.file, r.requestID}]; false && into != nil {",
  "TestRefusals_AMidStream")
m("SP a synthetic line is folded into a response in another file", "internal/spend/scan.go",
  "\t\tfor _, f := range r.files {\n\t\t\tbilled[key{f.idx, r.requestID}] = r", "\t\tfor range r.files {\n\t\t\tbilled[key{0, r.requestID}] = r",
  "TestRefusals_AMidStream")
m("SP a requestId is read whatever its shape", "internal/spend/scan.go",
  "\tif requestIDShaped(l.RequestID) {", "\tif l.RequestID != \"\" {", "TestRefusals_AMidStream")
m("SP a folded synthetic line does not make its response a refusal", "internal/spend/scan.go",
  "\t\t\t\tif into.StopReason != \"refusal\" {\n", "\t\t\t\tif false {\n", "TestRefusals_AMidStream")
m("SP a pre-output refusal with usage is priced into the total", "internal/spend/scan.go",
  "\t\tif r.StopReason == \"refusal\" && r.Tokens.Output == 0 && r.Tokens.Total() > 0 {", "\t\tif false {",
  "TestRefusals_APreOutputRefusalIsLeftOutOfTheTotal|TestRefusals_AreSplit")
m("SP the header does not say the total leaves out pre-output refusals", "internal/spend/text.go",
  "\tif line := preOutputLine(s.Refusals); line != \"\" {", "\tif line := preOutputLine(s.Refusals); false && line != \"\" {",
  "TestRefusals_")
m("SP a synthetic refusal group reads as a model named other", "internal/spend/spend.go",
  "\tif model == \"<synthetic>\" {\n\t\treturn ModelNotRecorded", "\tif false {\n\t\treturn ModelNotRecorded",
  "TestRefusals_")
m("SP the served model is read from message.model, not the fallback entry", "internal/spend/scan.go",
  "\tif served != \"\" {\n\t\tcand.Model = served", "\tif false && served != \"\" {\n\t\tcand.Model = served",
  "TestExtraAttempts_TheServedModelIsTheFallbackEntrys")
m("SP a chain where every model declined is reported as served", "internal/spend/spend.go",
  "\tif r.Fallback && r.StopReason != \"refusal\" {", "\tif r.Fallback {", "TestExtraAttempts_AnAllDeclinedChain")
m("SP an undecodable line ends the tie only when it holds the byte string user", "internal/report/transcript.go",
  "\t\tcase LineUndecodable:\n\t\t\t// A line whose header does not decode",
  "\t\tcase LineUndecodable:\n\t\t\tif !bytes.Contains(raw, []byte(`\"user\"`)) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\t// A line whose header does not decode",
  "TestFinalAssistantTexts_AnUndecodableLine")
m("SP an undecodable sidechain line ends the tie", "internal/report/transcript.go",
  "\tif topLevelSidechain(raw) {", "\tif false && topLevelSidechain(raw) {",
  "TestFinalAssistantTexts_AnUndecodableLine")
m("SP the bound does not say a sidechain user line keeps the tie", "internal/spend/join.go",
  "and a sidechain user line does not end the tie; ", "", "TestJoin_TheBoundNamesTheUnkeyedLinesThatKeepATie")

m("SP a refusal's category is not kept from the completed line", "internal/spend/scan.go",
  "\tprev.Category = cand.Category\n", "", "TestDedupe_TheCompletedLineWins")
m("SP a fallback's attempts are not kept from the completed line", "internal/spend/scan.go",
  "\tprev.Attempts, prev.Fallback = cand.Attempts, cand.Fallback\n", "",
  "TestDedupe_TheCompletedLineWins")
m("SP the served model is not kept from the completed line", "internal/spend/scan.go",
  "\tif cand.Model != \"\" && (prev.Model == \"\" || cand.Fallback) {", "\tif cand.Model != \"\" && prev.Model == \"\" {",
  "TestDedupe_TheCompletedLineWins")
m("SP an assistant line whose content does not decode leaves the earlier words standing", "internal/report/transcript.go",
  "\t\t\tif err != nil || line.Message.Role != \"assistant\" {\n\t\t\t\tunsay()\n", "\t\t\tif err != nil || line.Message.Role != \"assistant\" {\n",
  "TestFinalAssistantTexts_AnUnreadableLine")
m("SP a response is judged cold in a file it was copied into", "internal/spend/spend.go",
  "\t\t\tif rs[i].file != k.file {", "\t\t\tif false && rs[i].file != k.file {", "TestCacheExpiry_AResponseIsJudged")
m("SP an assistant line's content is decoded whole into a RawMessage again", "internal/report/transcript.go",
  "\t\t\tline, err := decodeAssistantLine(raw)\n\t\t\tif err != nil || line.Message.Role != \"assistant\" {\n\t\t\t\tunsay()\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\ttext, ok := line.Message.Content.text()",
  "\t\t\tvar line struct {\n\t\t\t\tMessage struct {\n\t\t\t\t\tRole    string          `json:\"role\"`\n\t\t\t\t\tContent json.RawMessage `json:\"content\"`\n\t\t\t\t} `json:\"message\"`\n\t\t\t}\n\t\t\tif json.Unmarshal(raw, &line) != nil || line.Message.Role != \"assistant\" {\n\t\t\t\tunsay()\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\ttext, ok := assistantText(line.Message.Content)",
  "TestAssistantLine_")
m("SP a non-firing turn's final words are printed to stderr", "internal/spend/join.go",
  "\t\tif !sf.Fires {\n\t\t\tcontinue\n\t\t}\n\t\tj.Turns++\n", "\t\tif !sf.Fires {\n\t\t\tfmt.Fprintln(os.Stderr, lastSaid(byFile, t.prompt))\n\t\t\tcontinue\n\t\t}\n\t\tj.Turns++\n",
  "TestSpend_NoMessageTextReachesTheOutput")
IMPORTS["SP a non-firing turn's final words are printed to stderr"] = ("internal/spend/join.go", '\t"encoding/json"\n', '\t"encoding/json"\n\t"fmt"\n')
m("SP a non-firing turn's final words reach the JSON", "internal/spend/join.go",
  "\t\tif !sf.Fires {\n\t\t\tcontinue\n\t\t}\n\t\tj.Turns++\n", "\t\tif !sf.Fires {\n\t\t\tj.Bound = lastSaid(byFile, t.prompt)\n\t\t\tcontinue\n\t\t}\n\t\tj.Turns++\n",
  "TestJoin_NoMessageTextReachesTheOutput")
m("SP cmdSpend reads a 30-day window whatever --days says", "cmd/rashomon/main.go",
  "\tfiles, err := spend.Discover(configDir, spend.WindowStart(now, days))", "\tfiles, err := spend.Discover(configDir, spend.WindowStart(now, 30))",
  "TestSpend_OldTranscriptsAreSkippedAndSaid")
m("SP the decoded message hands its bytes to its own decoder", "internal/spend/line.go",
  "\tUsage       *usage       `json:\"usage\"`\n}\n", "\tUsage       *usage       `json:\"usage\"`\n}\n\nfunc (m *message) UnmarshalJSON(b []byte) error {\n\ttype plain message\n\treturn json.Unmarshal(b, (*plain)(m))\n}\n",
  "TestContentHasNoFieldToLandIn")
IMPORTS["SP the decoded message hands its bytes to its own decoder"] = ("internal/spend/line.go", 'import "strings"\n', 'import (\n\t"encoding/json"\n\t"strings"\n)\n')

m("SP an unkeyed main-transcript sidechain user line ends the tie", "internal/report/transcript.go",
  "\t\t\t\ttie(head.Message.ID)\n\t\t\t}\n\t\t\tcontinue\n", "\t\t\t\ttie(head.Message.ID)\n\t\t\t}\n\t\t\tif head.Type == \"user\" && head.PromptID == \"\" {\n\t\t\t\tcurrent = \"\"\n\t\t\t}\n\t\t\tcontinue\n",
  "TestJoin_AMainTranscriptSidechain")
m("SP a main-transcript sidechain line's words are the turn's final word", "internal/report/transcript.go",
  "\t\t\tif head.Type == \"assistant\" && want[current] && head.Message.ID != \"\" {\n\t\t\t\ttie(head.Message.ID)\n\t\t\t}\n\t\t\tcontinue\n",
  "\t\t\tif head.Type == \"assistant\" && want[current] && head.Message.ID != \"\" {\n\t\t\t\ttie(head.Message.ID)\n\t\t\t}\n\t\t\tif head.Type == \"assistant\" && want[current] {\n\t\t\t\tif l, err := decodeAssistantLine(raw); err == nil {\n\t\t\t\t\tif text, ok := l.Message.Content.text(); ok {\n\t\t\t\t\t\ttf := out[current]\n\t\t\t\t\t\ttf.Said, tf.Text = true, text\n\t\t\t\t\t\tout[current] = tf\n\t\t\t\t\t}\n\t\t\t\t}\n\t\t\t}\n\t\t\tcontinue\n",
  "TestJoin_AMainTranscriptSidechain")

m("SP an extra attempt's tokens are not counted", "internal/spend/spend.go",
  "\t\te.Tokens.add(a)\n", "\t\t_ = a\n",
  "TestRefusalsAndExtraAttempts|TestExtraAttempts_")
m("SP an extra attempt's tokens are counted in the token total", "internal/spend/spend.go",
  "\ts.ExtraAttempts.add(r)\n", "\ts.ExtraAttempts.add(r)\n\tfor _, a := range r.Attempts {\n\t\ts.Tokens.add(a)\n\t}\n",
  "TestRefusalsAndExtraAttempts|TestExtraAttempts_")
m("SP a pre-output refusal is counted in the breakdowns", "internal/spend/spend.go",
  "\t\t\ts.refused = append(s.refused, r)\n\t\t\tcontinue\n", "\t\t\ts.refused = append(s.refused, r)\n",
  "TestRefusals_APreOutputRefusalIsLeftOutOfTheTotal")
m("SP an old transcript is counted once per spelling", "internal/spend/discover.go",
  "\t\t\tstale[resolved(p)] = true\n", "\t\t\tstale[p] = true\n",
  "TestDiscover_OneTranscriptUnderTwoSpellings")
m("SP a model-less fallback line replaces a known model", "internal/spend/scan.go",
  "\tif cand.Model != \"\" && (prev.Model == \"\" || cand.Fallback) {", "\tif prev.Model == \"\" || cand.Fallback {",
  "TestExtraAttempts_AModelLessFallbackLineKeepsAKnownModel")
m("SP a synthetic refusal line is folded after pre-output refusals are marked", "internal/spend/scan.go",
  "\tsc.foldRefusalMessages()\n\tfor _, r := range sc.Responses {", "\tdefer sc.foldRefusalMessages()\n\tfor _, r := range sc.Responses {",
  "TestRefusals_")
m("SP a cold write is priced on a response whose cost is unknown", "internal/spend/amounts.go",
  "\tif !ok || r.costUnknown {\n\t\treturn 0, tokens, false", "\tif !ok {\n\t\treturn 0, tokens, false",
  "TestCacheExpiry_")
m("SP a write on another model is judged against the previous model's cache", "internal/spend/spend.go",
  "\t\t\tif rs[i].Model != rs[i-1].Model || ", "\t\t\tif ",
  "TestCacheExpiry_")
m("SP a smaller prompt is judged as a re-write of the previous cache", "internal/spend/spend.go",
  " || cur.Input+cur.CacheRead+cur.CacheWrite5m+cur.CacheWrite1h < cached {", " {",
  "TestCacheExpiry_")
m("SP a cold 1h write ignores the 5m write already counted cold", "internal/spend/spend.go",
  "min(cur.CacheWrite1h, short-w.CacheWrite5m)", "min(cur.CacheWrite1h, short)",
  "TestCacheExpiry_")
m("SP a subagent transcript's lines are prefiltered on the byte string user again", "internal/spend/scan.go",
  "\t\tif !mayCarry && !f.Subagent && fileDated {", "\t\tif !mayCarry && (f.Subagent && !bytes.Contains(raw, []byte(`\"user\"`)) || !f.Subagent && fileDated) {",
  "TestJoin_ATurnsSpendIsKeyedByItsPrompt")
m("SP an undecodable line is a sidechain one by its bytes anywhere, not its top-level key", "internal/report/transcript.go",
  "\tif topLevelSidechain(raw) {", "\tif bytes.Contains(raw, []byte(`\"isSidechain\":true`)) {",
  "TestFinalAssistantTexts_AnUndecodableLine")
m("SP a whitespace-only line ends a main transcript's tie", "internal/report/transcript.go",
  "\tif len(bytes.TrimSpace(raw)) == 0 {", "\tif len(raw) == 0 {",
  "TestFinalAssistantTexts_AnUndecodableLine")
m("SP the bound does not say an undecodable sidechain line keeps the tie", "internal/spend/join.go",
  " (any in a subagent transcript; in the main transcript, unless it is a sidechain line), is tied", ", is tied",
  "TestJoin_TheBoundNamesTheUnkeyedLinesThatKeepATie")
m("SP a shared response's owner ignores its file's first dated line", "internal/spend/scan.go",
  "\t\tif ms := sc.firstMS[s.idx]; ms != 0 {", "\t\tif ms := sc.firstMS[s.idx]; false && ms != 0 {",
  "TestJoin_")
m("SP the store is read only for the sessions a response belongs to", "internal/spend/join.go",
  "\t\t\tfor _, f := range r.files {\n\t\t\t\tsessions[f.session] = true\n\t\t\t}", "\t\t\tfor _, id := range r.owners {\n\t\t\t\tsessions[id] = true\n\t\t\t}",
  "TestJoin_")
m("SP a file is dated by its literal first line", "internal/spend/scan.go",
  "\t\tif !fileDated {\n\t\t\tif ms, ok := parseTimestamp(l.Timestamp); ok {\n\t\t\t\tsc.firstMS[idx], fileDated = ms, true", "\t\tif !fileDated {\n\t\t\tfileDated = true\n\t\t\tif ms, ok := parseTimestamp(l.Timestamp); ok {\n\t\t\t\tsc.firstMS[idx] = ms",
  "TestJoin_")
m("SP a tie between two first-dated files goes to the lower session id", "internal/spend/scan.go",
  "\tsort.Strings(out)\n\treturn out\n}", "\tsort.Strings(out)\n\treturn out[:1]\n}",
  "TestJoin_")
m("SP a session holding a response it does not own has no row", "internal/spend/spend.go",
  "\t\tfor _, f := range r.files {\n\t\t\trow(f.session)\n\t\t}\n", "",
  "TestJoin_")
m("SP a row holding not-covered dollars reads recorded", "internal/spend/join.go",
  "\t\t\ttallyOf(id).out++", "\t\t\ttallyOf(id).in++",
  "TestJoin_")
m("SP a lost declaration's failed call is placed in a turn by recorded time", "internal/spend/join.go",
  "\t\t\tif x.Outcome == store.ExecFailed && !report.IsLookup(x.ToolName) {\n\t\t\t\tlost = append(lost, x.RecordedAtMS)\n\t\t\t}\n\t\t\tcontinue\n",
  "\t\t\tif x.Outcome != store.ExecFailed || report.IsLookup(x.ToolName) {\n\t\t\t\tcontinue\n\t\t\t}\n\t\t\tfor _, c := range byPrompt {\n\t\t\t\tif c.firstMS <= x.RecordedAtMS && (t == nil || c.firstMS > t.firstMS) {\n\t\t\t\t\tt = c\n\t\t\t\t}\n\t\t\t}\n\t\t\tif t == nil {\n\t\t\t\tlost = append(lost, x.RecordedAtMS)\n\t\t\t\tcontinue\n\t\t\t}\n",
  "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP a failed call whose declaration was lost is not counted", "internal/spend/join.go",
  "\t\t\t\tj.UndeclaredFailedCalls++", "\t\t\t\t_ = ms",
  "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP a lost declaration's failed call before the window is counted", "internal/spend/join.go",
  "\t\t\tif ms >= s.FromUnixMS {\n\t\t\t\tj.UndeclaredFailedCalls++", "\t\t\tif ms >= 0 {\n\t\t\t\tj.UndeclaredFailedCalls++",
  "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP a lost declaration's successful call is counted", "internal/spend/join.go",
  "\t\t\tif x.Outcome == store.ExecFailed && !report.IsLookup(x.ToolName) {", "\t\t\tif !report.IsLookup(x.ToolName) {",
  "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP a failed call whose declaration carried no prompt_id is not counted", "internal/spend/join.go",
  "\t\t\tif x.Outcome == store.ExecFailed && !report.IsLookup(x.ToolName) {",
  "\t\t\tif !slices.ContainsFunc(run.Declarations, func(d store.Declaration) bool { return d.ToolUseID == x.ToolUseID }) && x.Outcome == store.ExecFailed && !report.IsLookup(x.ToolName) {",
  "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP an undeclared failed call reads as a checked none", "internal/spend/text.go",
  "\tcase j.Turns == 0 && j.Unjudged == 0 && j.UndeclaredFailedCalls == 0:\n", "\tcase j.Turns == 0 && j.Unjudged == 0:\n",
  "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP the undeclared failed call is not said", "internal/spend/text.go",
  "\tif j.UndeclaredFailedCalls == 1 {", "\tif false {", "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP undeclared failed calls are not said", "internal/spend/text.go",
  "\t} else if j.UndeclaredFailedCalls > 1 {", "\t} else if false {", "TestJoin_AFailedCallWhoseDeclarationWasLost")
m("SP undeclared failed calls marshal as a checked 0 with nothing covered", "internal/spend/join.go",
  "\tif j.CoveredTranscripts > 0 {\n\t\tout.Turns, out.Unjudged, out.Undeclared, out.Cost = &j.Turns, &j.Unjudged, &j.UndeclaredFailedCalls, &j.Cost",
  "\tout.Undeclared = &j.UndeclaredFailedCalls\n\tif j.CoveredTranscripts > 0 {\n\t\tout.Turns, out.Unjudged, out.Cost = &j.Turns, &j.Unjudged, &j.Cost",
  "TestJoin_NoStore")
m("SP a session holding only pre-output refusals has no row", "internal/spend/spend.go",
  "\t\tfor _, f := range r.files {\n\t\t\trow(f.session)\n\t\t}\n\t\tif r.costUnknown {\n\t\t\ts.Refusals.BeforeOutput++\n\t\t\ts.Refusals.BeforeOutputTokens += r.Tokens.Total()\n\t\t\trefusal(r).BeforeOutput++\n\t\t\ts.refused = append(s.refused, r)\n\t\t\tcontinue\n\t\t}\n",
  "\t\tif r.costUnknown {\n\t\t\ts.Refusals.BeforeOutput++\n\t\t\ts.Refusals.BeforeOutputTokens += r.Tokens.Total()\n\t\t\trefusal(r).BeforeOutput++\n\t\t\ts.refused = append(s.refused, r)\n\t\t\tcontinue\n\t\t}\n\t\tfor _, f := range r.files {\n\t\t\trow(f.session)\n\t\t}\n",
  "TestSessions_ASessionHoldingOnlyPreOutputRefusals")
m("SP a pre-output refusal's transcript is not one of the window's", "internal/spend/join.go",
  "\tfor _, rs := range [][]*Response{s.window, s.refused} {", "\tfor _, rs := range [][]*Response{s.window} {",
  "TestSessions_ASessionHoldingOnlyPreOutputRefusals")
m("SP a session holding only pre-output refusals without usage has no row", "internal/spend/spend.go",
  "\t\t\t\trefusal(r).WithoutUsage++\n\t\t\t\tfor _, f := range r.files {\n\t\t\t\t\trow(f.session)\n\t\t\t\t}\n", "\t\t\t\trefusal(r).WithoutUsage++\n",
  "TestSessions_ASessionHoldingOnlyPreOutputRefusals|TestRefusals_APreOutputRefusal")
m("SP a pre-output refusal without usage is not one of the window's", "internal/spend/spend.go",
  "\t\t\t\ts.refused = append(s.refused, r)\n\t\t\t}\n\t\t\tcontinue\n", "\t\t\t}\n\t\t\tcontinue\n",
  "TestSessions_ASessionHoldingOnlyPreOutputRefusals|TestRefusals_APreOutputRefusal")
m("SP a response a covered turn counted marks every row holding it covered", "internal/spend/join.go",
  "\t\t\tif !slices.ContainsFunc(s.mainsOf(r), func(m string) bool { return !covered[m] }) {", "\t\t\tif true {",
  "TestJoin_")
m("SP a response whose transcripts were all recorded marks no row recorded", "internal/spend/join.go",
  "\t\t\tif !slices.ContainsFunc(s.mainsOf(r), func(m string) bool { return !covered[m] }) {", "\t\t\tif false {",
  "TestJoin_")
m("SP only an uncounted response marks the rows holding it recorded", "internal/spend/join.go",
  "\t\t\tif !slices.ContainsFunc(s.mainsOf(r), func(m string) bool { return !covered[m] }) {", "\t\t\tif !counted[r] {",
  "TestJoin_")
m("SP a not-covered transcript names its file's own session", "internal/spend/join.go",
  "\t\t\tj.NotCoveredTranscripts++\n\t\t\tt.out++\n", "\t\t\tj.NotCoveredTranscripts++\n\t\t\tt.out++\n\t\t\tnamed[displaySession(id)] = true\n",
  "TestJoin_")
m("SP a row with no tally has no coverage label", "internal/spend/join.go",
  "\t\tcase t == nil:\n\t\t\ts.PerSession[i].Coverage = CoverageNotRecorded\n", "\t\tcase t == nil:\n",
  "TestJoin_")
m("SP the pre-output refusals' rule is not in the JSON", "internal/spend/spend.go",
  "Refusals{BeforeOutputPricing: PreOutputRefusalPricing, ByCategory", "Refusals{ByCategory",
  "TestRefusalsAndExtraAttempts_")
m("SP the extra attempts' rule is not in the JSON", "internal/spend/spend.go",
  "\t\tExtraAttempts: ExtraAttempts{Pricing: ExtraAttemptsPricing},\n", "",
  "TestRefusalsAndExtraAttempts_")
m("SP the pre-output refusals' rule does not say they are out of the total", "internal/spend/spend.go",
  "\"tokens only, cost unknown, out of total and tokens: whether", "\"tokens only, cost unknown: whether",
  "TestRefusalsAndExtraAttempts_")
m("SP a blank subagent line ends its tie", "internal/spend/scan.go",
  "\t\tcase report.LineBlank:\n\t\t\tcontinue\n\t\tcase report.LineUndecodable, report.LineUndecodableSidechain:",
  "\t\tcase report.LineBlank, report.LineUndecodable, report.LineUndecodableSidechain:",
  "TestJoin_ATurnsSpendIsKeyedByItsPrompt")
m("SP a pre-output refusal's extra attempts are left out of the header", "internal/spend/spend.go",
  "\t\ts.ExtraAttempts.add(r)\n", "\t\tif !r.costUnknown {\n\t\t\ts.ExtraAttempts.add(r)\n\t\t}\n",
  "TestExtraAttempts_AnAllDeclinedChainIsNotServed")
m("SP the combined pre-output header drops the refusals written without usage", "internal/spend/text.go",
  "\tif r.WithoutUsage > 0 {\n\t\tparts = append(parts, countOf(r.WithoutUsage, \"pre-output refusal\")+\" written without usage\")",
  "\tif r.WithoutUsage > 0 && len(parts) == 0 {\n\t\tparts = append(parts, countOf(r.WithoutUsage, \"pre-output refusal\")+\" written without usage\")",
  "TestRefusals_AreSplitByCategoryAndModel")
m("SP only pre-output refusals with usage print no refusals line", "internal/spend/text.go",
  "\t} else if s.Refusals.WithoutUsage > 0 || s.Refusals.BeforeOutput > 0 {", "\t} else if s.Refusals.WithoutUsage > 0 {",
  "TestRefusals_APreOutputRefusalIsLeftOutOfTheTotal")
m("SP the cache label says the figure errs low", "internal/spend/spend.go",
  "; it skips a model switch and any request smaller than the previous cache, so it misses some true expiries, and can still count new content in a request that grew past the previous cache\"",
  "; it errs low\"",
  "TestCacheExpiry_TheLabel")
m("SP a response held by tied sessions is not counted as shared", "internal/spend/spend.go",
  "\t\t\ts.SharedResponses++\n", "",
  "TestJoin_ARecordedOriginalWithAnUnrecordedCopy")
m("SP a response held by one session is counted as shared", "internal/spend/spend.go",
  "\t\tif len(r.owners) > 1 {\n\t\t\ts.SharedResponses++", "\t\tif len(r.owners) > 0 {\n\t\t\ts.SharedResponses++",
  "TestJoin_ASharedResponseBelongsToTheFileFirstDated")
m("SP the rows' shared responses are not said", "internal/spend/text.go",
  "\t\tif n := s.SharedResponses; n > 0 {", "\t\tif n := s.SharedResponses; n < 0 {",
  "TestJoin_ARecordedOriginalWithAnUnrecordedCopy")
m("SP every shared response after the first is not counted", "internal/spend/spend.go",
  "\t\t\ts.SharedResponses++\n", "\t\t\ts.SharedResponses = 1\n",
  "TestJoin_")
m("SP several shared responses read as one that appears", "internal/spend/text.go",
  "\treturn \"appear\"\n}", "\treturn \"appears\"\n}",
  "TestJoin_")
m("SP an unreadable entry is keyed as written, not resolved", "internal/spend/discover.go",
  "\t\t\t\tunreadable[resolved(path)] = true", "\t\t\t\tunreadable[path] = true",
  "TestDiscover_OneTranscriptUnderTwoSpellings")
m("SP a path that does not resolve is keyed as written", "internal/spend/discover.go",
  "\treturn filepath.Join(resolved(dir), filepath.Base(p))", "\treturn p",
  "TestDiscover_OneTranscriptUnderTwoSpellings")
m("SP a transcript that does not resolve is deduped as written", "internal/spend/discover.go",
  "\t\treal := resolved(f.Path)\n", "\t\treal, err := filepath.EvalSymlinks(f.Path)\n\t\tif err != nil {\n\t\t\treal = f.Path\n\t\t}\n",
  "TestDiscover_OneTranscriptUnderTwoSpellings")
m("SP an undated usage line re-opens its file's dating", "internal/spend/scan.go",
  "\t\tstartMS, dated := parseTimestamp(l.Timestamp)\n", "\t\tstartMS, dated := parseTimestamp(l.Timestamp)\n\t\tfileDated = dated\n",
  "TestJoin_AnUndatedUsageLineDoesNotReDateItsFile")
# A session watched with no call made (Join, noCallStretches, allWatched,
# mayHaveCalled):
# one break per guard of the rule, and the rule itself.
m("SP a session watched with no call made is never covered", "internal/spend/join.go",
  "\t\tif stretches := noCallStretches(run); len(stretches) > 0 {\n", "\t\tif stretches := noCallStretches(run); false {\n",
  "TestJoin_AWatchedSession|TestSpend_AWatchedSession")
m("SP a run holding a call's records covers by its watched stretches", "internal/spend/join.go",
  "\tif len(run.Declarations)+len(run.Executions)+len(run.Terminals) > 0 || run.Skipped > 0 {", "\tif run.Skipped > 0 {",
  "TestJoin_AWatchedSession")
m("SP a run with a record that did not parse covers by its watched stretches", "internal/spend/join.go",
  "\tif len(run.Declarations)+len(run.Executions)+len(run.Terminals) > 0 || run.Skipped > 0 {",
  "\tif len(run.Declarations)+len(run.Executions)+len(run.Terminals) > 0 {",
  "TestJoin_AWatchedSession")
m("SP a call or post coverage record does not stop a run's watched stretches", "internal/spend/join.go",
  "\t\tdefault:\n\t\t\treturn nil\n", "",
  "TestJoin_AWatchedSession|TestSpend_AWatchedSession")
m("SP an unverified coverage record does not stop a run's watched stretches", "internal/spend/join.go",
  "\t\tif c.State != store.StateVerified {\n\t\t\treturn nil\n\t\t}\n", "",
  "TestJoin_AWatchedSession")
m("SP an end with no start before it closes a watched stretch", "internal/spend/join.go",
  "\topen := false\n", "\topen := true\n",
  "TestJoin_AWatchedSession")
m("SP a start with no end yet watches what follows it", "internal/spend/join.go",
  "\treturn stretches\n}", "\tif open {\n\t\tstretches = append(stretches, [2]int64{from, math.MaxInt64})\n\t}\n\treturn stretches\n}",
  "TestJoin_AWatchedSession|TestSpend_AWatchedSession")
m("SP a later start does not begin the watched stretch again", "internal/spend/join.go",
  "\t\t\tfrom, open = c.RecordedAtMS, true\n", "\t\t\tif !open {\n\t\t\t\tfrom = c.RecordedAtMS\n\t\t\t}\n\t\t\topen = true\n",
  "TestJoin_AWatchedSession")
m("SP a response before a watched stretch is watched", "internal/spend/join.go",
  "w[0] <= r.StartMS", "true",
  "TestJoin_AWatchedSession")
m("SP a response after a watched stretch is watched", "internal/spend/join.go",
  "r.StartMS <= w[1]", "true",
  "TestJoin_AWatchedSession")
m("SP a response at a start record's millisecond is not watched", "internal/spend/join.go",
  "w[0] <= r.StartMS", "w[0] < r.StartMS",
  "TestJoin_AWatchedSession")
m("SP a response at an end record's millisecond is not watched", "internal/spend/join.go",
  "r.StartMS <= w[1]", "r.StartMS < w[1]",
  "TestJoin_AWatchedSession")
m("SP a run's watched stretches are one from its first start to its last end", "internal/spend/join.go",
  "!slices.ContainsFunc(stretches, func(w [2]int64) bool {\n\t\t\treturn w[0] <= r.StartMS && r.StartMS <= w[1]\n\t\t})",
  "!(stretches[0][0] <= r.StartMS && r.StartMS <= stretches[len(stretches)-1][1])",
  "TestJoin_AWatchedSession")
m("SP a response that made a call is watched", "internal/spend/join.go",
  "r.StopReason == \"tool_use\" || ", "",
  "TestJoin_AWatchedSession")
m("SP a response with no stop_reason is watched", "internal/spend/join.go",
  "!r.complete || ", "",
  "TestJoin_AWatchedSession")
m("SP a subagent's response is watched", "internal/spend/join.go",
  " || r.Subagent {", " {",
  "TestJoin_AWatchedSession")
m("SP a run's watched stretches cover another session's transcript", "internal/spend/join.go",
  "f.Session == id && ", "",
  "TestJoin_AWatchedSessionWithNoCallIsRecorded")
# Each trace of a call alone keeps a run from vouching (noCallStretches).
m("SP a run's declarations do not stop its watched stretches", "internal/spend/join.go",
  "len(run.Declarations)+len(run.Executions)+len(run.Terminals) > 0", "len(run.Executions)+len(run.Terminals) > 0",
  "TestJoin_AWatchedSession")
m("SP a run's executions do not stop its watched stretches", "internal/spend/join.go",
  "len(run.Declarations)+len(run.Executions)+len(run.Terminals) > 0", "len(run.Declarations)+len(run.Terminals) > 0",
  "TestJoin_AWatchedSession")
m("SP a run's terminals do not stop its watched stretches", "internal/spend/join.go",
  "len(run.Declarations)+len(run.Executions)+len(run.Terminals) > 0", "len(run.Declarations)+len(run.Executions) > 0",
  "TestJoin_AWatchedSession")
# The call guards read every response the scan read, and every file whole
# (mayHaveCalled, Scan.partial): a turn can start before the window and end
# inside it, and a response missing from the scan may be the call.
m("SP a transcript that shows a call is covered by its watched stretches", "internal/spend/join.go",
  "\tmayCall := s.mayHaveCalled()\n", "\tmayCall := map[string]bool{}\n",
  "TestJoin_AWatchedSession")
m("SP a response before the window is not checked for a call", "internal/spend/join.go",
  "\tfor _, r := range s.scan.Responses {\n", "\tfor _, r := range s.scan.Responses {\n\t\tif r.StartMS < s.FromUnixMS {\n\t\t\tcontinue\n\t\t}\n",
  "TestJoin_AWatchedSession")
m("SP an undated response is not checked for a call", "internal/spend/join.go",
  "\tfor _, r := range s.scan.Responses {\n", "\tfor _, r := range s.scan.Responses {\n\t\tif r.StartMS == 0 {\n\t\t\tcontinue\n\t\t}\n",
  "TestJoin_AWatchedSession")
m("SP a transcript not read whole is taken for one read whole", "internal/spend/join.go",
  "\tfor i, p := range s.scan.partial {\n\t\tif p {\n\t\t\tout[s.scan.Files[i].Main] = true\n\t\t}\n\t}\n", "",
  "TestJoin_AWatchedSession")
m("SP a file not read to the end is marked read whole", "internal/spend/scan.go",
  "\t\t\tsc.Unreadable++\n\t\t\tsc.partial[i] = true\n", "\t\t\tsc.Unreadable++\n",
  "TestJoin_AWatchedSession")
m("SP a usage line that could not be counted leaves its file marked read whole", "internal/spend/scan.go",
  "\t\tif sc.Unparsed > unparsed {\n", "\t\tif sc.Unparsed < unparsed {\n",
  "TestJoin_AWatchedSession")
