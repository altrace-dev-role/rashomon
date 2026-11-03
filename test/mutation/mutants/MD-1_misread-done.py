# MD-1: a name, an item number, half a word or a table's header is no claim
# (card MISREAD-DONE1): a number is a count only in its predicate's clause; a
# predicate in capitals in a sentence that is not is a name; a predicate
# joined by a hyphen to a word rune is part of another word; a line with a bar
# over a delimiter row names columns. Each break removes one reading and names
# the tests that hold it.
C = "internal/verify/claims.go"
T = "internal/verify/claimtext.go"
m("MD-1 a number from another clause is a count", C,
  "\t\tif r.number && isNumber(ws[s]) && cs[s] == cs[p] {\n", "\t\tif r.number && isNumber(ws[s]) {\n", "TestMD1a_")
m("MD-1 a predicate in capitals is a claim", C,
  "\t\tif m&markJoined != 0 || m&markUpper != 0 && !s.loud {\n", "\t\tif m&markJoined != 0 {\n", "TestMD1b_")
m("MD-1 a sentence in capitals throughout loses its claim", C,
  "\t\ts.loud = s.loud && m&markLower == 0\n", "\t\ts.loud = false\n", "TestMD1b_")
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
m("MD-1 a table's header is read", T,
  "isDelivered(lead, delivered), tableHeader(lead, b):\n", "isDelivered(lead, delivered):\n", "TestMD1d_")
m("MD-1 a header needs no delimiter row under it", T,
  "\treturn delimiterRow(rest)\n", "\treturn true\n", "TestMD1d_")
m("MD-1 a line with no bar heads a table", T,
  "\tif bytes.IndexByte(line, '|') < 0 {\n\t\treturn false\n\t}\n", "", "TestMD1d_")
m("MD-1 a delimiter cell may hold words", T,
  "\t\tif len(cell) == 0 || len(bytes.Trim(cell, \"-\")) != 0 {\n", "\t\tif len(cell) == 0 {\n", "TestMD1d_")
m("MD-1 an empty delimiter cell counts", T,
  "\t\tif len(cell) == 0 || len(bytes.Trim(cell, \"-\")) != 0 {\n", "\t\tif len(bytes.Trim(cell, \"-\")) != 0 {\n", "TestMD1d_")
m("MD-1 an aligned delimiter cell is none", T,
  "\t\tcell = bytes.TrimSuffix(bytes.TrimPrefix(bytes.TrimSpace(cell), []byte(\":\")), []byte(\":\"))\n",
  "\t\tcell = bytes.TrimSpace(cell)\n", "TestMD1d_")
m("MD-1 a closing bar is a cell", T,
  "\tl = bytes.TrimSuffix(bytes.TrimPrefix(l, []byte(\"|\")), []byte(\"|\"))\n", "\tl = bytes.TrimPrefix(l, []byte(\"|\"))\n", "TestMD1d_")
