# MD-1: a name, an item number, half a word or a table's header is no claim
# (card MISREAD-DONE1): a number is a count only in its predicate's clause and
# never after '#'; a done predicate in a run of capitals in a sentence that is
# not is a name, and a predicate read with its subject is not; a predicate
# joined by a hyphen to a word rune is part of another word unless a form of
# be stands right before it; a line with a bar outside backticks over a
# delimiter row names columns. Each break removes one reading and names the
# tests that hold it.
C = "internal/verify/claims.go"
T = "internal/verify/claimtext.go"
m("MD-1 a number from another clause is a count", C,
  "\t\tif r.number && isNumber(ws[s]) && cs[s] == cs[p] && ms[s]&markID == 0 {\n",
  "\t\tif r.number && isNumber(ws[s]) && ms[s]&markID == 0 {\n", "TestMD1a_")
m("MD-1 an id is a count", C,
  "\t\tif r.number && isNumber(ws[s]) && cs[s] == cs[p] && ms[s]&markID == 0 {\n",
  "\t\tif r.number && isNumber(ws[s]) && cs[s] == cs[p] {\n", "TestMD1a_")
m("MD-1 no word is marked an id", T,
  "\t\t\tif i > 0 && s[i-1] == '#' {\n", "\t\t\tif false {\n", "TestMD1a_")
# Each break on the capitals reading keeps p+i in use: one that drops the
# loop's only use of i does not build, and a row that does not build is no
# kill. With named the reading's last test, a run of capitals read as no name
# is this first break.
m("MD-1 a predicate in capitals is a claim", C,
  "\t\tif m&markJoined != 0 && !s.predicative(p) || bare && m&markUpper != 0 && !s.loud && s.named(p+i) {\n",
  "\t\tif m&markJoined != 0 && !s.predicative(p) || false && s.named(p+i) {\n", "TestMD1b_")
m("MD-1 a predicate in capitals is a name beside its subject", C,
  "|| bare && m&markUpper != 0 && !s.loud && s.named(p+i) {\n", "|| m&markUpper != 0 && !s.loud && s.named(p+i) {\n", "TestMD1b_")
# Round 5 (QA-3): one predicate in capitals is emphasis; a run of capitals of
# two letters or more, in the clause or across or, is a name.
m("MD-1 one predicate in capitals is a name", C, " && !s.loud && s.named(p+i) {\n", " && !s.loud && (s.named(p+i) || true) {\n", "TestMD1b_")
m("MD-1 or makes no run", C, ' || (d == -2 || d == 2) && s.words[k+d/2] != "or" {\n', " || (d == -2 || d == 2) {\n", "TestMD1b_")
m("MD-1 any word between makes a run", C, ' || (d == -2 || d == 2) && s.words[k+d/2] != "or" {\n', " {\n", "TestMD1b_")
m("MD-1 a run crosses its clause", C, " || s.clauses[j] != s.clauses[k] || (d", " || (d", "TestMD1b_")
m("MD-1 one letter makes a run", C,
  "\t\tif s.marks[j]&markUpper != 0 && utf8.RuneCountInString(s.words[j]) > 1 {\n",
  "\t\tif s.marks[j]&markUpper != 0 && utf8.RuneCountInString(s.words[j]) > 0 {\n", "TestMD1b_")
m("MD-1 every rule reads bare", C,
  "!s.misspelt(p, n, r.class == ClaimDone)", "!s.misspelt(p, n, true)", "TestMD1b_")
# The break starts the sentence quiet, so the loop's `s.loud && ...` keeps it
# so; breaking the loop's body instead leaves its m unused, which does not build.
m("MD-1 a sentence in capitals throughout loses its claim", C,
  "\ts.loud = true\n\tfor _, m := range s.marks {\n", "\ts.loud = false\n\tfor _, m := range s.marks {\n", "TestMD1b_")
m("MD-1 a word's case is read from its first letter", T,
  "\t\t\tupper, lower = upper || unicode.IsUpper(r), lower || unicode.IsLower(r)\n", "", "TestMD1b_")
m("MD-1 a hyphened predicate is a predicate", T,
  "\t\t\tif joined(s, i, j) {\n", "\t\t\tif false {\n", "TestMD1c_")
m("MD-1 a hyphen before the word joins nothing", T,
  "\tif i >= 2 && s[i-1] == '-' {\n", "\tif false {\n", "TestMD1c_")
m("MD-1 a hyphen after the word joins nothing", T,
  "\tif j+1 < len(s) && s[j] == '-' {\n", "\tif false {\n", "TestMD1c_")
m("MD-1 a hyphen joins whatever follows it", T,
  "\t\tif r, _ := utf8.DecodeRuneInString(s[j+1:]); wordRune(r) {\n", "\t\tif true {\n", "TestMD1c_")
# Round 5 (QA-3): a hyphened predicate right after a form of be, past any
# adverb, is predicative and claims.
m("MD-1 a predicative compound is half a word", C,
  "\t\tif m&markJoined != 0 && !s.predicative(p) || bare", "\t\tif m&markJoined != 0 || bare", "TestMD1c_")
m("MD-1 every hyphened predicate is predicative", C,
  "\t\tif m&markJoined != 0 && !s.predicative(p) || bare", "\t\tif false || bare", "TestMD1c_")
m("MD-1 an adverb after be ends the reading", C,
  "\tfor k >= 0 && slices.Contains(claimAdverbs[:], s.words[k]) {\n\t\tk--\n\t}\n", "", "TestMD1c_")
m("MD-1 any word before is be", C,
  "\treturn k >= 0 && slices.Contains(claimBe[:], s.words[k])\n", "\treturn k >= 0\n", "TestMD1c_")
m("MD-1 a table's header is read", T,
  "isDelivered(lead, delivered), tableHeader(lead, b):\n", "isDelivered(lead, delivered):\n", "TestMD1d_")
m("MD-1 a header needs no delimiter row under it", T,
  "\treturn delimiterRow(rest)\n", "\treturn true\n", "TestMD1d_")
m("MD-1 a line with no bar heads a table", T,
  "\tif !cellBar(line) {\n\t\treturn false\n\t}\n", "", "TestMD1d_")
# Round 5 (QA-3): a bar inside backticks is a pipe, not a cell.
m("MD-1 a bar inside backticks is a cell", T, "\t\tline = line[i+1+end+1:]\n", "\t\tline = line[i+1:]\n", "TestMD1d_")
m("MD-1 a code span hides the bars after it", T, "\t\tline = line[i+1+end+1:]\n", "\t\treturn false\n", "TestMD1d_")
m("MD-1 an unclosed backtick hides the bars after it", T,
  "\t\t\treturn bytes.IndexByte(line[i+1:], '|') >= 0\n", "\t\t\treturn false\n", "TestMD1d_")
m("MD-1 a delimiter cell may hold words", T,
  "\t\tif len(cell) == 0 || len(bytes.Trim(cell, \"-\")) != 0 {\n", "\t\tif len(cell) == 0 {\n", "TestMD1d_")
m("MD-1 an empty delimiter cell counts", T,
  "\t\tif len(cell) == 0 || len(bytes.Trim(cell, \"-\")) != 0 {\n", "\t\tif len(bytes.Trim(cell, \"-\")) != 0 {\n", "TestMD1d_")
m("MD-1 an aligned delimiter cell is none", T,
  "\t\tcell = bytes.TrimSuffix(bytes.TrimPrefix(bytes.TrimSpace(cell), []byte(\":\")), []byte(\":\"))\n",
  "\t\tcell = bytes.TrimSpace(cell)\n", "TestMD1d_")
m("MD-1 a closing bar is a cell", T,
  "\tl = bytes.TrimSuffix(bytes.TrimPrefix(l, []byte(\"|\")), []byte(\"|\"))\n", "\tl = bytes.TrimPrefix(l, []byte(\"|\"))\n", "TestMD1d_")
# How a thing works (works from, through, by, off, as a) and a plan (then, next,
# after that, once; i or we; a verb in the present) are no claim.
L = "internal/verify/claimlex.go"
m("MD-1 how a thing works is a claim", C,
  "\tfor _, mn := range claimManners {\n\t\tif phraseAt(ws, p, mn) {\n\t\t\treturn false // how a thing works, not that it does (card MISREAD-DONE1)\n\t\t}\n\t}\n",
  "", "TestMD1e_")
m("MD-1 as with no article is a manner", L,
  '{"works", "as", "a"}, {"works", "as", "an"},\n', '{"works", "as"},\n', "TestMD1e_")
m("MD-1 working says nothing of how", L,
  '\t{"working", "from"}, {"working", "through"}, {"working", "by"}, {"working", "off"}, {"working", "as", "a"}, {"working", "as", "an"},\n',
  "", "TestMD1e_")
m("MD-1 a plan is a claim", C, "\treturn planned(ws)\n", "\treturn false\n", "TestMD1f_")
m("MD-1 after that opens no plan", L, '{"next"}, {"after", "that"}, {"once"}}', '{"next"}, {"once"}}', "TestMD1f_")
m("MD-1 a plan's opening needs no i or we", C,
  '\t\tif k == len(ws) || ws[k] != "i" && ws[k] != "we" {\n', "\t\tif k == len(ws) {\n", "TestMD1f_")
m("MD-1 an adverb is a plan's verb", C,
  "\t\tfor k < len(ws) && slices.Contains(claimAdverbs[:], ws[k]) {\n\t\t\tk++\n\t\t}\n", "", "TestMD1f_")
m("MD-1 a past verb in ed opens a plan", C,
  'k < len(ws) && !strings.HasSuffix(ws[k], "ed") && !slices', "k < len(ws) && !slices", "TestMD1f_")
m("MD-1 an irregular past verb opens a plan", C, " && !slices.Contains(claimPast[:], ws[k])", "", "TestMD1f_")
m("MD-1 be or have opens a plan", C, " &&\n\t\t\t!slices.Contains(claimAuxiliaries[:], ws[k])", "", "TestMD1f_")
# Round 5 (QA-3): a verb of perception after then, next or after that reports;
# after once it stays a condition.
m("MD-1 a verb of perception opens a plan", C,
  ' && (op[0] == "once" || !slices.Contains(claimPerceptions[:], ws[k]))', "", "TestMD1f_")
m("MD-1 once reports a perception", C,
  ' && (op[0] == "once" || !slices.Contains(claimPerceptions[:], ws[k]))', " && !slices.Contains(claimPerceptions[:], ws[k])", "TestMD1f_")
m("MD-1 notice is no verb of perception", L, '{"see", "find", "notice"}', '{"see", "find"}', "TestMD1f_")
