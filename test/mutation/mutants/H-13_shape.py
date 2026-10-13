m("H-13 program carries the whole command line", "internal/shape/shape.go", "prog := path.Base(pshaped[i].text)", "prog := cmd + path.Base(pshaped[i].text[:0])", "TestH13_")
m("the report counts an unknown program among the real ones", "internal/report/report.go",
  "\t\tcase d.ToolName == \"Bash\":", "\t\tcase false:", "TestProgramsUnknownIsCountedApart|TestNonShell")
m("the report stops counting programs at all", "internal/report/report.go",
  "\t\t\tsess.Declarations.ByProgram[*d.Shape.Program]++", "\t\t\t_ = d.Shape.Program",
  "TestByProgramAndVerb")
m("H-13 a word the tokenizer cannot vouch for is read as the program", "internal/shape/shape.go",
  "\t\tcase t.opaque, isComment(t):", "\t\tcase isComment(t):", "TestNoCorpusLineLeaksIntoProgram")
m("H-13 the tokenizer never marks an expansion opaque", "internal/shape/tokenize.go",
  "\t\tdefault:\n\t\t\tif expands(i, false) {\n\t\t\t\tw.opaque = true\n\t\t\t}",
  "\t\tdefault:\n\t\t\tif expands(i, false) {\n\t\t\t\t_ = w.opaque\n\t\t\t}", "TestNoCorpusLineLeaksIntoProgram")
m("H-13 backslash-newlines are not joined before the program search", "internal/shape/shape.go",
  "\tif !strings.Contains(s, \"\\\\\\n\") {\n\t\treturn s, nil\n\t}", "\tif true {\n\t\treturn s, nil\n\t}",
  "TestNoCorpusLineLeaksIntoProgram|TestTokenizeProgram")
m("H-13 a paren where a redirect target belongs is read as a subshell", "internal/shape/shape.go",
  "\tif toks[i+1].meta {\n\t\t// A paren where the target belongs",
  "\tif false && toks[i+1].meta {\n\t\t// A paren where the target belongs",
  "TestNoCorpusLineLeaksIntoProgram")
m("H-13 a word quoted before its = is read as an assignment", "internal/shape/shape.go",
  "\treturn t.quotedAt < 0 || t.quotedAt > eq", "\treturn true", "TestProgramIsAProgram|TestNoCorpusLineLeaksIntoProgram")
m("H-13 a quoted command line with spaces is recorded as the program", "internal/shape/shape.go",
  "\t\tcase c < 0x21 || c > 0x7e:", "\t\tcase c < 0x20 || c > 0x7e:", "TestProgramIsAProgram")
m("H-13 program is whatever token came first, operator or not", "internal/shape/shape.go",
  "\t\tt := toks[i]\n\t\tif t.meta {",
  "\t\tt := toks[i]\n\t\tif false && t.meta {", "TestProgramIsAProgram")
m("H-13 a split redirect operator's second half is taken for its target", "internal/shape/shape.go",
  "toks[i+1].meta && toks[i+1].glued && continuesRedirect(op, toks[i+1].text); n++ {", "toks[i+1].meta && op == \"\"; n++ {",
  "TestH13_CanaryNeverReachesDisk|TestProgramIsAProgram")
m("H-13 a redirect skips every operator before its target", "internal/shape/shape.go",
  "\tfor n := 0; n < pieces && i+1 < len(toks) && toks[i+1].meta && toks[i+1].glued && continuesRedirect(op, toks[i+1].text); n++ {\n\t\ti++\n\t}",
  "\tfor i+1 < len(toks) && toks[i+1].meta {\n\t\ti++\n\t}\n\t_, _ = op, pieces",
  "TestH13_CanaryNeverReachesDisk")
# The program search: one break per rule, each judged by the rows and corpus
# lines that rule alone closes -- the matrix in shape_program_test.go carries a
# row per glob character, per separator, per glue check and per case of an
# unknowable group depth, so that no rule rides on another.
PROG = "TestProgramIsAProgram|TestNoCorpusLineLeaksIntoProgram"
m("H-13 a redirect's pieces are joined across a blank or a newline", "internal/shape/shape.go",
  "toks[i+1].meta && toks[i+1].glued && continuesRedirect(op, toks[i+1].text)", "toks[i+1].meta && continuesRedirect(op, toks[i+1].text)", PROG)
m("H-13 an operator where a redirect target belongs is read past to the next command", "internal/shape/shape.go",
  "\treturn !procSub(toks, i+1)\n}", "\treturn false\n}", PROG)
m("H-13 the tokenizer never marks an operator glued", "internal/shape/tokenize.go",
  "nlBefore: sawNL, glued: !gap && len(toks) > 0})", "nlBefore: sawNL, glued: false})", "TestTokenizeGlued|TestProgramIsAProgram")
m("H-13 the tokenizer never marks a word glued", "internal/shape/tokenize.go",
  "\t\t\tw.glued = !gap && len(toks) > 0\n", "\t\t\tw.glued = false\n", "TestTokenizeGlued|TestProgramIsAProgram")
m("H-13 a byte the shell does not split on is read as part of a program name", "internal/shape/shape.go",
  "\t\tcase c < 0x21 || c > 0x7e:", "\t\tcase c == ' ' || c == '\\t' || c == '\\n' || c == '\\r':", PROG)
m("H-13 a command word holding $ is named", "internal/shape/shape.go",
  "\t\tcase c == '$':", "\t\tcase false && c == '$':", PROG)
m("H-13 a glob command word is named", "internal/shape/shape.go",
  "\t\tcase c == '*' || c == '?':", "\t\tcase false && (c == '*' || c == '?'):", PROG)
m("H-13 zsh's ~ exclusion is read as a path", "internal/shape/shape.go",
  "\t\tcase c == '~' && j > 0:", "\t\tcase false && c == '~' && j > 0:", PROG)
m("H-13 an all-digit command word is named", "internal/shape/shape.go",
  "\treturn !digits\n}", "\treturn true\n}", PROG)
m("H-13 a word glued to <( or a numeric glob is named by its front", "internal/shape/shape.go",
  "\treturn next.meta && next.text == \"(\" || op == \"<\" && isNumericGlob(next)",
  "\treturn false && next.meta && op == \"\"", PROG)
m("H-13 a directory is named by its last component", "internal/shape/shape.go",
  "\tcase strings.HasSuffix(s, \"/\"):", "\tcase false && strings.HasSuffix(s, \"/\"):", PROG)
m("H-13 a control byte in the line is read past", "internal/shape/shape.go",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\t' && c != '\\n' || c == 0x7f {", "\t\tif c := cmd[i]; c == '\\r' {", PROG)
m("H-13 the function-definition scan stops at a redirection", "internal/shape/shape.go",
  "\t\tcase isRedirect(t.text):\n\t\t\tend := operatorEnd(toks, j)\n",
  "\t\tcase isRedirect(t.text):\n\t\t\tif true {\n\t\t\t\treturn -1, true\n\t\t\t}\n\t\t\tend := operatorEnd(toks, j)\n", PROG)
m("H-13 the function-definition scan reads &> as the background separator", "internal/shape/shape.go",
  "\t\tcase t.text == \"&\" && j+1 < len(toks)", "\t\tcase false && t.text == \"&\" && j+1 < len(toks)", PROG)
m("H-13 the function-definition scan stops at a paren that does not close at once", "internal/shape/shape.go",
  "\t\t\tif j+1 < len(toks) && toks[j+1].meta && toks[j+1].text == \")\" {\n\t\t\t\treturn 0, false\n\t\t\t}\n",
  "\t\t\tif j+1 < len(toks) && toks[j+1].meta && toks[j+1].text == \")\" {\n\t\t\t\treturn 0, false\n\t\t\t}\n\t\t\treturn -1, true\n", PROG)
m("H-13 the function-definition scan runs on past a newline", "internal/shape/shape.go",
  "\t\tcase t.nlBefore:\n\t\t\treturn -1, true\n", "\t\tcase false && t.nlBefore:\n\t\t\treturn -1, true\n", "TestProgramIsAProgram")
SEPS = ["\";\"", "\"&&\"", "\"||\"", "\"|\"", "\"&\""]
_seps = "\t\tcase " + " || ".join("t.text == " + x for x in SEPS) + ":\n\t\t\treturn j, true"
for _drop in SEPS:
    m("H-13 the function-definition scan reads on past " + _drop.strip('"'), "internal/shape/shape.go",
      _seps, "\t\tcase " + " || ".join("t.text == " + x for x in SEPS if x != _drop) + ":\n\t\t\treturn j, true", PROG)
m("H-13 the function-definition scan does not skip a paren group", "internal/shape/shape.go",
  "\t\t\t}\n\t\t\topen = append(open, '(')\n", "\t\t\t}\n", PROG)
m("H-13 the function-definition scan does not skip backticks", "internal/shape/shape.go",
  "\t\tcase t.ticks%2 == 1:\n\t\t\topen = append(open, '`')\n", "\t\tcase false:\n\t\t\topen = append(open, '`')\n", PROG)
m("H-13 a paren inside a group does not nest", "internal/shape/shape.go",
  "\tcase t.text == \"(\":\n\t\treturn append(open, '(')\n", "\tcase t.text == \"(\":\n", PROG)
m("H-13 a paren group never closes", "internal/shape/shape.go",
  "\tcase t.text == \")\":\n\t\treturn open[:len(open)-1]\n", "\tcase t.text == \")\":\n", PROG)
m("H-13 backticks never close", "internal/shape/shape.go",
  "\tcase t.ticks%2 == 1 && top == '`':\n\t\treturn open[:len(open)-1]\n", "\tcase false:\n\t\treturn open[:len(open)-1]\n", PROG)
m("H-13 a group that never closes is read as closed", "internal/shape/shape.go",
  "\tif len(open) > 0 || uncertain {\n\t\treturn 0, false\n\t}", "\tif uncertain {\n\t\treturn 0, false\n\t}", PROG)
m("H-13 the function-definition scan reads past where the lexer's reading stopped", "internal/shape/shape.go",
  "\tif len(open) > 0 || uncertain {\n\t\treturn 0, false\n\t}", "\tif len(open) > 0 {\n\t\treturn 0, false\n\t}", PROG)
m("H-13 a ${ the word does not close is read past", "internal/shape/shape.go",
  "\t\tif openBrace(t) {\n\t\t\treturn 0, false\n\t\t}", "\t\tif false && openBrace(t) {\n\t\t\treturn 0, false\n\t\t}", PROG)
m("H-13 a closed ${ } is taken for an open one", "internal/shape/shape.go",
  "strings.Count(t.text[k:], \"{\") > strings.Count(t.text[k:], \"}\")",
  "strings.Count(t.text[k:], \"{\") >= strings.Count(t.text[k:], \"}\")", "TestProgramIsAProgram")
m("H-13 a quoted ${ is taken for an expansion", "internal/shape/shape.go",
  "\treturn t.opaque && k >= 0 &&", "\treturn k >= 0 &&", "TestProgramIsAProgram")
m("H-13 a comment inside a group is read past", "internal/shape/shape.go",
  "\tcase isComment(t):\n\t\treturn true\n", "\tcase false && isComment(t):\n\t\treturn true\n", PROG)
m("H-13 a case statement inside a group is read past", "internal/shape/shape.go",
  "\tcase !t.meta && t.quotedAt < 0 && t.text == \"case\":\n", "\tcase false:\n", PROG)
m("H-13 a here-document inside a group is read past", "internal/shape/shape.go",
  "\tcase hereDoc(toks, j):\n\t\treturn true\n", "\tcase false && hereDoc(toks, j):\n\t\treturn true\n", PROG)
m("H-13 a newline in a group after a here-document is read past", "internal/shape/shape.go",
  "\tcase heredoc && t.nlBefore:\n", "\tcase false:\n", PROG)
m("H-13 a here-string is taken for a here-document", "internal/shape/shape.go",
  "\treturn next >= len(toks) || !(toks[next].meta && toks[next].glued && toks[next].text == \"<\")",
  "\treturn next >= 0", "TestProgramIsAProgram")
m("H-13 a trailing backslash after an expansion is read as a real one", "internal/shape/tokenize.go",
  "\t\t\t\tflush()\n\t\t\t\treturn toks, unterminated()", "\t\t\t\tflush()\n\t\t\t\treturn toks, errUnterminated",
  "TestTokenizeProgramStops|" + PROG)
m("H-13 the function-definition scan reads &> as & and then >", "internal/shape/shape.go",
  "\t\t\tj++\n\t\t\tfallthrough", "\t\t\tfallthrough", "TestProgramIsAProgram")
m("H-13 a word glued to any operator and a paren is named by its front", "internal/shape/shape.go",
  "\tif op != \"<\" && op != \">\" {", "\tif false && op != \"<\" && op != \">\" {", "TestProgramIsAProgram")
m("H-13 a word glued to > and digits is read as a numeric glob", "internal/shape/shape.go",
  "|| op == \"<\" && isNumericGlob(next)", "|| isNumericGlob(next)", "TestProgramIsAProgram")
m("H-13 a backtick inside a paren group opens nothing", "internal/shape/shape.go",
  "\tcase t.ticks%2 == 1:\n\t\treturn append(open, '`')\n", "\tcase false:\n\t\treturn append(open, '`')\n", PROG)
m("H-13 a paren inside backticks closes them", "internal/shape/shape.go",
  "\tcase top == '`' || !t.meta:", "\tcase !t.meta:", PROG)
m("H-13 & then > is read as &>", "internal/shape/shape.go",
  "toks[j+1].meta && toks[j+1].glued && strings.HasPrefix(toks[j+1].text, \">\")",
  "toks[j+1].meta && strings.HasPrefix(toks[j+1].text, \">\")", "TestProgramIsAProgram")
m("H-13 a numeric glob spaced from the command word is read as part of it", "internal/shape/shape.go",
  "!toks[i+1].meta || !toks[i+1].glued || !toks[i+2].glued", "!toks[i+1].meta || !toks[i+2].glued", "TestProgramIsAProgram")
m("H-13 digits spaced from a glued < are read as a numeric glob", "internal/shape/shape.go",
  "!toks[i+1].meta || !toks[i+1].glued || !toks[i+2].glued", "!toks[i+1].meta || !toks[i+1].glued", "TestProgramIsAProgram")
m("H-13 a path ending in . is named", "internal/shape/shape.go",
  "\tcase s != \".\" && (path.Base(s) == \".\" || path.Base(s) == \"..\"):",
  "\tcase s != \".\" && path.Base(s) == \"..\":", PROG)
m("H-13 a path ending in .. is named", "internal/shape/shape.go",
  "\tcase s != \".\" && (path.Base(s) == \".\" || path.Base(s) == \"..\"):",
  "\tcase s != \".\" && path.Base(s) == \".\":", PROG)
m("H-13 the source builtin is refused as a directory", "internal/shape/shape.go",
  "\tcase s != \".\" && (path.Base(s) == \".\" || path.Base(s) == \"..\"):",
  "\tcase path.Base(s) == \".\" || path.Base(s) == \"..\":", "TestProgramIsAProgram")
m("H-13 a jobspec is named", "internal/shape/shape.go",
  "\tcase strings.HasPrefix(s, \"%\"):", "\tcase false && strings.HasPrefix(s, \"%\"):", PROG)
m("H-13 a quoted command word split by a tab is named", "internal/shape/shape.go",
  "\t\tcase c < 0x21 || c > 0x7e:", "\t\tcase c < 0x21 && c != '\\t' || c > 0x7e:", PROG)
m("H-13 a quoted command word split by a newline is named", "internal/shape/shape.go",
  "\t\tcase c < 0x21 || c > 0x7e:", "\t\tcase c < 0x21 && c != '\\n' || c > 0x7e:", PROG)
m("H-13 a tab in the line is taken for a control byte", "internal/shape/shape.go",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\t' && c != '\\n' || c == 0x7f {",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\n' || c == 0x7f {", "TestProgramIsAProgram")
m("H-13 a newline in the line is taken for a control byte", "internal/shape/shape.go",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\t' && c != '\\n' || c == 0x7f {",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\t' || c == 0x7f {", "TestProgramIsAProgram")
# Re-anchored after the masked-status refactor: the backtick case also marks the
# line grouped for the list walk's here-document reading; only the count goes.
m("H-13 the tokenizer never counts an unquoted backtick", "internal/shape/tokenize.go",
  "\t\t\tif c == '`' {\n\t\t\t\tw.ticks++\n\t\t\t\tgrouped = true\n\t\t\t}\n", "\t\t\tif c == '`' {\n\t\t\t\tgrouped = true\n\t\t\t}\n", "TestTokenizeTicks|" + PROG)
m("H-13 the tokenizer never counts a backtick inside double quotes", "internal/shape/tokenize.go",
  "\t\t\t\tif s[i] == '`' {\n\t\t\t\t\tw.ticks++\n\t\t\t\t}\n", "", "TestTokenizeTicks")
# Judged by the corpus alone, where the two lines of the PR #29 review are the
# only ones this break reopens: "$(echo "a;b")" ends at its first inner quote
# again, and the ; inside it ends the search for the () after the command word.
m("H-13 a $( ) inside double quotes is not a context in which quotes nest", "internal/shape/tokenize.go",
  "\t\t\t\tif program && s[i] == '$' && i+1 < len(s) && s[i+1] == '(' {",
  "\t\t\t\tif false && program && s[i] == '$' && i+1 < len(s) && s[i+1] == '(' {", "TestNoCorpusLineLeaksIntoProgram")
# The review of that change: each guard comsubEnd and its helpers keep against
# a reading the shell may not share. TPROG adds the tokenizer's own tests.
TPROG = "TestTokenizeProgram|" + PROG
C = "internal/shape/comsub.go"
m("H-13 a paren inside $[ ] counts as a group", C,
  "\t\tcase '(', ')', '{', '}', '\\'', '\"', '`', '\\\\', '\\n':", "\t\tcase '{', '}', '\\'', '\"', '`', '\\\\', '\\n':", TPROG)
m("H-13 a nested [ ] ends a $[ ]", C, "\t\tcase '[':\n\t\t\tdepth++\n", "", TPROG)
m("H-13 $[ is not read as arithmetic", C, "\t\treturn bracketEnd(r.s, i+2)", "\t\treturn i", TPROG)
m("H-13 an even run of $ opens an expansion in a substitution", C,
  "\t\tif dollars%2 == 0 {\n\t\t\treturn -1\n\t\t}", "\t\tif false {\n\t\t\treturn -1\n\t\t}", TPROG)
m("H-13 \"$$(\" opens a substitution", "internal/shape/tokenize.go",
  "\t\t\t\t\tif dq%2 == 1 {", "\t\t\t\t\tif true {", TPROG)
m("H-13 a $' inside double quotes in a substitution is a quote", C,
  "\t\t\t\tcontinue // no quote inside double quotes\n", "", TPROG)
m("H-13 a nested $( ) is counted as parens", C, "\t\treturn r.substEnd(i, nest)", "\t\treturn i", TPROG)
m("H-13 a backslash-newline the joining left is read past in a substitution", C,
  "\t\t\tif i+1 >= len(s) || r.continuesAt(i) {", "\t\t\tif i+1 >= len(s) {", TPROG)
m("H-13 a backslash-newline the joining left is read past in its double quotes", C,
  "\t\t\tif r.continuesAt(i) {", "\t\t\tif false {", TPROG)
m("H-13 the joining's offsets are not kept", "internal/shape/shape.go",
  "\t\t\t\tjoins = append(joins, b.Len())\n", "", TPROG)
m("H-13 a quoted span joined across a newline opens a here-document's body", C,
  " || r.joinedIn(from, to)))", "))", TPROG)
m("H-13 an unquoted here-document's body is read across backslash-newlines", C,
  "\t\t\tif !d.quoted && (r.joinedIn(i-1, e)", "\t\t\tif false && (r.joinedIn(i-1, e)", TPROG)
m("H-13 a quoted here-document's line as written may begin with its delimiter", C,
  "\t\tif strings.HasPrefix(line(r.s[from:p]), d.delim) {", "\t\tif false {", TPROG)
m("H-13 a quoted here-document's joined line is read as one line", C,
  "\t\tfrom = p\n", "\t\t_ = p\n", TPROG)
m("H-13 a quoted delimiter leaves its body unquoted", C,
  "\t\t\td.quoted = true\n\t\t\ti += j + 1", "\t\t\ti += j + 1", TPROG)
m("H-13 case is a keyword wherever it stands", C,
  "\t\tcase c == 'c' && cmd.begins() && isCase(s, i, body):", "\t\tcase c == 'c' && isCase(s, i, body):", TPROG)
m("H-13 a command's first word never names it", C, "\t\tp.named = true", "\t\tp.named = false", TPROG)
m("H-13 a separator does not begin a command", C,
  "\tif strings.IndexByte(\";&|()\\n\", c) >= 0 {", "\tif false {", TPROG)
m("H-13 zsh's } and ]] do not begin a command", C, "\tcase w == \"}\" || w == \"]]\":", "\tcase false:", TPROG)
m("H-13 time's options name the command", C, "\t\tp.prefix = true", "\t\tp.prefix = false", TPROG)
m("H-13 then names the command", C, "\"select\": true, \"then\": true,", "\"select\": true,", TPROG)
m("H-13 an assignment names the command", C, "commandKeywords[w], strings.IndexByte(w, '=') >= 0, ", "commandKeywords[w], ", TPROG)
m("H-13 a redirect's target names the command", C,
  "\t\tcase '<', '>':\n\t\t\treturn true", "\t\tcase '<', '>':\n\t\t\treturn false", TPROG)
m("H-13 a redirect's fd names the command", C,
  "\tif i+n < len(s) && (s[i+n] == '<' || s[i+n] == '>') {", "\tif false {", TPROG)
m("H-13 $(( is read as a $( ) with a group", C,
  "\tif i+2 < len(r.s) && r.s[i+2] == '(' {", "\tif false {", TPROG)
m("H-13 $(( whose ( closes alone is read as arithmetic", C,
  "\t\t\tif i+1 < len(s) && s[i+1] == ')' {\n\t\t\t\treturn i + 1\n\t\t\t}\n\t\t\treturn -1", "\t\t\treturn i + 1", TPROG)
m("H-13 arithmetic holds what only a command can", C,
  "\t\tcase '\\'', '\"', '`', '\\\\', '\\n', ';':\n\t\t\treturn -1\n", "\t\tcase '\\'', '\"', '`', '\\\\', '\\n', ';':\n", TPROG)
m("H-13 a # after a blank in arithmetic is read past", C,
  "\t\t\tif i == body || strings.IndexByte(wordBreak, s[i-1]) >= 0 {\n\t\t\t\treturn -1", "\t\t\tif false {\n\t\t\t\treturn -1", TPROG)
m("H-13 case in arithmetic is read past", C, "\t\t\tif isCase(s, i, body) {", "\t\t\tif false {", TPROG)
m("H-13 a run of $ in arithmetic is not counted", C, "\t\t\tdollars++\n\t\t\tend := r.dollarOpens(i, dollars, nest+1)", "\t\t\tdollars = 1\n\t\t\tend := r.dollarOpens(i, dollars, nest+1)", TPROG)
m("H-13 an expansion in arithmetic is read as parens", C,
  "\t\t\tdollarAt = i\n\t\t\tend := r.dollarOpens(i, dollars, nest+1)\n\t\t\tif end < 0 {\n\t\t\t\treturn -1\n\t\t\t}\n\t\t\ti = end",
  "\t\t\tdollarAt = i\n\t\t\tend := i\n\t\t\tif end < 0 {\n\t\t\t\treturn -1\n\t\t\t}\n\t\t\ti = end", TPROG)
m("H-13 the function-definition scan reads past a redirect with no target", "internal/shape/shape.go",
  "\t\t\tif noTarget(toks, end) {", "\t\t\tif false && noTarget(toks, end) {", PROG)
m("H-13 the program search reads past a redirect with no target", "internal/shape/shape.go",
  "\tif noTarget(toks, i) {\n\t\treturn 0, false\n\t}\n", "\tif false && noTarget(toks, i) {\n\t\treturn 0, false\n\t}\n", PROG)
m("H-13 a redirect takes its target from the next line", "internal/shape/shape.go",
  "\tcase next.nlBefore:\n\t\treturn true\n", "\tcase false && next.nlBefore:\n\t\treturn true\n", PROG)
m("H-13 a redirect followed by another is read past", "internal/shape/shape.go",
  "\treturn !procSub(toks, i+1)\n}", "\treturn !procSub(toks, i+1) && !isRedirect(next.text)\n}", PROG)
m("H-13 a paren is refused as a redirect's missing target", "internal/shape/shape.go",
  "\tcase !next.meta, next.text == \"(\":\n", "\tcase !next.meta:\n", "TestProgramIsAProgram")
m("H-13 a process substitution is refused as a redirect's target", "internal/shape/shape.go",
  "\treturn !procSub(toks, i+1)\n}", "\treturn true\n}", "TestProgramIsAProgram")
m("H-13 a paren spaced from its < is read as a process substitution", "internal/shape/shape.go",
  "\treturn next.meta && next.glued && next.text == \"(\"\n}", "\treturn next.meta && next.text == \"(\"\n}", PROG)
m("H-13 a comment where a redirect target belongs is taken for the target", "internal/shape/shape.go",
  "\tcase isComment(next):\n\t\treturn true\n", "\tcase false && isComment(next):\n\t\treturn true\n", PROG)
m("H-13 a C0 control other than NUL and CR is read past", "internal/shape/shape.go",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\t' && c != '\\n' || c == 0x7f {",
  "\t\tif c := cmd[i]; c == 0 || c == '\\r' || c == 0x7f {", PROG)
m("H-13 a DEL in the line is read past", "internal/shape/shape.go",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\t' && c != '\\n' || c == 0x7f {",
  "\t\tif c := cmd[i]; c < 0x20 && c != '\\t' && c != '\\n' {", PROG)
m("H-13 ~user is named", "internal/shape/shape.go",
  "\tcase strings.HasPrefix(s, \"~\") && !strings.Contains(s, \"/\"):",
  "\tcase false && strings.HasPrefix(s, \"~\") && !strings.Contains(s, \"/\"):", PROG)
m("H-13 zsh's ^ is read as part of a program name", "internal/shape/shape.go",
  "\t\tcase c == '^' || c == '#':", "\t\tcase c == '#':", PROG)
m("H-13 zsh's # is read as part of a program name", "internal/shape/shape.go",
  "\t\tcase c == '^' || c == '#':", "\t\tcase c == '^':", PROG)
m("H-13 an empty word is named `.`", "internal/shape/shape.go",
  "which reads as the source builtin.\n\t\treturn false", "which reads as the source builtin.\n\t\treturn true", PROG)

m("H-13 the program search reads $'...' as argc does", "internal/shape/shape.go",
  "\tpshaped, perr := tokenizeProgram(cmd)", "\tpshaped, perr := tokenizeShape(cmd)", PROG)
m("H-13 any earlier $ opens a $'...' string", "internal/shape/tokenize.go",
  "\t\t\tif program && dollarAt >= 0 && dollarAt == i-1 {", "\t\t\tif program && dollarAt >= 0 {", "TestTokenizeProgramReadsANSICQuotes")
m("H-13 a quote at the start of the line is read as $'...'", "internal/shape/tokenize.go",
  "program && dollarAt >= 0 && dollarAt == i-1", "program && dollarAt == i-1", "TestTokenizeProgramReadsANSICQuotes|" + PROG)
m("H-13 bash's $$' is read as zsh's $'...'", "internal/shape/tokenize.go",
  "\t\t\t\tif dollars%2 == 0 {", "\t\t\t\tif false {", "TestTokenizeProgramReadsANSICQuotes|" + PROG)
m("H-13 an odd run of $ is read as bash's $$'", "internal/shape/tokenize.go",
  "\t\t\t\tif dollars%2 == 0 {", "\t\t\t\tif dollars > 1 {", "TestTokenizeProgramReadsANSICQuotes")
m("H-13 an unterminated quote after an expansion is read as a real one", "internal/shape/tokenize.go",
  "\t\tif program && (w.opaque || expanded) {", "\t\tif false {", "TestTokenizeProgramStops|" + PROG)
m("H-13 an expansion in an earlier word leaves a later quote certain", "internal/shape/tokenize.go",
  "\t\tif program && (w.opaque || expanded) {", "\t\tif program && w.opaque {", "TestTokenizeProgramStops|" + PROG)
m("H-13 the count's tokenizer stops where the program search does", "internal/shape/tokenize.go",
  "\t\tif program && (w.opaque || expanded) {", "\t\tif w.opaque || expanded {", "TestTokenize")
